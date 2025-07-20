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

	"kindle_bot/utils"
)

const (
	// Error messages
	errFetchPaperBooks  = "error fetching paper books ASINs"
	errSavePaperBooks   = "error saving paper books ASINs"
	errFetchUnprocessed = "error fetching unprocessed ASINs"
	errSaveUnprocessed  = "error saving unprocessed ASINs"
	errFetchNotified    = "error fetching notified ASINs"
	errSaveNotified     = "error saving notified ASINs"
	errSearchingItems   = "error searching items"
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
	originalPaperBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3PaperBooksObjectKey)
	if err != nil {
		return fmt.Errorf("%s: %w", errFetchPaperBooks, err)
	}

	var newUnprocessed, newPaperBooks []utils.KindleBook
	for _, chunk := range utils.ChunkedASINs(utils.UniqueASINs(originalPaperBooks), 10) {
		items, err := utils.GetItems(cfg, client, chunk)
		if err != nil {
			newPaperBooks = append(newPaperBooks, utils.AppendFallbackBooks(chunk, originalPaperBooks)...)
			utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
			// utils.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}
		utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

		for _, paper := range items.ItemsResult.Items {
			log.Println(paper.ItemInfo.Title.DisplayValue)

			kindleItem, err := searchKindleEdition(cfg, client, paper)
			if err != nil {
				utils.AlertToSlack(err, false)
				newPaperBooks = append(newPaperBooks, utils.MakeBook(paper, 0))
				utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
				continue
			}
			utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

			if kindleItem != nil {
				utils.LogAndNotify(formatSlackMessage(paper, *kindleItem), true)
				newUnprocessed = append(newUnprocessed, utils.MakeBook(*kindleItem, 0))
			} else {
				newPaperBooks = append(newPaperBooks, utils.MakeBook(paper, 0))
			}
		}
	}

	if err := updateASINs(cfg, newUnprocessed); err != nil {
		return err
	}

	utils.SortByReleaseDate(newPaperBooks)
	if !reflect.DeepEqual(originalPaperBooks, newPaperBooks) {
		if err := utils.SaveASINs(cfg, newPaperBooks, utils.EnvConfig.S3PaperBooksObjectKey); err != nil {
			return fmt.Errorf("%s: %w", errSavePaperBooks, err)
		}
	}

	return nil
}

func searchKindleEdition(cfg aws.Config, client paapi5.Client, paper entity.Item) (*entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Title,
		cleanTitle(paper.ItemInfo.Title.DisplayValue),
		(*paper.Offers.Listings)[0].Price.Amount+20000,
	)

	res, err := utils.SearchItems(cfg, client, q, 5)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errSearchingItems, err)
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

func updateASINs(cfg aws.Config, newItems []utils.KindleBook) error {
	if len(newItems) == 0 {
		return nil
	}

	currentUnprocessed, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("%s: %w", errFetchUnprocessed, err)
	}

	allUnprocessed := append(currentUnprocessed, newItems...)
	utils.SortByReleaseDate(allUnprocessed)

	if err := utils.SaveASINs(cfg, allUnprocessed, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("%s: %w", errSaveUnprocessed, err)
	}

	currentNotified, err := utils.FetchASINs(cfg, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return fmt.Errorf("%s: %w", errFetchNotified, err)
	}

	allNotified := append(currentNotified, newItems...)
	utils.SortByReleaseDate(allNotified)

	if err := utils.SaveASINs(cfg, allNotified, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
		return fmt.Errorf("%s: %w", errSaveNotified, err)
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
