package main

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"

	"kindle_bot/utils"
)

const (
	gistID       = "571a55fc0f9e56156cae277ded0cf09c"
	gistFilename = "わいのセールになってほしい本.md"
)

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return fmt.Errorf("failed to initialize AWS config: %w", err)
	}

	client := utils.CreateClient()

	originalBooks, upcomingBooks, err := fetchAllBooks(cfg)
	if err != nil {
		return fmt.Errorf("failed to fetch books: %w", err)
	}

	allBooks := append(originalBooks, upcomingBooks...)
	newBooks, hasAPIError := processASINs(cfg, client, allBooks)

	if err := saveProcessedBooks(cfg, originalBooks, newBooks); err != nil {
		return fmt.Errorf("failed to save processed books: %w", err)
	}

	if hasAPIError {
		return fmt.Errorf("PA API errors occurred during processing")
	}

	return nil
}

func fetchAllBooks(cfg aws.Config) ([]utils.KindleBook, []utils.KindleBook, error) {
	originalBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch unprocessed ASINs: %w", err)
	}

	upcomingBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch upcoming ASINs: %w", err)
	}

	return originalBooks, upcomingBooks, nil
}

func saveProcessedBooks(cfg aws.Config, originalBooks, newBooks []utils.KindleBook) error {
	if err := utils.SaveBooksIfChanged(cfg, originalBooks, newBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save unprocessed ASINs: %w", err)
	}

	// Only update gist and clear upcoming if books changed
	utils.SortByReleaseDate(newBooks)
	if reflect.DeepEqual(originalBooks, newBooks) {
		return nil
	}

	if err := updateGist(newBooks); err != nil {
		return fmt.Errorf("failed to update gist: %w", err)
	}

	if err := utils.SaveASINs(cfg, []utils.KindleBook{}, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
		return fmt.Errorf("failed to clear upcoming ASINs: %w", err)
	}

	return nil
}

func processASINs(cfg aws.Config, client paapi5.Client, original []utils.KindleBook) ([]utils.KindleBook, bool) {
	var result []utils.KindleBook
	var hasAPIError bool

	for _, chunk := range utils.ChunkedASINs(utils.UniqueASINs(original), utils.DefaultChunkSize) {
		resp, err := utils.GetItems(cfg, client, chunk)
		if err != nil {
			result = append(result, utils.AppendFallbackBooks(chunk, original)...)
			utils.RecordAPIMetric(cfg, "KindleBot/SaleChecker", false)
			hasAPIError = true
			continue
		}

		utils.RecordAPIMetric(cfg, "KindleBot/SaleChecker", true)
		processedBooks := processItemsForSale(resp.ItemsResult.Items, original)
		result = append(result, processedBooks...)
	}

	return result, hasAPIError
}

func processItemsForSale(items []entity.Item, original []utils.KindleBook) []utils.KindleBook {
	var result []utils.KindleBook

	for _, item := range items {
		log.Printf("Processing: %s", item.ItemInfo.Title.DisplayValue)

		if !utils.IsKindleEdition(item) {
			alertNonKindleItem(item)
			continue
		}

		book := utils.GetBook(item.ASIN, original)
		maxPrice := math.Max(book.MaxPrice, (*item.Offers.Listings)[0].Price.Amount)

		if conditions := extractQualifiedConditions(item, maxPrice); len(conditions) > 0 {
			utils.LogAndNotify(formatSlackMessage(item, conditions), true)
		} else {
			result = append(result, utils.MakeBook(item, maxPrice))
		}
	}

	return result
}

func alertNonKindleItem(item entity.Item) {
	utils.AlertToSlack(fmt.Errorf(
		"The item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
		item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
	), false)
}

func extractQualifiedConditions(item entity.Item, maxPrice float64) []string {
	amount := (*item.Offers.Listings)[0].Price.Amount
	points := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string
	if diff := maxPrice - amount; diff >= utils.MinPriceDiff {
		conditions = append(conditions, fmt.Sprintf("✅ 最高額との価格差 %.0f円", diff))
	}
	if points >= utils.MinPoints {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント %dpt", points))
	}
	if percent := float64(points) / amount * 100; percent >= utils.MinPointPercent {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント還元 %.1f%%", percent))
	}

	return conditions
}

func formatSlackMessage(item entity.Item, conditions []string) string {
	return fmt.Sprintf(
		"📚 セール情報: %s\n条件達成: %s\n%s",
		item.ItemInfo.Title.DisplayValue,
		strings.Join(conditions, " "),
		item.DetailPageURL,
	)
}

func updateGist(books []utils.KindleBook) error {
	var lines []string
	for _, book := range books {
		lines = append(lines, fmt.Sprintf("* [[%s]%s](%s)", book.ReleaseDate.Format("2006-01-02"), book.Title, book.URL))
	}

	markdown := fmt.Sprintf("## 合計 %d冊\n%s", len(books), strings.Join(lines, "\n"))

	return utils.UpdateGist(gistID, gistFilename, markdown)
}
