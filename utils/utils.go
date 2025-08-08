package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/goark/errs"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"
	"github.com/mattn/go-mastodon"
	"github.com/slack-go/slack"
)

var (
	EnvConfig Config

	getItemsPAAPIRetryCount = 3
	configInitErr           error
	once                    sync.Once
)

func Run(process func() error) {
	if err := initConfig(); err != nil {
		log.Println("Error loading configuration:", err)
		return
	}

	handler := func(ctx context.Context) (string, error) {
		err := process()
		if err != nil {
			AlertToSlack(err, false)
		}
		return "Processing complete: " + getFilename(), err
	}

	if IsLambda() {
		lambda.Start(handler)
	} else {
		handler(context.Background())
	}
}

func initConfig() error {
	if IsLambda() {
		once.Do(func() {
			ctx := context.Background()

			plainParams, err := getSSMParameters(ctx, "/myapp/plain", false)
			if err != nil {
				configInitErr = err
				return
			}

			secureParams, err := getSSMParameters(ctx, "/myapp/secure", true)
			if err != nil {
				configInitErr = err
				return
			}

			maps.Copy(plainParams, secureParams)

			paramMap := plainParams
			EnvConfig = Config{
				S3BucketName:                      paramMap["S3_BUCKET_NAME"],
				S3UnprocessedObjectKey:            paramMap["S3_UNPROCESSED_OBJECT_KEY"],
				S3PaperBooksObjectKey:             paramMap["S3_PAPER_BOOKS_OBJECT_KEY"],
				S3AuthorsObjectKey:                paramMap["S3_AUTHORS_OBJECT_KEY"],
				S3ExcludedTitleKeywordsObjectKey:  paramMap["S3_EXCLUDED_TITLE_KEYWORDS_OBJECT_KEY"],
				S3NotifiedObjectKey:               paramMap["S3_NOTIFIED_OBJECT_KEY"],
				S3UpcomingObjectKey:               paramMap["S3_UPCOMING_OBJECT_KEY"],
				S3PrevIndexNewReleaseObjectKey:    paramMap["S3_PREV_INDEX_NEW_RELEASE_OBJECT_KEY"],
				S3PrevIndexPaperToKindleObjectKey: paramMap["S3_PREV_INDEX_PAPER_TO_KINDLE_OBJECT_KEY"],
				S3PrevIndexSaleCheckerObjectKey:   paramMap["S3_PREV_INDEX_SALE_CHECKER_OBJECT_KEY"],
				S3Region:                          paramMap["S3_REGION"],
				AmazonPartnerTag:                  paramMap["AMAZON_PARTNER_TAG"],
				AmazonAccessKey:                   paramMap["AMAZON_ACCESS_KEY"],
				AmazonSecretKey:                   paramMap["AMAZON_SECRET_KEY"],
				MastodonServer:                    paramMap["MASTODON_SERVER"],
				MastodonClientID:                  paramMap["MASTODON_CLIENT_ID"],
				MastodonClientSecret:              paramMap["MASTODON_CLIENT_SECRET"],
				MastodonAccessToken:               paramMap["MASTODON_ACCESS_TOKEN"],
				SlackBotToken:                     paramMap["SLACK_BOT_TOKEN"],
				SlackNoticeChannel:                paramMap["SLACK_NOTICE_CHANNEL"],
				SlackErrorChannel:                 paramMap["SLACK_ERROR_CHANNEL"],
				GitHubToken:                       paramMap["GITHUB_TOKEN"],
			}
		})
	} else {
		data, err := os.ReadFile("config.json")
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, &EnvConfig); err != nil {
			return err
		}
	}

	initEnvironmentVariables()

	return configInitErr
}

func initEnvironmentVariables() {
	if envRetryCount := os.Getenv("GET_ITEMS_PAAPI_RETRY_COUNT"); envRetryCount != "" {
		if count, err := strconv.Atoi(envRetryCount); err == nil && count > 0 {
			getItemsPAAPIRetryCount = count
		}
	}
}

func getSSMParameters(ctx context.Context, prefix string, withDecryption bool) (map[string]string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := ssm.NewFromConfig(cfg)

	params := make(map[string]string)
	var nextToken *string

	for {
		input := &ssm.GetParametersByPathInput{
			Path:           aws.String(prefix),
			WithDecryption: aws.Bool(withDecryption),
			Recursive:      aws.Bool(true),
			NextToken:      nextToken,
		}

		output, err := client.GetParametersByPath(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, param := range output.Parameters {
			key := strings.TrimPrefix(*param.Name, prefix+"/")
			params[key] = *param.Value
		}

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return params, nil
}

func getFilename() string {
	const maxDepth = 50
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	var prevFrame *runtime.Frame

	for {
		frame, more := frames.Next()
		baseFile := filepath.Base(frame.File)
		if baseFile == "proc.go" {
			if prevFrame != nil {
				return prevFrame.File
			}
			return "unknown"
		}

		prevFrame = &frame
		if !more {
			break
		}
	}

	return "unknown"
}

func IsLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

func InitAWSConfig() (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(EnvConfig.S3Region),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %v", err)
	}
	return cfg, nil
}

func CreateClient() paapi5.Client {
	return paapi5.New(
		paapi5.WithMarketplace(paapi5.LocaleJapan),
	).CreateClient(
		EnvConfig.AmazonPartnerTag,
		EnvConfig.AmazonAccessKey,
		EnvConfig.AmazonSecretKey,
		paapi5.WithHttpClient(&http.Client{}),
	)
}

func GetS3Object(cfg aws.Config, objectKey string) ([]byte, error) {
	client := s3.NewFromConfig(cfg)

	input := &s3.GetObjectInput{
		Bucket: aws.String(EnvConfig.S3BucketName),
		Key:    aws.String(objectKey),
	}

	resp, err := client.GetObject(context.TODO(), input)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func PutS3Object(cfg aws.Config, body, objectKey string) error {
	client := s3.NewFromConfig(cfg)

	_, err := client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(EnvConfig.S3BucketName),
		Key:         aws.String(objectKey),
		Body:        strings.NewReader(body),
		ACL:         types.ObjectCannedACLPrivate,
		ContentType: aws.String("application/json"),
	})
	return err
}

func FetchASINs(cfg aws.Config, objectKey string) ([]KindleBook, error) {
	body, err := GetS3Object(cfg, objectKey)
	if err != nil {
		return nil, err
	}

	var ASINs []KindleBook
	if err := json.Unmarshal(body, &ASINs); err != nil {
		return nil, err
	}

	return ASINs, nil
}

func UniqueASINs(slice []KindleBook) []KindleBook {
	seen := make(map[string]struct{})
	result := []KindleBook{}

	for _, s := range slice {
		if _, exists := seen[s.ASIN]; !exists {
			seen[s.ASIN] = struct{}{}
			result = append(result, s)
		}
	}

	return result
}

func SortByReleaseDate(books []KindleBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].ReleaseDate.Time.After(books[j].ReleaseDate.Time) {
			return true
		} else if books[i].ReleaseDate.Time.Equal(books[j].ReleaseDate.Time) {
			return books[i].Title < books[j].Title
		}
		return false
	})
}

func GetBook(ASIN string, slice []KindleBook) KindleBook {
	for _, s := range slice {
		if ASIN == s.ASIN {
			return s
		}
	}
	return KindleBook{}
}

func MakeBook(item entity.Item, maxPrice float64) KindleBook {
	book := KindleBook{
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

func GetItems(cfg aws.Config, client paapi5.Client, asinChunk []string, initialRetrySeconds int) (*entity.Response, error) {
	q := query.NewGetItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
		ASINs(asinChunk).
		EnableItemInfo().
		EnableOffers()

	body, err := requestWithBackoff(cfg, client, q, getItemsPAAPIRetryCount, initialRetrySeconds)
	if err != nil {
		return nil, err
	}

	res, err := entity.DecodeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("json decode error: %w", err)
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
	body, err := requestWithBackoff(cfg, client, q, maxRetryCount, 2)
	if err != nil {
		return nil, err
	}

	res, err := entity.DecodeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("json decode error: %w", err)
	}

	return res, nil
}

func requestWithBackoff[T paapi5.Query](cfg aws.Config, client paapi5.Client, q T, maxRetryCount int, initialRetrySeconds int) ([]byte, error) {
	const maxWait = 30 * time.Second
	for i := range maxRetryCount {
		body, err := client.Request(q)
		PutMetric(cfg, "KindleBot/Usage", "PAAPIRequest")
		if err == nil {
			PutMetric(cfg, "KindleBot/Usage", "PAAPISuccess")
			return body, nil
		}

		PutMetric(cfg, "KindleBot/Usage", "PAAPIFailure")
		if isRetryableError(err) {
			if i == maxRetryCount-1 {
				PutMetric(cfg, "KindleBot/Usage", "PAAPIMaxRetriesReached")
				return nil, fmt.Errorf("max retries reached, last error: %w", err)
			}

			waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second * time.Duration(initialRetrySeconds)
			waitTime += time.Duration(rand.Intn(500)) * time.Millisecond
			if waitTime > maxWait {
				waitTime = maxWait
			}

			log.Printf("Rate limit hit. Retrying in %v... (error: %v)", waitTime, err)
			time.Sleep(waitTime)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("unexpected: loop completed without return")
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

func FetchNotifiedASINs(cfg aws.Config, now time.Time) (map[string]KindleBook, error) {
	books, err := FetchASINs(cfg, EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch notified ASINs: %w", err)
	}
	m := make(map[string]KindleBook)
	for _, b := range books {
		if b.ReleaseDate.After(now) {
			m[b.ASIN] = b
		}
	}
	return m, nil
}

func SaveNotifiedAndUpcomingASINs(cfg aws.Config, notifiedMap, upcomingMap map[string]KindleBook) error {
	if len(upcomingMap) == 0 {
		return nil
	}

	if err := saveASINsFromMap(cfg, notifiedMap, EnvConfig.S3NotifiedObjectKey); err != nil {
		return err
	}

	return updateUpcomingASINs(cfg, upcomingMap)
}

func updateUpcomingASINs(cfg aws.Config, upcomingMap map[string]KindleBook) error {
	currentUpcoming, err := FetchASINs(cfg, EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch upcoming ASINs: %w", err)
	}

	for _, b := range currentUpcoming {
		upcomingMap[b.ASIN] = b
	}

	return saveASINsFromMap(cfg, upcomingMap, EnvConfig.S3UpcomingObjectKey)
}

func saveASINsFromMap(cfg aws.Config, m map[string]KindleBook, key string) error {
	var list []KindleBook
	for _, book := range m {
		list = append(list, book)
	}
	SortByReleaseDate(list)
	return SaveASINs(cfg, list, key)
}

func SaveASINs(cfg aws.Config, ASINs []KindleBook, objectKey string) error {
	prettyJSON, err := json.MarshalIndent(ASINs, "", "    ")
	if err != nil {
		return err
	}

	return PutS3Object(cfg, strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"), objectKey)
}

func ProcessSlot(cfg aws.Config, itemCount int, cycleDays float64, prevIndexKey string) (int, bool, error) {
	if itemCount == 0 {
		return 0, false, fmt.Errorf("no items available")
	}

	index := getIndexByTime(itemCount, cycleDays)
	format := GetCountFormat(itemCount)

	prevIndexBytes, err := GetS3Object(cfg, prevIndexKey)
	if err != nil {
		return 0, false, fmt.Errorf("failed to fetch prev_index: %w", err)
	}
	prevIndex, _ := strconv.Atoi(string(prevIndexBytes))

	if prevIndex == index {
		skipLogFormat := fmt.Sprintf("Not my slot, skipping (%s / %s)", format, format)
		log.Printf(skipLogFormat, index+1, itemCount)
		return index, false, nil
	}

	return index, true, nil
}

func getIndexByTime(itemCount int, cycleDays float64) int {
	if itemCount <= 0 {
		return 0
	}

	secondsPerCycle := int64(cycleDays * 24 * 60 * 60)
	sec := time.Now().Unix() % secondsPerCycle
	return int(sec * int64(itemCount) / secondsPerCycle)
}

func GetCountFormat(itemCount int) string {
	digits := len(fmt.Sprintf("%d", itemCount))
	return fmt.Sprintf("%%0%dd", digits+1)
}

func LogAndNotify(message string, sendToSlack bool) {
	log.Println(message)
	if sendToSlack {
		if _, err := TootMastodon(message); err != nil {
			AlertToSlack(fmt.Errorf("failed to post to Mastodon: %v", err), false)
		}
	}
	if err := PostToSlack(message, EnvConfig.SlackNoticeChannel); err != nil {
		AlertToSlack(fmt.Errorf("failed to post to Slack: %v", err), false)
	}
}

func AlertToSlack(err error, withMention bool) error {
	if withMention {
		return PostToSlack(fmt.Sprintf("<@U0MHY7ATX> %s\n```%v```", getFilename(), err), EnvConfig.SlackErrorChannel)
	} else {
		return PostToSlack(fmt.Sprintf("%s\n```%v```", getFilename(), err), EnvConfig.SlackErrorChannel)
	}
}

func PostToSlack(message string, targetChannel string) error {
	api := slack.New(EnvConfig.SlackBotToken)

	_, _, err := api.PostMessage(
		targetChannel,
		slack.MsgOptionText(message, false),
	)
	return err
}

func TootMastodon(message string) (*mastodon.Status, error) {
	c := mastodon.NewClient(&mastodon.Config{
		Server:       EnvConfig.MastodonServer,
		ClientID:     EnvConfig.MastodonClientID,
		ClientSecret: EnvConfig.MastodonClientSecret,
		AccessToken:  EnvConfig.MastodonAccessToken,
	})

	return c.PostStatus(context.Background(), &mastodon.Toot{Status: message, Visibility: "public"})
}

func UpdateGist(gistID, filename, markdown string) error {
	payload := GistPayload{
		Files: GistFiles{
			filename: {
				Content: markdown,
			},
		},
	}

	url := fmt.Sprintf("https://api.github.com/gists/%s", gistID)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+EnvConfig.GitHubToken)
	req.Header.Set("Content-Type", "application/json")

	var client http.Client
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func PutMetric(cfg aws.Config, namespace, metricName string) error {
	cw := cloudwatch.NewFromConfig(cfg)
	_, err := cw.PutMetricData(context.TODO(), &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(namespace),
		MetricData: []cwtypes.MetricDatum{
			{
				MetricName: aws.String(metricName),
				Value:      aws.Float64(1.0),
				Unit:       cwtypes.StandardUnitCount,
				Timestamp:  aws.Time(time.Now()),
			},
		},
	})
	return err
}

func PrintPrettyJSON(v any) {
	prettyJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}

	fmt.Println(strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"))
}
