package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
)

type Author struct {
	Name string `json:"Name"`
	URL  string `json:"URL"`
}

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
	return "Processing complete: new_release_checker.go", process()
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	client := utils.CreateClient()

	now := time.Now()

	notifiedASINs, err := utils.FetchASINs(cfg, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching notified ASINs: %v", err)
	}

	notifiedMap := make(map[string]utils.KindleBook)
	for _, book := range notifiedASINs {
		if book.ReleaseDate.After(now) {
			notifiedMap[book.ASIN] = book
		}
	}

	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3AuthorsObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching authors file: %v", err)
	}

	var authors []Author
	if err := json.Unmarshal(body, &authors); err != nil {
		return err
	}

	start := time.Now()

	hasUpdate := false
	for i, author := range authors {
		log.Printf("%04d / %04d %s\n", i+1, len(authors), author.Name)

		q := query.NewSearchItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
			Search(query.Keywords, fmt.Sprintf("Kindle版 %s", author.Name)).
			Search(query.SearchIndex, "KindleStore").
			Search(query.SortBy, "NewestArrivals").
			EnableItemInfo().
			EnableOffers()

		res, err := utils.SearchItems(client, q)
		if err != nil {
			return fmt.Errorf("Error search items: %v", err)
		}

		if res.SearchResult != nil {
			for _, i := range res.SearchResult.Items {
				if shouldSkip(i, notifiedMap, now) {
					continue
				}

				message := fmt.Sprintf("新刊予定があります: %s\n作者: %s\n発売日: %s\n%s",
					i.ItemInfo.Title.DisplayValue,
					author.Name,
					i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02"),
					i.DetailPageURL,
				)
				log.Println(message)

				if err := utils.PostToSlack(message); err != nil {
					utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), true)
					continue
				}

				hasUpdate = true
				notifiedMap[i.ASIN] = utils.KindleBook{
					ASIN:         i.ASIN,
					Title:        i.ItemInfo.Title.DisplayValue,
					ReleaseDate:  i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue,
					CurrentPrice: (*i.Offers.Listings)[0].Price.Amount,
					MaxPrice:     (*i.Offers.Listings)[0].Price.Amount,
					URL:          i.DetailPageURL,
				}
			}
		} else {
			message := fmt.Sprintf("検索結果が見つかりませんでした: %s\n%s", author.Name, author.URL)
			log.Println(message)

			if err := utils.PostToSlack(message); err != nil {
				utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), true)
			}
		}
	}

	elapsed := time.Since(start)
	log.Printf("処理時間: %.2f 分\n", elapsed.Minutes())

	if hasUpdate {
		var finalASINs []utils.KindleBook
		for _, book := range notifiedMap {
			finalASINs = append(finalASINs, book)
		}
		utils.SortByReleaseDate(finalASINs)
		if err := utils.SaveASINs(cfg, finalASINs, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
			return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
		}
	}

	return nil
}

func shouldSkip(i entity.Item, notifiedMap map[string]utils.KindleBook, now time.Time) bool {
	if i.ItemInfo.ProductInfo.ReleaseDate == nil {
		return true
	}

	if _, exists := notifiedMap[i.ASIN]; exists {
		return true
	}

	if i.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return true
	}

	if strings.Contains(i.ItemInfo.Title.DisplayValue, "分冊版") {
		return true
	}

	if strings.Contains(i.ItemInfo.Title.DisplayValue, "連載版") {
		return true
	}

	if strings.Contains(i.ItemInfo.Title.DisplayValue, "アンソロジー") {
		return true
	}

	if i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Before(now) {
		return true
	}

	return false
}
