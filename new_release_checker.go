package main

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"

	"kindle_bot/utils"
)

type Author struct {
	Name              string    `json:"Name"`
	URL               string    `json:"URL"`
	LatestReleaseDate time.Time `json:"LatestReleaseDate"`
}

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	author, authors, index, err := getAuthorToProcess(cfg)
	if err != nil {
		return err
	}
	if author == nil {
		return nil
	}

	if err = processCore(cfg, author, authors); err != nil {
		return err
	}

	if err = utils.PutS3Object(cfg, strconv.Itoa(index), utils.EnvConfig.S3PrevIndexObjectKey); err != nil {
		return err
	}

	return nil
}

func processCore(cfg aws.Config, author *Author, authors []Author) error {
	start := time.Now()
	client := utils.CreateClient()

	notifiedMap, err := fetchNotifiedASINs(cfg, start)
	if err != nil {
		return err
	}

	ngWords, err := fetchExcludedTitleKeywords(cfg)
	if err != nil {
		return err
	}

	upcomingMap := make(map[string]utils.KindleBook)
	items, err := searchAuthorBooks(client, author.Name)
	if err != nil {
		utils.AlertToSlack(fmt.Errorf("Author: %s\n%s\nError search items: %v", author.Name, author.URL, err), false)
		return nil
	}

	if len(items) == 0 {
		utils.LogAndNotify(fmt.Sprintf("検索結果が見つかりませんでした: %s\n%s", author.Name, author.URL))
		return nil
	}

	latest := author.LatestReleaseDate
	for _, item := range items {
		if shouldSkip(item, author, notifiedMap, ngWords, start) {
			continue
		}

		utils.LogAndNotify(fmt.Sprintf("新刊予定があります: %s\n作者: %s\n発売日: %s\nASIN: %s\n%s",
			item.ItemInfo.Title.DisplayValue,
			author.Name,
			item.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02"),
			item.ASIN,
			item.DetailPageURL,
		))

		b := utils.MakeBook(item, 0)
		notifiedMap[item.ASIN] = b
		upcomingMap[item.ASIN] = b
	}

	if len(upcomingMap) > 0 {
		if err := saveASINs(cfg, notifiedMap, utils.EnvConfig.S3NotifiedObjectKey); err != nil {
			return err
		}

		books, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
		if err != nil {
			return err
		}
		for _, b := range books {
			upcomingMap[b.ASIN] = b
		}
		if err := saveASINs(cfg, upcomingMap, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
			return err
		}
	}

	if !author.LatestReleaseDate.Equal(latest) {
		if err := saveAuthors(cfg, authors); err != nil {
			return err
		}
	}

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

func getAuthorToProcess(cfg aws.Config) (*Author, []Author, int, error) {
	authors, err := fetchAuthors(cfg)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to fetch authors: %w", err)
	}
	if len(authors) == 0 {
		return nil, nil, 0, fmt.Errorf("no authors available")
	}

	index := getIndexByTime(len(authors))

	prevIndexBytes, err := utils.GetS3Object(cfg, utils.EnvConfig.S3PrevIndexObjectKey)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to fetch prev_index: %w", err)
	}
	prevIndex, _ := strconv.Atoi(string(prevIndexBytes))
	if prevIndex == index {
		log.Println("Not my slot, skipping")
		return nil, authors, index, nil
	}

	author := &authors[index]
	log.Printf("Author %04d / %04d: %s", index+1, len(authors), author.Name)
	return author, authors, index, nil
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

func getIndexByTime(authorCount int) int {
	sec := time.Now().Unix() % 86400
	interval := 86400 / authorCount
	return int(sec) / interval
}

func fetchExcludedTitleKeywords(cfg aws.Config) ([]string, error) {
	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3ExcludedTitleKeywordsObjectKey)
	if err != nil {
		return nil, fmt.Errorf("Error fetching excluded title keywords file: %v", err)
	}
	var keywords []string
	if err := json.Unmarshal(body, &keywords); err != nil {
		return nil, err
	}
	return keywords, nil
}

func searchAuthorBooks(client paapi5.Client, authorName string) ([]entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Author,
		authorName,
		0,
	)

	res, err := utils.SearchItems(client, q, 1)
	if err != nil {
		return nil, fmt.Errorf("Error search items: %v", err)
	}

	if res.SearchResult == nil {
		return nil, nil
	}

	return res.SearchResult.Items, nil
}

func shouldSkip(i entity.Item, author *Author, notifiedMap map[string]utils.KindleBook, ngWords []string, now time.Time) bool {
	if _, exists := notifiedMap[i.ASIN]; exists {
		return true
	}
	if i.ItemInfo.ProductInfo.ReleaseDate == nil {
		return true
	}
	if i.ItemInfo.Classifications.Binding.DisplayValue != "Kindle版" {
		return true
	}
	for _, s := range ngWords {
		if strings.Contains(i.ItemInfo.Title.DisplayValue, s) {
			return true
		}
	}
	if regexp.MustCompile(`\d{4}年\d{1,2}月`).MatchString(i.ItemInfo.Title.DisplayValue) {
		return true
	}
	if !isNameMatched(author, i) {
		return true
	}
	releaseDate := i.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Time

	// 発売日がこれまでの最大値より新しい場合、更新する
	if releaseDate.After(author.LatestReleaseDate) {
		author.LatestReleaseDate = releaseDate
	}

	if releaseDate.Before(now) {
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

func isNameMatched(author *Author, i entity.Item) bool {
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

func saveAuthors(cfg aws.Config, authors []Author) error {
	seen := make(map[string]bool)
	uniqueAuthors := make([]Author, 0, len(authors))

	for _, author := range authors {
		if !seen[author.Name] {
			seen[author.Name] = true
			uniqueAuthors = append(uniqueAuthors, author)
		}
	}

	sort.Slice(uniqueAuthors, func(i, j int) bool {
		if uniqueAuthors[i].LatestReleaseDate.After(uniqueAuthors[j].LatestReleaseDate) {
			return true
		}
		if uniqueAuthors[i].LatestReleaseDate.Before(uniqueAuthors[j].LatestReleaseDate) {
			return false
		}
		return uniqueAuthors[i].Name < uniqueAuthors[j].Name
	})

	prettyJSON, err := json.MarshalIndent(uniqueAuthors, "", "    ")
	if err != nil {
		return err
	}

	return utils.PutS3Object(cfg, strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"), utils.EnvConfig.S3AuthorsObjectKey)
}
