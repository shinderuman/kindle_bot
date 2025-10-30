package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"reflect"
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

var (
	yearMonthRegex = regexp.MustCompile(`\d{4}年\d{1,2}月`)
)

type Author struct {
	Name               string    `json:"Name"`
	URL                string    `json:"URL"`
	LatestReleaseDate  time.Time `json:"LatestReleaseDate"`
	LatestReleaseTitle string    `json:"LatestReleaseTitle"`
	LatestReleaseURL   string    `json:"LatestReleaseURL"`
}

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

	// フラグチェックを早期に行う
	if shouldShowNext() {
		return displayNextTarget(cfg, checkerConfigs)
	}

	if !checkerConfigs.NewReleaseChecker.Enabled && utils.IsLambda() {
		log.Printf("NewReleaseChecker is disabled, skipping execution")
		return nil
	}

	authors, index, err := getAuthorToProcess(cfg, checkerConfigs)
	if err != nil {
		return err
	}
	if authors == nil {
		return nil
	}

	if err = utils.PutS3Object(cfg, strconv.Itoa(index), utils.EnvConfig.S3PrevIndexNewReleaseObjectKey); err != nil {
		return err
	}

	if err = processCore(cfg, authors, index, checkerConfigs); err != nil {
		return err
	}

	utils.PutMetric(cfg, "KindleBot/NewReleaseChecker", "SlotSuccess")

	return nil
}

func shouldShowNext() bool {
	showNext := flag.Bool("show-next", false, "Show next processing target and insertion simulation")
	flag.BoolVar(showNext, "n", false, "Show next processing target and insertion simulation (shorthand)")
	flag.Parse()
	return *showNext
}

func displayNextTarget(cfg aws.Config, checkerConfigs *utils.CheckerConfigs) error {
	authors, err := fetchAuthors(cfg)
	if err != nil {
		return fmt.Errorf("failed to fetch authors: %w", err)
	}

	if len(authors) == 0 {
		fmt.Println("No authors found")
		return nil
	}

	index, _, nextExecutionTime, err := utils.ProcessSlot(cfg, len(authors), checkerConfigs.NewReleaseChecker.CycleDays, utils.EnvConfig.S3PrevIndexNewReleaseObjectKey)
	if err != nil {
		return err
	}

	printNextTargetInfo(authors, index, nextExecutionTime, checkerConfigs.NewReleaseChecker.CycleDays)

	return nil
}

func printNextTargetInfo(authors []Author, index int, nextExecutionTime time.Time, cycleDays float64) {
	lineNumber := getAuthorLineNumber(index)
	currentItemCount := len(authors)
	simulatedItemCount := currentItemCount + 1
	simulatedIndex, _ := utils.GetIndexAndNextExecutionTime(simulatedItemCount, cycleDays)

	printBasicInfo(index+1, currentItemCount, float64(index+1)/float64(currentItemCount)*100, authors[index].Name, lineNumber, nextExecutionTime)
	printSimulationResult(index, simulatedIndex, simulatedIndex+1, simulatedItemCount, authors, lineNumber)
}

func printBasicInfo(currentPosition, currentItemCount int, currentPercentage float64, authorName string, lineNumber int, nextExecutionTime time.Time) {
	fmt.Printf(`Next processing target: %d/%d (%.1f%%)
Author: %s
Line number: %d
Next execution: %s
`,
		currentPosition, currentItemCount, currentPercentage,
		authorName,
		lineNumber,
		utils.FormatTimeJST(nextExecutionTime))
}

func printSimulationResult(index, simulatedIndex, simulatedPosition, simulatedItemCount int, authors []Author, lineNumber int) {
	fmt.Printf(`--- After inserting a new author ---
Next processing target would be: %d/%d
`,
		simulatedPosition, simulatedItemCount)

	if simulatedIndex == index {
		fmt.Printf(`✅ Safe: Insert at index %d (line %d)
New author will be processed in the next execution
`,
			index, lineNumber)
	} else {
		fmt.Printf(`⚠️  WARNING: Timeline shift detected!
Current plan: Process index %d (%s) at line %d
After insertion: Will process index %d (%s) at line %d instead
Solution: Insert new author at index %d (line %d) to be processed next
Don't insert at index %d - it will be skipped!
`,
			index, authors[index].Name, lineNumber,
			simulatedIndex, authors[simulatedIndex].Name, getAuthorLineNumber(simulatedIndex),
			simulatedIndex, getAuthorLineNumber(simulatedIndex),
			index)
	}
}

func getAuthorLineNumber(index int) int {
	authorType := reflect.TypeOf(Author{})
	fieldCount := authorType.NumField()

	linesPerAuthor := fieldCount + 2

	return linesPerAuthor*index + 2
}

func getAuthorToProcess(cfg aws.Config, checkerConfigs *utils.CheckerConfigs) ([]Author, int, error) {
	authors, err := fetchAuthors(cfg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch authors: %w", err)
	}

	index, shouldProcess, nextExecutionTime, err := utils.ProcessSlot(cfg, len(authors), checkerConfigs.NewReleaseChecker.CycleDays, utils.EnvConfig.S3PrevIndexNewReleaseObjectKey)
	if err != nil {
		return nil, 0, err
	}
	if !shouldProcess {
		return nil, 0, nil
	}

	format := utils.GetCountFormat(len(authors))
	log.Printf(fmt.Sprintf("Processing slot (%s / %s): %%s, next execution: %s (%s)", format, format, utils.FormatTimeJST(nextExecutionTime), utils.FormatExecutionInterval(nextExecutionTime)), index+1, len(authors), authors[index].Name)
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

func processCore(cfg aws.Config, authors []Author, index int, checkerConfigs *utils.CheckerConfigs) error {
	start := time.Now()
	client := utils.CreateClient()
	author := &authors[index]

	if author.Name == "" {
		return fmt.Errorf("empty name found in author at index %d: URL=%s", index, author.URL)
	}

	notifiedMap, err := utils.FetchNotifiedASINs(cfg, start)
	if err != nil {
		return err
	}

	ngWords, err := fetchExcludedTitleKeywords(cfg)
	if err != nil {
		return err
	}

	upcomingMap := make(map[string]utils.KindleBook)
	items, err := searchAuthorBooks(cfg, client, author.Name, checkerConfigs)
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

		utils.LogAndNotify(fmt.Sprintf(strings.TrimSpace(`
📚 新刊予定があります: %s
作者: %s
発売日: %s
ASIN: %s
%s`),
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

	if err := utils.SaveNotifiedASINs(cfg, notifiedMap); err != nil {
		return err
	}

	if err := utils.SaveUpcomingASINs(cfg, upcomingMap); err != nil {
		return err
	}

	if !author.LatestReleaseDate.Equal(latest) {
		authors = sortUniqueAuthors(authors)
		if err := saveAuthors(cfg, authors); err != nil {
			return err
		}
		if err := updateGist(authors, checkerConfigs); err != nil {
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

func searchAuthorBooks(cfg aws.Config, client paapi5.Client, authorName string, checkerConfigs *utils.CheckerConfigs) ([]entity.Item, error) {
	q := utils.CreateSearchQuery(
		client,
		query.Author,
		authorName,
		0,
	)

	res, err := utils.SearchItems(cfg, client, q, checkerConfigs.NewReleaseChecker.SearchItemsPaapiRetryCount, checkerConfigs.NewReleaseChecker.SearchItemsInitialRetrySeconds)
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

	if releaseDate.After(author.LatestReleaseDate) {
		author.LatestReleaseDate = releaseDate
		author.LatestReleaseTitle = i.ItemInfo.Title.DisplayValue
		author.LatestReleaseURL = cleanURL(i.DetailPageURL)
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

func cleanURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""

	return parsedURL.String()
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

func updateGist(authors []Author, checkerConfigs *utils.CheckerConfigs) error {
	var lines []string

	lines = append(lines, "| 作者 | 最新作 |")
	lines = append(lines, "|------|--------|")
	for _, author := range authors {
		lines = append(lines, fmt.Sprintf("| [%s](%s) | [[%s] %s](%s) |",
			author.Name,
			author.URL,
			author.LatestReleaseDate.Format("2006-01-02"),
			author.LatestReleaseTitle,
			author.LatestReleaseURL))
	}

	markdown := fmt.Sprintf("## 合計 %d人(最新の単行本発売日降順)\n%s", len(authors), strings.Join(lines, "\n"))

	return utils.UpdateGist(checkerConfigs.NewReleaseChecker.GistID, checkerConfigs.NewReleaseChecker.GistFilename, markdown)
}
