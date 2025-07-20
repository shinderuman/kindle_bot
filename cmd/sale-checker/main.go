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

	"kindle_bot/internal/config"
	"kindle_bot/internal/notification"
	"kindle_bot/internal/paapi"
	"kindle_bot/internal/runner"
	"kindle_bot/internal/storage"
	"kindle_bot/pkg/models"
)

const (
	gistID       = "571a55fc0f9e56156cae277ded0cf09c"
	gistFilename = "わいのセールになってほしい本.md"
)

func main() {
	runner.Run(process)
}

func process() error {
	cfg, err := storage.InitAWSConfig()
	if err != nil {
		return err
	}

	client := paapi.CreateClient()

	originalBooks, err := storage.FetchASINs(cfg, config.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching unprocessed ASINs: %v", err)
	}

	upcomingBooks, err := storage.FetchASINs(cfg, config.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching upcoming ASINs: %v", err)
	}

	newBooks := processASINs(cfg, client, append(originalBooks, upcomingBooks...))

	storage.SortByReleaseDate(newBooks)
	if reflect.DeepEqual(originalBooks, newBooks) {
		return nil
	}

	if err := storage.SaveASINs(cfg, newBooks, config.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
	}

	if err := updateGist(newBooks); err != nil {
		return fmt.Errorf("Error update gist: %s", err)
	}

	if err := storage.SaveASINs(cfg, []models.KindleBook{}, config.EnvConfig.S3UpcomingObjectKey); err != nil {
		return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
	}

	return nil
}

func processASINs(cfg aws.Config, client paapi5.Client, original []models.KindleBook) []models.KindleBook {
	var result []models.KindleBook

	for _, chunk := range storage.ChunkedASINs(storage.UniqueASINs(original), 10) {
		resp, err := paapi.GetItems(cfg, client, chunk)
		if err != nil {
			result = append(result, storage.AppendFallbackBooks(chunk, original)...)
			notification.PutMetric(cfg, "KindleBot/SaleChecker", "APIFailure")
			// notification.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}

		notification.PutMetric(cfg, "KindleBot/SaleChecker", "APISuccess")
		for _, item := range resp.ItemsResult.Items {
			log.Println(item.ItemInfo.Title.DisplayValue)

			if !isKindle(item) {
				notification.AlertToSlack(fmt.Errorf(
					"The item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
					item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
				), false)
				continue
			}

			book := storage.GetBook(item.ASIN, original)
			maxPrice := math.Max(book.MaxPrice, (*item.Offers.Listings)[0].Price.Amount)

			if conditions := extractQualifiedConditions(item, maxPrice); len(conditions) > 0 {
				notification.LogAndNotify(formatSlackMessage(item, conditions), true)
			} else {
				result = append(result, paapi.MakeBook(item, maxPrice))
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

func updateGist(books []models.KindleBook) error {
	var lines []string
	for _, book := range books {
		lines = append(lines, fmt.Sprintf("* [[%s]%s](%s)", book.ReleaseDate.Format("2006-01-02"), book.Title, book.URL))
	}

	markdown := fmt.Sprintf("## 合計 %d冊\n%s", len(books), strings.Join(lines, "\n"))

	return notification.UpdateGist(gistID, gistFilename, markdown)
}