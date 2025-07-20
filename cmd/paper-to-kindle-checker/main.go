package main

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/internal/config"
	"kindle_bot/internal/notification"
	"kindle_bot/internal/paapi"
	"kindle_bot/internal/runner"
	"kindle_bot/internal/storage"
	"kindle_bot/pkg/models"
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
	originalPaperBooks, err := storage.FetchASINs(cfg, config.EnvConfig.S3PaperBooksObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching paper books ASINs: %v", err)
	}

	var newUnprocessed, newPaperBooks []models.KindleBook
	for _, chunk := range storage.ChunkedASINs(storage.UniqueASINs(originalPaperBooks), 10) {
		items, err := paapi.GetItems(cfg, client, chunk)
		if err != nil {
			newPaperBooks = append(newPaperBooks, storage.AppendFallbackBooks(chunk, originalPaperBooks)...)
			notification.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
			// notification.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}
		notification.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

		for _, paper := range items.ItemsResult.Items {
			log.Println(paper.ItemInfo.Title.DisplayValue)

			kindleItem, err := searchKindleEdition(cfg, client, paper)
			if err != nil {
				notification.AlertToSlack(err, false)
				newPaperBooks = append(newPaperBooks, paapi.MakeBook(paper, 0))
				notification.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
				continue
			}
			notification.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

			if kindleItem != nil {
				notification.LogAndNotify(formatSlackMessage(paper, *kindleItem), true)
				newUnprocessed = append(newUnprocessed, paapi.MakeBook(*kindleItem, 0))
			} else {
				newPaperBooks = append(newPaperBooks, paapi.MakeBook(paper, 0))
			}
		}
	}

	if err := updateASINs(cfg, newUnprocessed); err != nil {
		return err
	}

	storage.SortByReleaseDate(newPaperBooks)
	if !reflect.DeepEqual(originalPaperBooks, newPaperBooks) {
		if err := storage.SaveASINs(cfg, newPaperBooks, config.EnvConfig.S3PaperBooksObjectKey); err != nil {
			return fmt.Errorf("Error saving paper books ASINs: %v", err)
		}
	}

	return nil
}

func searchKindleEdition(cfg aws.Config, client paapi5.Client, paper entity.Item) (*entity.Item, error) {
	q := paapi.CreateSearchQuery(
		client,
		query.Title,
		cleanTitle(paper.ItemInfo.Title.DisplayValue),
		(*paper.Offers.Listings)[0].Price.Amount+20000,
	)

	res, err := paapi.SearchItems(cfg, client, q, 5)
	if err != nil {
		return nil, fmt.Errorf("Error searching items: %v", err)
	}

	if res.SearchResult == nil {
		return nil, nil
	}

	for _, kindle := range res.SearchResult.Items {
		if isSameKindleBook(paper, kindle) {
			return &kindle, nil
		}
	}
	return nil, nil
}

func updateASINs(cfg aws.Config, newItems []models.KindleBook) error {
	if len(newItems) == 0 {
		return nil
	}

	currentUnprocessed, err := storage.FetchASINs(cfg, config.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching unprocessed ASINs: %v", err)
	}

	allUnprocessed := append(currentUnprocessed, newItems...)
	storage.SortByReleaseDate(allUnprocessed)

	if err := storage.SaveASINs(cfg, allUnprocessed, config.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
	}

	currentNotified, err := storage.FetchASINs(cfg, config.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching notified ASINs: %v", err)
	}

	allNotified := append(currentNotified, newItems...)
	storage.SortByReleaseDate(allNotified)

	if err := storage.SaveASINs(cfg, allNotified, config.EnvConfig.S3NotifiedObjectKey); err != nil {
		return fmt.Errorf("Error saving notified ASINs: %v", err)
	}
	return nil
}

func cleanTitle(title string) string {
	return strings.TrimSpace(regexp.MustCompile(`[\(\)（）【】〔〕]|\s*[0-9]+.*$`).ReplaceAllString(title, ""))
}

func formatSlackMessage(paper, kindle entity.Item) string {
	return fmt.Sprintf(
		"📚 新刊予定があります: %s\n📕 紙書籍(%.0f円): %s\n📱 電子書籍(%.0f円): %s",
		kindle.ItemInfo.Title.DisplayValue,
		(*paper.Offers.Listings)[0].Price.Amount,
		paper.DetailPageURL,
		(*kindle.Offers.Listings)[0].Price.Amount,
		kindle.DetailPageURL,
	)
}

func isSameKindleBook(paper, kindle entity.Item) bool {
	if paper.ASIN == kindle.ASIN {
		return false
	}
	if kindle.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return false
	}
	if paper.ItemInfo.ProductInfo.ReleaseDate == nil {
		return false
	}
	if kindle.ItemInfo.ProductInfo.ReleaseDate == nil {
		return false
	}
	return paper.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02") ==
		kindle.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02")
}