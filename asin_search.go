package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
)

func main() {
	if err := utils.InitConfig(); err != nil {
		log.Println("Error loading configuration:", err)
		return
	}

	if utils.IsLambda() {
		lambda.Start(handler)
	} else {
		if _, err := handler(context.Background()); err != nil {
			utils.AlertToSlack(err, true)
		}
	}
}

func handler(ctx context.Context) (string, error) {
	return "Processing complete: asin_search.go", process()
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	client := utils.CreateClient()

	paperBooksASINs, err := utils.FetchASINs(cfg, utils.EnvConfig.S3PaperBooksObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching paper books ASINs: %v", err)
	}

	newUnprocessedASINs := []utils.KindleBook{}
	newPaperBooksASINs := []utils.KindleBook{}
	for _, asinChunk := range utils.ChunkedASINs(utils.UniqueASINs(paperBooksASINs), 10) {
		res, err := utils.GetItems(client, asinChunk)
		if err != nil {
			for _, asin := range asinChunk {
				newPaperBooksASINs = append(newPaperBooksASINs, utils.GetBook(asin, paperBooksASINs))
			}
			// utils.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err), false)
			continue
		}

		for _, i := range res.ItemsResult.Items {
			if i.ItemInfo.Classifications.Binding.DisplayValue != "コミック" {
				utils.AlertToSlack(fmt.Errorf(
					"The item category is not a コミック.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
					i.ASIN, i.ItemInfo.Title.DisplayValue, i.ItemInfo.Classifications.Binding.DisplayValue, i.DetailPageURL,
				), true)
				continue
			}

			q := query.NewSearchItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
				Search(query.Title, i.ItemInfo.Title.DisplayValue).
				Search(query.SearchIndex, "KindleStore").
				Search(query.Keywords, "Kindle版").
				EnableItemInfo().
				EnableOffers()
			res, err := utils.SearchItems(client, q)
			if err != nil {
				utils.AlertToSlack(fmt.Errorf("Error search items: %v", err), true)
				continue
			}

			foundKindle := false
			if res.SearchResult != nil {
				for _, j := range res.SearchResult.Items {
					if isSameKindleBook(i, j) {
						message := fmt.Sprintf("📚 %s\n📕 紙書籍(%.0f円): %s\n📱 電子書籍(%.0f円): %s", j.ItemInfo.Title.DisplayValue, (*i.Offers.Listings)[0].Price.Amount, i.DetailPageURL, (*j.Offers.Listings)[0].Price.Amount, j.DetailPageURL)
						log.Println(message)
						if err = utils.PostToSlack(message); err != nil {
							utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), true)
						}

						newUnprocessedASINs = append(newUnprocessedASINs, utils.KindleBook{
							ASIN:         j.ASIN,
							Title:        j.ItemInfo.Title.DisplayValue,
							ReleaseDate:  j.ItemInfo.ProductInfo.ReleaseDate.DisplayValue,
							CurrentPrice: (*j.Offers.Listings)[0].Price.Amount,
							MaxPrice:     (*j.Offers.Listings)[0].Price.Amount,
							URL:          j.DetailPageURL,
						})
						foundKindle = true
					}
				}
			}
			if !foundKindle {
				newPaperBooksASINs = append(newPaperBooksASINs, utils.KindleBook{
					ASIN:         i.ASIN,
					Title:        i.ItemInfo.Title.DisplayValue,
					ReleaseDate:  i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue,
					CurrentPrice: (*i.Offers.Listings)[0].Price.Amount,
					MaxPrice:     (*i.Offers.Listings)[0].Price.Amount,
					URL:          i.DetailPageURL,
				})
			}
		}
	}

	if len(newUnprocessedASINs) > 0 {
		unprocessedASINs, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
		if err != nil {
			return fmt.Errorf("Error fetching unprocessed ASINs: %v", err)
		}

		unprocessedASINs = append(unprocessedASINs, newUnprocessedASINs...)
		utils.SortByReleaseDate(unprocessedASINs)

		if err := utils.SaveASINs(cfg, unprocessedASINs, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
			return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
		}
	}

	utils.SortByReleaseDate(newPaperBooksASINs)
	if !reflect.DeepEqual(paperBooksASINs, newPaperBooksASINs) {
		if err := utils.SaveASINs(cfg, newPaperBooksASINs, utils.EnvConfig.S3PaperBooksObjectKey); err != nil {
			return fmt.Errorf("Error saving paper books ASINs: %v", err)
		}
	}

	return nil
}

func cleanTitle(title string) string {
	return strings.TrimSpace(regexp.MustCompile(`[\(\)（）【】〔〕]|\s*[0-9]+.*$`).ReplaceAllString(title, ""))
}

func isSameKindleBook(paperBook, kindleBook entity.Item) bool {
	if paperBook.ASIN == kindleBook.ASIN {
		return false
	}
	if kindleBook.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return false
	}
	if kindleBook.ItemInfo.ProductInfo.ReleaseDate == nil {
		return false
	}
	if paperBook.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02") != kindleBook.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02") {
		return false
	}
	// 紙の値段より200円以上高い商品は除外する（特典付き特装版の可能性）
	if (*paperBook.Offers.Listings)[0].Price.Amount+200 <= (*kindleBook.Offers.Listings)[0].Price.Amount {
		return false
	}
	return true
}
