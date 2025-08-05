package main

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"
	"time"

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
		return err
	}

	client := utils.CreateClient()

	originalBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch unprocessed ASINs: %w", err)
	}

	upcomingBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch upcoming ASINs: %w", err)
	}

	newBooks, err := processASINs(cfg, client, append(originalBooks, upcomingBooks...))
	if err != nil {
		return fmt.Errorf("PA API processing failed: %v", err)
	}

	utils.SortByReleaseDate(newBooks)
	if reflect.DeepEqual(originalBooks, newBooks) {
		return nil
	}

	if err := utils.SaveASINs(cfg, newBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save unprocessed ASINs: %w", err)
	}

	if err := updateGist(newBooks); err != nil {
		return fmt.Errorf("error update gist: %s", err)
	}

	if err := utils.SaveASINs(cfg, []utils.KindleBook{}, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
		return fmt.Errorf("failed to clear upcoming ASINs: %w", err)
	}

	return nil
}

func processASINs(cfg aws.Config, client paapi5.Client, original []utils.KindleBook) ([]utils.KindleBook, error) {
	var result []utils.KindleBook
	var successfulRequests int
	var processedCount int

	uniqueBooks := utils.UniqueASINs(original)
	totalBooks := len(uniqueBooks)
	chunks := utils.ChunkedASINs(uniqueBooks, 10)

	logBookProcessing := func(title, url string, releaseDate time.Time, prefix string) {
		processedCount++
		releaseDateStr := "N/A"
		if !releaseDate.IsZero() {
			releaseDateStr = releaseDate.Format("2006-01-02")
		}
		log.Printf("%s %04d/%04d: %s | %s | %s", prefix, processedCount, totalBooks, releaseDateStr, title, url)
	}

	for _, chunk := range chunks {
		resp, err := utils.GetItems(cfg, client, chunk)
		if err != nil {
			fallbackBooks := utils.AppendFallbackBooks(chunk, original)
			for _, book := range fallbackBooks {
				logBookProcessing(book.Title, book.URL, book.ReleaseDate.Time, "[Failure]")
				result = append(result, book)
			}

			utils.PutMetric(cfg, "KindleBot/SaleChecker", "APIFailure")
			// utils.AlertToSlack(fmt.Errorf("error fetching item details: %v", err), false)
			continue
		}

		successfulRequests++
		utils.PutMetric(cfg, "KindleBot/SaleChecker", "APISuccess")
		for _, item := range resp.ItemsResult.Items {
			book := utils.GetBook(item.ASIN, original)
			logBookProcessing(item.ItemInfo.Title.DisplayValue, item.DetailPageURL, book.ReleaseDate.Time, "[Success]")

			if !isKindle(item) {
				utils.AlertToSlack(fmt.Errorf(
					"the item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
					item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
				), false)
				continue
			}

			maxPrice := math.Max(book.MaxPrice, (*item.Offers.Listings)[0].Price.Amount)

			if conditions := extractQualifiedConditions(item, maxPrice); len(conditions) > 0 {
				utils.LogAndNotify(formatSlackMessage(item, conditions), true)
			} else {
				result = append(result, utils.MakeBook(item, maxPrice))
			}
		}
	}

	if successfulRequests == 0 {
		return result, fmt.Errorf("all PA API requests failed (%d/%d)", successfulRequests, len(chunks))
	}

	return result, nil
}

func isKindle(item entity.Item) bool {
	return item.ItemInfo.Classifications.Binding.DisplayValue == "Kindle版"
}

func extractQualifiedConditions(item entity.Item, maxPrice float64) []string {
	amount := (*item.Offers.Listings)[0].Price.Amount
	points := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string
	if diff := maxPrice - amount; diff >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅ 最高額との価格差 %.0f円", diff))
	}
	if points >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント %dpt", points))
	}
	if percent := float64(points) / amount * 100; percent >= 20 {
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
