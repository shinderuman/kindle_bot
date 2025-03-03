package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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
	return "Processing complete: new_release_checker.go", process()
}

func process() error {
	sess, err := utils.InitSession()
	if err != nil {
		return err
	}

	client := utils.CreateClient()

	now := time.Now()

	newlyNotifiedASINs := []utils.KindleBook{}
	notifiedASINs, err := utils.FetchASINs(sess, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching notified ASINs: %v", err)
	}

	ongoingASINs, err := utils.FetchASINs(sess, utils.EnvConfig.S3OngoingObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching ongoing ASINs: %v", err)
	}

	notifiedMap := make(map[string]struct{})
	for _, n := range notifiedASINs {
		notifiedMap[n.ASIN] = struct{}{}
	}

	updated := false
	ongoingASINs = utils.UniqueASINs(ongoingASINs)
	for i := range ongoingASINs {
		book := &ongoingASINs[i]
		log.Println(book.Title)

		q := query.NewSearchItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
			Search(query.Title, book.Title).
			Search(query.Keywords, "Kindle版").
			Search(query.SearchIndex, "KindleStore").
			Search(query.SortBy, "NewestArrivals").
			EnableItemInfo().
			EnableOffers()

		res, err := utils.SearchItems(client, q)
		if err != nil {
			// utils.AlertToSlack(fmt.Errorf("Error search items: %v", err), false)
			continue
		}

		if res.SearchResult != nil {
			for _, i := range res.SearchResult.Items {
				if book.ReleaseDate.Before(i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Time) {
					book.ReleaseDate = i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue
					updated = true
				}

				if !shouldNotify(i, notifiedMap, now) {
					continue
				}

				message := fmt.Sprintf("新刊予定があります: %s\n発売日: %s\n%s",
					i.ItemInfo.Title.DisplayValue,
					i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02"),
					i.DetailPageURL,
				)
				log.Println(message)

				if err := utils.PostToSlack(message); err != nil {
					utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), true)
					continue
				}

				newlyNotifiedASINs = append(newlyNotifiedASINs, utils.KindleBook{
					ASIN:         i.ASIN,
					Title:        i.ItemInfo.Title.DisplayValue,
					ReleaseDate:  i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue,
					CurrentPrice: (*i.Offers.Listings)[0].Price.Amount,
					MaxPrice:     (*i.Offers.Listings)[0].Price.Amount,
					URL:          i.DetailPageURL,
				})
				notifiedMap[i.ASIN] = struct{}{}
			}
		} else {
			message := fmt.Sprintf("検索結果が見つかりませんでした: %s\n%s", book.Title, book.URL)
			log.Println(message)

			if err := utils.PostToSlack(message); err != nil {
				utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), true)
			}
		}
	}

	if len(newlyNotifiedASINs) > 0 {
		notifiedASINs = append(notifiedASINs, newlyNotifiedASINs...)
		utils.SortByReleaseDate(notifiedASINs)
		if err := utils.SaveASINs(sess, notifiedASINs, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
			return fmt.Errorf("Error saving unprocessed ASINs: %v", err)
		}
	}

	if updated {
		utils.SortByReleaseDate(ongoingASINs)
		if err := utils.SaveASINs(sess, ongoingASINs, utils.EnvConfig.S3OngoingObjectKey); err != nil {
			return fmt.Errorf("Error saving updated ongoing ASINs: %v", err)
		}
		if err := utils.UpdateGist(ongoingASINs, "新刊チェック中の本.md"); err != nil {
			return fmt.Errorf("Error update gist: %s", err)
		}
	}

	return nil
}

func shouldNotify(item entity.Item, notifiedMap map[string]struct{}, now time.Time) bool {
	if _, exists := notifiedMap[item.ASIN]; exists {
		return false
	}
	if item.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Before(now) {
		return false
	}
	if strings.Contains(item.ItemInfo.Title.DisplayValue, "分冊") {
		return false
	}
	return true
}
