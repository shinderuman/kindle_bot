package main

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
)

var (
	titleCleanRegex = regexp.MustCompile(`[\(\)（）【】〔〕：:]|\s*[0-9０-９]`)
)

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	checkerConfigs, err := utils.FetchCheckerConfigs(cfg)
	if err != nil {
		return fmt.Errorf("failed to fetch checker configs: %w", err)
	}

	if !checkerConfigs.PaperToKindleChecker.Enabled && utils.IsLambda() {
		log.Printf("PaperToKindleChecker is disabled, skipping execution")
		return nil
	}

	books, index, err := getBookToProcess(cfg, checkerConfigs)
	if err != nil {
		return err
	}
	if books == nil {
		return nil
	}

	if err = utils.PutS3Object(cfg, strconv.Itoa(index), utils.EnvConfig.S3PrevIndexPaperToKindleObjectKey); err != nil {
		return err
	}

	if err = processCore(cfg, books, index, checkerConfigs); err != nil {
		return err
	}

	utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "SlotSuccess")

	return nil
}

func getBookToProcess(cfg aws.Config, checkerConfigs *utils.CheckerConfigs) ([]utils.KindleBook, int, error) {
	books, err := fetchPaperBooks(cfg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch paper books: %w", err)
	}

	index, shouldProcess, nextExecutionTime, err := utils.ProcessSlot(cfg, len(books), checkerConfigs.PaperToKindleChecker.CycleDays, utils.EnvConfig.S3PrevIndexPaperToKindleObjectKey)
	if err != nil {
		return nil, 0, err
	}
	if !shouldProcess {
		return nil, 0, nil
	}

	format := utils.GetCountFormat(len(books))
	log.Printf(fmt.Sprintf("Processing slot (%s / %s): %%s, next execution: %s", format, format, utils.FormatTimeJST(nextExecutionTime)), index+1, len(books), books[index].Title)
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

func processCore(cfg aws.Config, books []utils.KindleBook, index int, checkerConfigs *utils.CheckerConfigs) error {
	client := utils.CreateClient()
	book := &books[index]

	if book.CurrentPrice == 0 {
		items, err := utils.GetItems(cfg, client, []string{book.ASIN}, checkerConfigs.PaperToKindleChecker.GetItemsInitialRetrySeconds, checkerConfigs.PaperToKindleChecker.GetItemsPaapiRetryCount)
		if err != nil {
			utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
			return formatProcessError("getItems", index, books, err)
		}

		utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")
		if len(items.ItemsResult.Items) == 0 {
			log.Printf("No item found for ASIN: %s", book.ASIN)
			return nil
		}

		item := items.ItemsResult.Items[0]
		if !isComic(item) {
			return fmt.Errorf(strings.TrimSpace(`
the item category is not a コミック.
ASIN: %s
Title: %s
Category: %s
URL: %s`),
				item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
			)
		}

		*book = utils.MakeBook(item, 0)
		if err := savePaperBooksAndUpdateGist(cfg, books, checkerConfigs); err != nil {
			return err
		}
	}

	kindleItem, err := searchKindleEdition(cfg, client, *book, checkerConfigs)
	if err != nil {
		utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APIFailure")
		return formatProcessError("searchKindleEdition", index, books, err)
	}
	utils.PutMetric(cfg, "KindleBot/PaperToKindleChecker", "APISuccess")

	if kindleItem != nil {
		utils.LogAndNotify(formatSlackMessage(*book, *kindleItem), true)

		notifiedMap, err := utils.FetchNotifiedASINs(cfg, time.Now())
		if err != nil {
			return err
		}

		upcomingMap := make(map[string]utils.KindleBook)
		b := utils.MakeBook(*kindleItem, book.MaxPrice)
		notifiedMap[kindleItem.ASIN] = b
		upcomingMap[kindleItem.ASIN] = b

		if err := utils.SaveNotifiedASINs(cfg, notifiedMap); err != nil {
			return err
		}

		if err := utils.SaveUpcomingASINs(cfg, upcomingMap); err != nil {
			return err
		}

		var updatedBooks []utils.KindleBook
		for i, b := range books {
			if i != index {
				updatedBooks = append(updatedBooks, b)
			}
		}

		if err := savePaperBooksAndUpdateGist(cfg, updatedBooks, checkerConfigs); err != nil {
			return err
		}
	}

	return nil
}

func formatProcessError(operation string, index int, books []utils.KindleBook, err error) error {
	return fmt.Errorf(strings.TrimSpace(`
%s: %03d / %03d
https://www.amazon.co.jp/dp/%s
%v`),
		operation,
		index+1,
		len(books),
		books[index].ASIN,
		err,
	)
}

func isComic(item entity.Item) bool {
	binding := item.ItemInfo.Classifications.Binding.DisplayValue
	return binding == "コミック" || binding == "単行本" || binding == "ペーパーバック"
}

func savePaperBooksAndUpdateGist(cfg aws.Config, books []utils.KindleBook, checkerConfigs *utils.CheckerConfigs) error {
	books = utils.UniqueASINs(books)
	utils.SortByReleaseDate(books)
	if err := savePaperBooks(cfg, books); err != nil {
		return err
	}

	if err := utils.UpdateBookGist(checkerConfigs.PaperToKindleChecker.GistID, checkerConfigs.PaperToKindleChecker.GistFilename, books); err != nil {
		return fmt.Errorf("failed to update gist: %w", err)
	}

	return nil
}

func formatSlackMessage(paper utils.KindleBook, kindle entity.Item) string {
	return fmt.Sprintf(strings.TrimSpace(`
📚 新刊予定があります: %s
📕 紙書籍(%.0f円): %s
📱 電子書籍(%.0f円): %s`),
		kindle.ItemInfo.Title.DisplayValue,
		paper.CurrentPrice,
		paper.URL,
		(*kindle.Offers.Listings)[0].Price.Amount,
		kindle.DetailPageURL,
	)
}

func searchKindleEdition(cfg aws.Config, client paapi5.Client, paper utils.KindleBook, checkerConfigs *utils.CheckerConfigs) (*entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Title,
		cleanTitle(paper.Title),
		paper.CurrentPrice+20000,
	)

	res, err := utils.SearchItems(cfg, client, q, checkerConfigs.PaperToKindleChecker.SearchItemsPaapiRetryCount, checkerConfigs.PaperToKindleChecker.SearchItemsInitialRetrySeconds)
	if err != nil {
		return nil, err
	}

	if res.SearchResult == nil || len(res.SearchResult.Items) == 0 {
		return nil, fmt.Errorf("no search results found for title: %s", paper.Title)
	}

	for _, kindle := range res.SearchResult.Items {
		if isSameKindleBook(paper, kindle) {
			return &kindle, nil
		}
	}
	return nil, nil
}

func savePaperBooks(cfg aws.Config, books []utils.KindleBook) error {
	if err := utils.SaveASINs(cfg, books, utils.EnvConfig.S3PaperBooksObjectKey); err != nil {
		return fmt.Errorf("failed to save paper books: %w", err)
	}
	return nil
}

func cleanTitle(title string) string {
	return strings.TrimSpace(titleCleanRegex.Split(title, 2)[0])
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
