package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
)

var (
	paapiMaxRetryCount = 5
	cycleDays          = 1
)

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	initEnvironmentVariables()

	books, index, err := getBookToProcess(cfg)
	if err != nil {
		return err
	}
	if books == nil {
		return nil
	}

	if err = utils.PutS3Object(cfg, strconv.Itoa(index), utils.EnvConfig.S3PrevIndexPaperToKindleObjectKey); err != nil {
		return err
	}

	if err = processCore(cfg, books, index); err != nil {
		return err
	}

	utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "SlotSuccess")

	return nil
}

func initEnvironmentVariables() {
	if envRetryCount := os.Getenv("PAPER_TO_KINDLE_PAAPI_RETRY_COUNT"); envRetryCount != "" {
		if count, err := strconv.Atoi(envRetryCount); err == nil && count > 0 {
			paapiMaxRetryCount = count
		}
	}

	if envDays := os.Getenv("PAPER_TO_KINDLE_CYCLE_DAYS"); envDays != "" {
		if days, err := strconv.Atoi(envDays); err == nil && days > 0 {
			cycleDays = days
		}
	}
}

func getBookToProcess(cfg aws.Config) ([]utils.KindleBook, int, error) {
	books, err := fetchPaperBooks(cfg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch paper books: %w", err)
	}
	if len(books) == 0 {
		return nil, 0, fmt.Errorf("no paper books available")
	}

	index := utils.GetIndexByTime(len(books), cycleDays)

	prevIndexBytes, err := utils.GetS3Object(cfg, utils.EnvConfig.S3PrevIndexPaperToKindleObjectKey)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch prev_index: %w", err)
	}
	prevIndex, _ := strconv.Atoi(string(prevIndexBytes))

	if prevIndex == index {
		log.Printf("Not my slot, skipping (%03d / %03d)", index+1, len(books))
		return nil, 0, nil
	}

	log.Printf("%03d / %03d: %s", index+1, len(books), books[index].Title)
	return books, index, nil
}

func fetchPaperBooks(cfg aws.Config) ([]utils.KindleBook, error) {
	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3PaperBooksObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch paper books: %w", err)
	}
	var books []utils.KindleBook
	if err := json.Unmarshal(body, &books); err != nil {
		return nil, err
	}
	return books, nil
}

func processCore(cfg aws.Config, books []utils.KindleBook, index int) error {
	client := utils.CreateClient()
	book := &books[index]

	if book.Title == "" {
		items, err := utils.GetItems(cfg, client, []string{book.ASIN})
		if err != nil {
			utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
			return formatProcessError("getItems", index, books, book, err)
		}

		utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")
		if len(items.ItemsResult.Items) == 0 {
			log.Printf("No item found for ASIN: %s", book.ASIN)
			return nil
		}

		*book = utils.MakeBook(items.ItemsResult.Items[0], 0)
	}

	kindleItem, err := searchKindleEdition(cfg, client, *book)
	if err != nil {
		utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
		return formatProcessError("searchKindleEdition", index, books, book, err)
	}
	utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

	var newUnprocessed []utils.KindleBook
	var updatedBooks []utils.KindleBook

	for i, b := range books {
		if i != index {
			updatedBooks = append(updatedBooks, b)
		}
	}

	if kindleItem != nil {
		utils.LogAndNotify(formatSlackMessage(*book, *kindleItem), true)
		newUnprocessed = append(newUnprocessed, utils.MakeBook(*kindleItem, 0))
	} else {
		updatedBooks = append(updatedBooks, *book)
	}

	if err := updateASINs(cfg, newUnprocessed); err != nil {
		return err
	}

	utils.SortByReleaseDate(updatedBooks)
	if !reflect.DeepEqual(books, updatedBooks) {
		if err := utils.SaveASINs(cfg, updatedBooks, utils.EnvConfig.S3PaperBooksObjectKey); err != nil {
			return fmt.Errorf("failed to save paper books: %w", err)
		}
	}

	return nil
}

func formatProcessError(operation string, index int, books []utils.KindleBook, book *utils.KindleBook, err error) error {
	return fmt.Errorf(
		"%s: %03d / %03d\nhttps://www.amazon.co.jp/dp/%s\n%v",
		operation,
		index+1,
		len(books),
		book.ASIN,
		err,
	)
}

func searchKindleEdition(cfg aws.Config, client paapi5.Client, paper utils.KindleBook) (*entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Title,
		cleanTitle(paper.Title),
		paper.CurrentPrice+20000,
	)

	res, err := utils.SearchItems(cfg, client, q, paapiMaxRetryCount)
	if err != nil {
		return nil, err
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

func cleanTitle(title string) string {
	return strings.TrimSpace(regexp.MustCompile(`[\(\)（）【】〔〕]|\s*[0-9]+.*$`).ReplaceAllString(title, ""))
}

func isSameKindleBook(paper utils.KindleBook, kindle entity.Item) bool {
	if paper.ASIN == kindle.ASIN {
		return false
	}
	if kindle.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return false
	}
	if kindle.ItemInfo.ProductInfo.ReleaseDate == nil {
		return false
	}
	return paper.ReleaseDate.Format("2006-01-02") ==
		kindle.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02")
}

func formatSlackMessage(paper utils.KindleBook, kindle entity.Item) string {
	return fmt.Sprintf(
		"📚 新刊予定があります: %s\n📕 紙書籍(%.0f円): %s\n📱 電子書籍(%.0f円): %s",
		kindle.ItemInfo.Title.DisplayValue,
		paper.CurrentPrice,
		paper.URL,
		(*kindle.Offers.Listings)[0].Price.Amount,
		kindle.DetailPageURL,
	)
}

func updateASINs(cfg aws.Config, newItems []utils.KindleBook) error {
	if len(newItems) == 0 {
		return nil
	}

	currentUnprocessed, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch unprocessed ASINs: %w", err)
	}

	allUnprocessed := append(currentUnprocessed, newItems...)
	utils.SortByReleaseDate(allUnprocessed)

	if err := utils.SaveASINs(cfg, allUnprocessed, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save unprocessed ASINs: %w", err)
	}

	currentNotified, err := utils.FetchASINs(cfg, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch notified ASINs: %w", err)
	}

	allNotified := append(currentNotified, newItems...)
	utils.SortByReleaseDate(allNotified)

	if err := utils.SaveASINs(cfg, allNotified, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
		return fmt.Errorf("failed to save notified ASINs: %w", err)
	}
	return nil
}
