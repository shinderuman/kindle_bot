package main

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"

	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"

	"kindle_bot/utils"
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
		return fmt.Errorf("Error fetching unprocessed ASINs: %v", err)
	}

	upcomingBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching upcoming ASINs: %v", err)
	}

	newBooks := processASINs(client, append(originalBooks, upcomingBooks...))

	utils.SortByReleaseDate(newBooks)
	if reflect.DeepEqual(originalBooks, newBooks) {
		return nil
	}

	if err := utils.SaveASINs(cfg, newBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
	}

	if err := utils.UpdateGist(newBooks, "わいのセールになってほしい本.md"); err != nil {
		return fmt.Errorf("Error update gist: %s", err)
	}

	if err := utils.SaveASINs(cfg, []utils.KindleBook{}, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
		return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
	}

	return nil
}

func processASINs(client paapi5.Client, original []utils.KindleBook) []utils.KindleBook {
	var result []utils.KindleBook

	for _, chunk := range utils.ChunkedASINs(utils.UniqueASINs(original), 10) {
		resp, err := utils.GetItems(client, chunk)
		if err != nil {
			result = append(result, utils.AppendFallbackBooks(chunk, original)...)
			// utils.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}

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
