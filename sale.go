package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"reflect"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"

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
		if err := process(); err != nil {
			log.Println(err)
			utils.AlertToSlack(err)
		}
	}
}

func handler(ctx context.Context) (string, error) {
	return utils.Handler(ctx, process)
}

func process() error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(utils.EnvConfig.S3Region),
	})
	if err != nil {
		return fmt.Errorf("AWS session error: %v", err)
	}

	client := paapi5.New(
		paapi5.WithMarketplace(paapi5.LocaleJapan),
	).CreateClient(utils.EnvConfig.AmazonPartnerTag, utils.EnvConfig.AmazonAccessKey, utils.EnvConfig.AmazonSecretKey, paapi5.WithHttpClient(&http.Client{}))

	newUnprocessedASINs := []utils.KindleBook{}
	unprocessedASINs, err := utils.FetchASINs(sess, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("Error fetching unprocessed ASINs: %v", err)
	}

	for _, asinChunk := range utils.ChunkedASINs(utils.UniqueASINs(unprocessedASINs), 10) {
		response, err := utils.GetItems(client, asinChunk)
		if err != nil {
			utils.AlertToSlack(fmt.Errorf("Error fetching item details: %v", err))
			continue
		}
		for _, item := range response.ItemsResult.Items {
			if item.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
				utils.AlertToSlack(fmt.Errorf(
					"The item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
					item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
				))
				continue
			}

			maxPrice := math.Max(getMaxPrice(item, unprocessedASINs), (*item.Offers.Listings)[0].Price.Amount)
			ok, conditions := checkConditions(item, maxPrice)
			if ok {
				message := fmt.Sprintf("📚 %s\n条件達成: %s\n%s", item.ItemInfo.Title.DisplayValue, conditions, item.DetailPageURL)
				log.Println(message)

				status, err := utils.TootMastodon(message)
				if err != nil {
					utils.AlertToSlack(fmt.Errorf("Failed to post to Mastodon: %v", err))
				}
				if err = utils.PostToSlack(fmt.Sprintf("%s\n\n%s", message, status.URI)); err != nil {
					utils.AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err))
				}
			} else {
				newUnprocessedASINs = append(newUnprocessedASINs, utils.KindleBook{
					ASIN:         item.ASIN,
					Title:        item.ItemInfo.Title.DisplayValue,
					ReleaseDate:  item.ItemInfo.ProductInfo.ReleaseDate.DisplayValue,
					CurrentPrice: (*item.Offers.Listings)[0].Price.Amount,
					MaxPrice:     maxPrice,
					URL:          item.DetailPageURL,
				})
			}
		}
	}

	utils.SortByReleaseDate(newUnprocessedASINs)
	if !reflect.DeepEqual(unprocessedASINs, newUnprocessedASINs) {
		if err := utils.SaveASINs(sess, newUnprocessedASINs, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
			return fmt.Errorf("Error saving unprocessed ASINs ObjectKey: %s\nError: %v", err)
		}
		if err := utils.UpdateGist(newUnprocessedASINs); err != nil {
			return fmt.Errorf("Error update gist: %s", err)
		}
	}

	return nil
}

func checkConditions(item entity.Item, maxPrice float64) (bool, string) {
	amount := (*item.Offers.Listings)[0].Price.Amount
	points := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string

	// 最大の値段より 151円以上安い
	priceDrop := maxPrice - amount
	if priceDrop >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅価格差 %.0f円", priceDrop))
	}

	// ポイント 151pt以上
	if points >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅ポイント %dpt", points))
	}

	// ポイント還元 20%以上
	pointPercentage := float64(points) / float64(amount) * 100
	if pointPercentage >= 20 {
		conditions = append(conditions, fmt.Sprintf("✅ポイント還元 %.1f%%", pointPercentage))
	}

	if len(conditions) > 0 {
		return true, strings.Join(conditions, " ")
	}
	return false, ""
}

func getMaxPrice(item entity.Item, slice []utils.KindleBook) float64 {
	for _, s := range slice {
		if item.ASIN == s.ASIN {
			return s.MaxPrice
		}
	}
	return 0
}
