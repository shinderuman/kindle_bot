package paapi

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/goark/errs"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	appconfig "kindle_bot/internal/config"
	"kindle_bot/internal/notification"
	"kindle_bot/pkg/models"
)

func CreateClient() paapi5.Client {
	return paapi5.New(
		paapi5.WithMarketplace(paapi5.LocaleJapan),
	).CreateClient(
		appconfig.EnvConfig.AmazonPartnerTag,
		appconfig.EnvConfig.AmazonAccessKey,
		appconfig.EnvConfig.AmazonSecretKey,
		paapi5.WithHttpClient(&http.Client{}),
	)
}

func GetItems(cfg aws.Config, client paapi5.Client, asinChunk []string) (*entity.Response, error) {
	q := query.NewGetItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
		ASINs(asinChunk).
		EnableItemInfo().
		EnableOffers()

	body, err := requestWithBackoff(cfg, client, q, 5)
	if err != nil {
		return nil, fmt.Errorf("PA API request failed: %w", err)
	}

	res, err := entity.DecodeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("JSON decode error: %w", err)
	}

	return res, nil
}

func CreateSearchQuery(client paapi5.Client, searchKey query.RequestFilter, searchValue string, maxPrice float64) *query.SearchItems {
	q := query.NewSearchItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
		Search(searchKey, searchValue).
		Request(query.SearchIndex, "KindleStore").
		Request(query.SortBy, "NewestArrivals").
		Request(query.BrowseNodeID, "2293143051").
		Request(query.MinPrice, 22100).
		EnableItemInfo().
		EnableOffers()

	if maxPrice > 0 {
		q = q.Request(query.MaxPrice, maxPrice)
	}

	return q
}

func SearchItems(cfg aws.Config, client paapi5.Client, q *query.SearchItems, maxRetryCount int) (*entity.Response, error) {
	body, err := requestWithBackoff(cfg, client, q, maxRetryCount)
	if err != nil {
		return nil, fmt.Errorf("PA API request failed: %w", err)
	}

	res, err := entity.DecodeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("JSON decode error: %w", err)
	}

	return res, nil
}

func requestWithBackoff[T paapi5.Query](cfg aws.Config, client paapi5.Client, q T, maxRetryCount int) ([]byte, error) {
	const maxWait = 30 * time.Second
	for i := 0; i < maxRetryCount; i++ {
		body, err := client.Request(q)
		notification.PutMetric(cfg, "KindleBot/Usage", "PAAPIRequest")
		if err == nil {
			time.Sleep(time.Second * 2)
			return body, nil
		}

		if isRetryableError(err) {
			if i == maxRetryCount-1 {
				break
			}

			waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second * 2
			waitTime += time.Duration(rand.Intn(500)) * time.Millisecond

			if waitTime > maxWait {
				waitTime = maxWait
			}

			log.Printf("Rate limit hit. Retrying in %v...\n", waitTime)
			time.Sleep(waitTime)
			continue
		}

		return nil, err
	}

	log.Println("Max retries reached")
	return nil, fmt.Errorf("Max retries reached")
}

func isRetryableError(err error) bool {
	if findStatusCode(err) == 429 {
		return true
	}
	if strings.Contains(err.Error(), "EOF") {
		log.Println(err.Error())
		return true
	}
	return false
}

func findStatusCode(err error) int {
	for err != nil {
		e, ok := err.(*errs.Error)
		if !ok {
			break
		}

		if val, ok := e.Context["status"]; ok {
			return val.(int)
		}

		if e.Cause != nil {
			err = e.Cause
		} else {
			err = e.Err
		}
	}
	return 0
}

func MakeBook(item entity.Item, maxPrice float64) models.KindleBook {
	book := models.KindleBook{
		ASIN:         item.ASIN,
		Title:        item.ItemInfo.Title.DisplayValue,
		CurrentPrice: (*item.Offers.Listings)[0].Price.Amount,
		MaxPrice:     (*item.Offers.Listings)[0].Price.Amount,
		URL:          item.DetailPageURL,
	}

	if item.ItemInfo.ProductInfo.ReleaseDate != nil {
		book.ReleaseDate = item.ItemInfo.ProductInfo.ReleaseDate.DisplayValue
	}

	if maxPrice > 0 {
		book.MaxPrice = maxPrice
	}

	return book
}
