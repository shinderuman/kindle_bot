package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
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

const (
	gistID       = "d5116b8fdce5cdd1995c2a7a3be325f4"
	gistFilename = "新刊チェック中の作者.md"
)

var (
	paapiMaxRetryCount         = 3
	cycleDays          float64 = 7
	yearMonthRegex             = regexp.MustCompile(`\d{4}年\d{1,2}月`)
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

	initEnvironmentVariables()

	authors, index, err := getAuthorToProcess(cfg)
	if err != nil {
		return err
	}
	if authors == nil {
		return nil
	}

	if err = utils.PutS3Object(cfg, strconv.Itoa(index), utils.EnvConfig.S3PrevIndexNewReleaseObjectKey); err != nil {
		return err
	}

	if err = processCore(cfg, authors, index); err != nil {
		return err
	}

	utils.PutMetric(cfg, "KindleBot/NewReleaseChecker", "SlotSuccess")

	return nil
}

func initEnvironmentVariables() {
	if envRetryCount := os.Getenv("NEW_RELEASE_PAAPI_RETRY_COUNT"); envRetryCount != "" {
		if count, err := strconv.Atoi(envRetryCount); err == nil && count > 0 {
			paapiMaxRetryCount = count
		}
	}

	if envDays := os.Getenv("NEW_RELEASE_CYCLE_DAYS"); envDays != "" {
		if days, err := strconv.ParseFloat(envDays, 64); err == nil && days > 0 {
			cycleDays = days
		}
	}
}

func getAuthorToProcess(cfg aws.Config) ([]Author, int, error) {
	authors, err := fetchAuthors(cfg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch authors: %w", err)
	}

	index, shouldProcess, err := utils.ProcessSlot(cfg, len(authors), cycleDays, utils.EnvConfig.S3PrevIndexNewReleaseObjectKey)
	if err != nil {
		return nil, 0, err
	}
	if !shouldProcess {
		return nil, 0, nil
	}

	format := utils.GetCountFormat(len(authors))
	log.Printf(fmt.Sprintf("%s / %s: %%s", format, format), index+1, len(authors), authors[index].Name)
	return authors, index, nil
}

func fetchAuthors(cfg aws.Config) ([]Author, error) {
	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3AuthorsObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch authors: %w", err)
	}
	var authors []Author
	if err := json.Unmarshal(body, &authors); err != nil {
		return nil, err
	}
	return authors, nil
}

func processCore(cfg aws.Config, authors []Author, index int) error {
	start := time.Now()
	client := utils.CreateClient()
	author := &authors[index]

	notifiedMap, err := utils.FetchNotifiedASINs(cfg, start)
	if err != nil {
		return err
	}

	ngWords, err := fetchExcludedTitleKeywords(cfg)
	if err != nil {
		return err
	}

	upcomingMap := make(map[string]utils.KindleBook)
	items, err := searchAuthorBooks(cfg, client, author.Name)
	if err != nil {
		utils.PutMetric(cfg, "KindleBot/NewReleaseChecker", "SlotFailure")
		return formatProcessError(index, authors, err)
	}

	if len(items) == 0 {
		return formatProcessError(index, authors, errors.New("no search results found"))
	}

	latest := author.LatestReleaseDate
	for _, item := range items {
		if shouldSkip(item, author, notifiedMap, ngWords, start) {
			continue
		}

		utils.LogAndNotify(fmt.Sprintf("📚 新刊予定があります: %s\n作者: %s\n発売日: %s\nASIN: %s\n%s",
			item.ItemInfo.Title.DisplayValue,
			author.Name,
			item.ItemInfo.ProductInfo.ReleaseDate.DisplayValue.Format("2006-01-02"),
			item.ASIN,
			item.DetailPageURL,
		), true)

		b := utils.MakeBook(item, 0)
		notifiedMap[item.ASIN] = b
		upcomingMap[item.ASIN] = b
	}

	if err := utils.SaveNotifiedAndUpcomingASINs(cfg, notifiedMap, upcomingMap); err != nil {
		return err
	}

	if !author.LatestReleaseDate.Equal(latest) {
		authors = sortUniqueAuthors(authors)
		if err := saveAuthors(cfg, authors); err != nil {
			return err
		}
		if err := updateGist(authors); err != nil {
			return err
		}
	}

	return nil
}

func fetchExcludedTitleKeywords(cfg aws.Config) ([]string, error) {
	body, err := utils.GetS3Object(cfg, utils.EnvConfig.S3ExcludedTitleKeywordsObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch excluded keywords: %w", err)
	}
	var keywords []string
	if err := json.Unmarshal(body, &keywords); err != nil {
		return nil, err
	}
	return keywords, nil
}

func searchAuthorBooks(cfg aws.Config, client paapi5.Client, authorName string) ([]entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Author,
		authorName,
		0,
	)

	res, err := utils.SearchItems(cfg, client, q, paapiMaxRetryCount)
	if err != nil {
		return nil, err
	}

	if res.SearchResult == nil {
		return nil, nil
	}

	return res.SearchResult.Items, nil
}

func formatProcessError(index int, authors []Author, err error) error {
	return fmt.Errorf(
		"%04d / %04d: %s\n%s\n%v",
		index+1,
		len(authors),
		authors[index].Name,
		authors[index].URL,
		err,
	)
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
	if yearMonthRegex.MatchString(i.ItemInfo.Title.DisplayValue) {
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

func isNameMatched(author *Author, i entity.Item) bool {
	authorName := normalizeName(author.Name)
	for _, c := range i.ItemInfo.ByLineInfo.Contributors {
		if strings.Contains(authorName, normalizeName(c.Name)) {
			return true
		}
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

func sortUniqueAuthors(authors []Author) []Author {
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

	return uniqueAuthors
}

func saveAuthors(cfg aws.Config, authors []Author) error {
	prettyJSON, err := json.MarshalIndent(authors, "", "    ")
	if err != nil {
		return err
	}

	return utils.PutS3Object(cfg, strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"), utils.EnvConfig.S3AuthorsObjectKey)
}

func updateGist(authors []Author) error {
	var lines []string
	for _, author := range authors {
		lines = append(lines, fmt.Sprintf("* [[%s]%s](%s)", author.LatestReleaseDate.Format("2006-01-02"), author.Name, author.URL))
	}

	markdown := fmt.Sprintf("## 合計 %d人(最新の単行本発売日降順)\n%s", len(authors), strings.Join(lines, "\n"))

	return utils.UpdateGist(gistID, gistFilename, markdown)
}
