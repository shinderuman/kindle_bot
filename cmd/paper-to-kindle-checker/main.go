package main

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
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
	originalPaperBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3PaperBooksObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch paper books ASINs: %w", err)
	}

	newUnprocessed, newPaperBooks, hasAPIError := processPaperBooks(cfg, client, originalPaperBooks)

	if err := utils.UpdateASINsInMultipleFiles(cfg, newUnprocessed); err != nil {
		return fmt.Errorf("failed to update ASINs: %w", err)
	}

	if err := utils.SaveBooksIfChanged(cfg, originalPaperBooks, newPaperBooks, utils.EnvConfig.S3PaperBooksObjectKey); err != nil {
		return fmt.Errorf("failed to save paper books: %w", err)
	}

	if hasAPIError {
		return fmt.Errorf("PA API errors occurred during processing")
	}

	return nil
}

func processPaperBooks(cfg aws.Config, client paapi5.Client, originalBooks []utils.KindleBook) ([]utils.KindleBook, []utils.KindleBook, bool) {
	var newUnprocessed, newPaperBooks []utils.KindleBook
	var hasAPIError bool

	for _, chunk := range utils.ChunkedASINs(utils.UniqueASINs(originalBooks), utils.DefaultChunkSize) {
		items, err := utils.GetItems(cfg, client, chunk)
		if err != nil {
			newPaperBooks = append(newPaperBooks, utils.AppendFallbackBooks(chunk, originalBooks)...)
			utils.RecordAPIMetric(cfg, "KindleBot/PaperToKindleChecker", false)
			hasAPIError = true
			continue
		}
		utils.RecordAPIMetric(cfg, "KindleBot/PaperToKindleChecker", true)

		unprocessed, paperBooks, apiErr := processItemsForKindleEdition(cfg, client, items.ItemsResult.Items)
		newUnprocessed = append(newUnprocessed, unprocessed...)
		newPaperBooks = append(newPaperBooks, paperBooks...)
		if apiErr {
			hasAPIError = true
		}
	}

	return newUnprocessed, newPaperBooks, hasAPIError
}

func processItemsForKindleEdition(cfg aws.Config, client paapi5.Client, items []entity.Item) ([]utils.KindleBook, []utils.KindleBook, bool) {
	var newUnprocessed, newPaperBooks []utils.KindleBook
	var hasAPIError bool

	for _, paper := range items {
		log.Printf("Processing: %s", paper.ItemInfo.Title.DisplayValue)

		kindleItem, err := searchKindleEdition(cfg, client, paper)
		if err != nil {
			utils.AlertToSlack(err, false)
			newPaperBooks = append(newPaperBooks, utils.MakeBook(paper, 0))
			utils.RecordAPIMetric(cfg, "KindleBot/PaperToKindleChecker", false)
			hasAPIError = true
			continue
		}
		utils.RecordAPIMetric(cfg, "KindleBot/PaperToKindleChecker", true)

		if kindleItem != nil {
			utils.LogAndNotify(formatSlackMessage(paper, *kindleItem), true)
			newUnprocessed = append(newUnprocessed, utils.MakeBook(*kindleItem, 0))
		} else {
			newPaperBooks = append(newPaperBooks, utils.MakeBook(paper, 0))
		}
	}

	return newUnprocessed, newPaperBooks, hasAPIError
}

func searchKindleEdition(cfg aws.Config, client paapi5.Client, paper entity.Item) (*entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Title,
		utils.CleanTitle(paper.ItemInfo.Title.DisplayValue),
		(*paper.Offers.Listings)[0].Price.Amount+utils.DefaultPriceBuffer,
	)

	res, err := utils.SearchItems(cfg, client, q, utils.DefaultSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to search items: %w", err)
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
	if !utils.IsKindleEdition(kindle) {
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
