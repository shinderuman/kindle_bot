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
	// Gist configuration
	gistID       = "571a55fc0f9e56156cae277ded0cf09c"
	gistFilename = "わいのセールになってほしい本.md"

	// Condition thresholds
	minPriceDiff     = 151
	minPoints        = 151
	minPointsPercent = 20.0

	// Error messages
	errFetchUnprocessedASINs = "error fetching unprocessed ASINs"
	errFetchUpcomingASINs    = "error fetching upcoming ASINs"
	errSaveUnprocessedASINs  = "error saving unprocessed ASINs"
	errUpdateGist            = "error update gist"
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
		return fmt.Errorf("%s: %w", errFetchUnprocessedASINs, err)
	}

	upcomingBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("%s: %w", errFetchUpcomingASINs, err)
	}

	newBooks := processASINs(cfg, client, append(originalBooks, upcomingBooks...))

	utils.SortByReleaseDate(newBooks)
	if reflect.DeepEqual(originalBooks, newBooks) {
		return nil
	}

	if err := utils.SaveASINs(cfg, newBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("%s: %w", errSaveUnprocessedASINs, err)
	}

	if err := updateGist(newBooks); err != nil {
		return fmt.Errorf("%s: %w", errUpdateGist, err)
	}

	if err := utils.SaveASINs(cfg, []utils.KindleBook{}, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
		return fmt.Errorf("%s: %w", errSaveUnprocessedASINs, err)
	}

	return nil
}

func processASINs(cfg aws.Config, client paapi5.Client, original []utils.KindleBook) []utils.KindleBook {
	var result []utils.KindleBook

	for _, chunk := range utils.ChunkedASINs(utils.UniqueASINs(original), 10) {
		resp, err := utils.GetItems(cfg, client, chunk)
		if err != nil {
			result = append(result, utils.AppendFallbackBooks(chunk, original)...)
			utils.PutMetric(cfg, "KindleBot/SaleChecker", "APIFailure")
			// utils.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}

		utils.PutMetric(cfg, "KindleBot/SaleChecker", "APISuccess")
		for _, item := range resp.ItemsResult.Items {
			log.Println(item.ItemInfo.Title.DisplayValue)

			if !isKindle(item) {
				utils.AlertToSlack(fmt.Errorf(
					"The item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
					item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
				), false)
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
	}

	return result
}

func isKindle(item entity.Item) bool {
	return item.ItemInfo.Classifications.Binding.DisplayValue == "Kindle版"
}

func extractQualifiedConditions(item entity.Item, maxPrice float64) []string {
	amount := (*item.Offers.Listings)[0].Price.Amount
	points := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string
	if diff := maxPrice - amount; diff >= minPriceDiff {
		conditions = append(conditions, fmt.Sprintf("✅ 最高額との価格差 %.0f円", diff))
	}
	if points >= minPoints {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント %dpt", points))
	}
	if percent := float64(points) / amount * 100; percent >= minPointsPercent {
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
