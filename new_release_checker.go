package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	paapi5 "github.com/goark/pa-api"
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
			utils.AlertToSlack(err)
		}
	}
}

func handler(ctx context.Context) (string, error) {
	return "Processing complete: new_release_checker.go", process()
}

func process() error {
	start := time.Now()
	client := utils.CreateClient()

	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	upcomingMap := make(map[string]utils.KindleBook)
	notifiedMap, err := fetchNotifiedASINs(cfg, start)
	if err != nil {
		return err
	}

	authors, err := fetchAuthors(cfg)
	if err != nil {
		return err
	}

	for i, author := range authors {
		log.Printf("%04d / %04d %s\n", i+1, len(authors), author.Name)

		items, err := searchAuthorBooks(client, author.Name)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			utils.LogAndNotify(fmt.Sprintf("検索結果が見つかりませんでした: %s\n%s", author.Name, author.URL))
			continue
		}

		for _, i := range items {
			if shouldSkip(i, author, notifiedMap, start) {
				continue
			}

			utils.LogAndNotify(fmt.Sprintf("新刊予定があります: %s\n作者: %s\n発売日: %s\nASIN: %s\n%s",
				i.ItemInfo.Title.DisplayValue,
				author.Name,
				i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02"),
				i.ASIN,
				i.DetailPageURL,
			))

			b := utils.MakeBook(i, 0)
			notifiedMap[i.ASIN] = b
			upcomingMap[i.ASIN] = b
		}
	}

	if len(upcomingMap) > 0 {
		if err := saveASINs(cfg, notifiedMap, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
			return err
		}
		if err := saveASINs(cfg, upcomingMap, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
			return err
		}
	}

	log.Printf("処理時間: %.2f 分\n", time.Since(start).Minutes())
	return nil
}

func fetchNotifiedASINs(cfg aws.Config, now time.Time) (map[string]utils.KindleBook, error) {
	books, err := utils.FetchASINs(cfg, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return nil, fmt.Errorf("Error fetching notified ASINs: %v", err)
	}
	m := make(map[string]utils.KindleBook)
	for _, b := range books {
		if b.ReleaseDate.After(now) {
			m[b.ASIN] = b
		}
	}
	return m, nil
}

func fetchAuthors(cfg aws.Config) ([]Author, error) {
	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3AuthorsObjectKey)
	if err != nil {
		return nil, fmt.Errorf("Error fetching authors file: %v", err)
	}
	var authors []Author
	if err := json.Unmarshal(body, &authors); err != nil {
		return nil, err
	}
	return authors, nil
}

func searchAuthorBooks(client paapi5.Client, authorName string) ([]entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Author,
		authorName,
		0,
	)

	res, err := utils.SearchItems(client, q)
	if err != nil {
		return nil, fmt.Errorf("Error search items: %v", err)
	}

	if res.SearchResult == nil {
		return nil, nil
	}

	return res.SearchResult.Items, nil
}

func shouldSkip(i entity.Item, author Author, notifiedMap map[string]utils.KindleBook, now time.Time) bool {
	if _, exists := notifiedMap[i.ASIN]; exists {
		return true
	}
	if i.ItemInfo.ProductInfo.ReleaseDate == nil {
		return true
	}
	if i.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return true
	}
	for _, s := range []string{
		"分冊版",
		"連載版",
		"単話版",
		"雑誌",
		"アンソロジー",
		"話売り",
	} {
		if strings.Contains(i.ItemInfo.Title.DisplayValue, s) {
			return true
		}
	}
	if regexp.MustCompile(`\d{4}年\d{1,2}月`).MatchString(i.ItemInfo.Title.DisplayValue) {
		return true
	}
	if i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Before(now) {
		return true
	}
	if !isNameMatched(author, i) {
		return true
	}
	return false
}

func normalizeName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		// 全角英数字: FF01(！) ～ FF5E(～)
		if r >= '！' && r <= '～' {
			r = rune(r - 0xFEE0)
		}
		// 全角スペース: U+3000
		if r == '　' {
			r = ' '
		}
		builder.WriteRune(r)
	}

	normalized := strings.ReplaceAll(builder.String(), " ", "")
	return strings.TrimSpace(normalized)
}

func isNameMatched(author Author, i entity.Item) bool {
	authorName := normalizeName(author.Name)
	for _, c := range i.ItemInfo.ByLineInfo.Contributors {
		if strings.Contains(authorName, normalizeName(c.Name)) {
			return true
		}
	}
	return false
}

func saveASINs(cfg aws.Config, m map[string]utils.KindleBook, key string) error {
	var list []utils.KindleBook
	for _, book := range m {
		list = append(list, book)
	}
	utils.SortByReleaseDate(list)
	return utils.SaveASINs(cfg, list, key)
}
