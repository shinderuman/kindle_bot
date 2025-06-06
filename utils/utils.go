package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
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

var EnvConfig Config

func InitConfig() error {
	if IsLambda() {
		ctx := context.Background()

		plainParams, err := getSSMParameters(ctx, "/myapp/plain", false)
		if err != nil {
			return err
		}

		secureParams, err := getSSMParameters(ctx, "/myapp/secure", true)
		if err != nil {
			return err
		}

		for k, v := range secureParams {
			plainParams[k] = v
		}

		paramMap := plainParams

		EnvConfig = Config{
			S3BucketName:                     paramMap["S3_BUCKET_NAME"],
			S3UnprocessedObjectKey:           paramMap["S3_UNPROCESSED_OBJECT_KEY"],
			S3PaperBooksObjectKey:            paramMap["S3_PAPER_BOOKS_OBJECT_KEY"],
			S3OngoingObjectKey:               paramMap["S3_ONGOING_OBJECT_KEY"],
			S3AuthorsObjectKey:               paramMap["S3_AUTHORS_OBJECT_KEY"],
			S3ExcludedTitleKeywordsObjectKey: paramMap["S3_EXCLUDED_TITLE_KEYWORDS_OBJECT_KEY"],
			S3NotifiedObjectKey:              paramMap["S3_NOTIFIED_OBJECT_KEY"],
			S3UpcomingObjectKey:              paramMap["S3_UPCOMING_OBJECT_KEY"],
			S3Region:                         paramMap["S3_REGION"],
			AmazonPartnerTag:                 paramMap["AMAZON_PARTNER_TAG"],
			AmazonAccessKey:                  paramMap["AMAZON_ACCESS_KEY"],
			AmazonSecretKey:                  paramMap["AMAZON_SECRET_KEY"],
			MastodonServer:                   paramMap["MASTODON_SERVER"],
			MastodonClientID:                 paramMap["MASTODON_CLIENT_ID"],
			MastodonClientSecret:             paramMap["MASTODON_CLIENT_SECRET"],
			MastodonAccessToken:              paramMap["MASTODON_ACCESS_TOKEN"],
			SlackBotToken:                    paramMap["SLACK_BOT_TOKEN"],
			SlackNoticeChannel:               paramMap["SLACK_NOTICE_CHANNEL"],
			SlackErrorChannel:                paramMap["SLACK_ERROR_CHANNEL"],
			GistID:                           paramMap["GIST_ID"],
			GitHubToken:                      paramMap["GITHUB_TOKEN"],
		}
	} else {
		data, err := ioutil.ReadFile("config.json")
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, &EnvConfig); err != nil {
			return err
		}
	}

	return nil
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

func ChunkedASINs(books []KindleBook, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(books); i += size {
		end := i + size
		if end > len(books) {
			end = len(books)
		}
		var chunk []string
		for _, book := range books[i:end] {
			chunk = append(chunk, book.ASIN)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func SortByReleaseDate(books []KindleBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].ReleaseDate.Time.After(books[j].ReleaseDate.Time) {
			return true
		} else if books[i].ReleaseDate.Time.Equal(books[j].ReleaseDate.Time) {
			// ReleaseDate が同じ場合は Title で比較
			return books[i].Title < books[j].Title
		}
		return false
	})
}

func GetItems(client paapi5.Client, asinChunk []string) (*entity.Response, error) {
	q := query.NewGetItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
		ASINs(asinChunk).
		EnableItemInfo().
		EnableOffers()

	body, err := requestWithBackoff(client, q)
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

func SearchItems(client paapi5.Client, q *query.SearchItems) (*entity.Response, error) {
	body, err := requestWithBackoff(client, q)
	if err != nil {
		return nil, fmt.Errorf("PA API request failed: %w", err)
	}

	res, err := entity.DecodeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("JSON decode error: %w", err)
	}

	return res, nil
}

func requestWithBackoff[T paapi5.Query](client paapi5.Client, q T) ([]byte, error) {
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		body, err := client.Request(q)
		if err == nil {
			return body, nil
		}

		if findStatusCode(err) == 429 {
			waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second
			waitTime += time.Duration(rand.Intn(1000)) * time.Millisecond
			log.Printf("Rate limit hit. Retrying in %v...\n", waitTime)
			time.Sleep(waitTime)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("Max retries reached")
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

func PrintPrettyJSON(v any) {
	prettyJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}

	fmt.Println(strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"))
}

func SaveASINs(cfg aws.Config, ASINs []KindleBook, objectKey string) error {
	client := s3.NewFromConfig(cfg)

	prettyJSON, err := json.MarshalIndent(ASINs, "", "    ")
	if err != nil {
		return err
	}

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(EnvConfig.S3BucketName),
		Key:         aws.String(objectKey),
		Body:        strings.NewReader(strings.ReplaceAll(string(prettyJSON), `\u0026`, "&")),
		ACL:         types.ObjectCannedACLPrivate,
		ContentType: aws.String("application/json"),
	})
	return err
}

func AlertToSlack(err error, withMention ...bool) error {
	log.Println(err)
	w := true
	if len(withMention) > 0 {
		w = withMention[0]
	}

	if w {
		return PostToSlack(fmt.Sprintf("<@U0MHY7ATX>\n```%v```", err), EnvConfig.SlackErrorChannel)
	} else {
		return PostToSlack(fmt.Sprintf("```%v```", err), EnvConfig.SlackErrorChannel)
	}
}

func PostToSlack(message string, channel ...string) error {
	api := slack.New(EnvConfig.SlackBotToken)

	targetChannel := EnvConfig.SlackNoticeChannel
	if len(channel) > 0 {
		targetChannel = channel[0]
	}

	_, _, err := api.PostMessage(
		targetChannel,
		slack.MsgOptionText(message, false),
	)
	return err
}

func TootMastodon(message string) (*mastodon.Status, error) {
	c := mastodon.NewClient(&mastodon.Config{
		EnvConfig.MastodonServer,
		EnvConfig.MastodonClientID,
		EnvConfig.MastodonClientSecret,
		EnvConfig.MastodonAccessToken,
	})

	return c.PostStatus(context.Background(), &mastodon.Toot{Status: message, Visibility: "public"})
}

func UpdateGist(books []KindleBook, filename string) error {
	url := fmt.Sprintf("https://api.github.com/gists/%s", EnvConfig.GistID)

	var lines []string
	for _, book := range books {
		lines = append(lines, fmt.Sprintf("* [[%s]%s](%s)", book.ReleaseDate.Format("2006-01-02"), book.Title, book.URL))
	}

	markdown := fmt.Sprintf("## 合計 %d冊\n%s", len(books), strings.Join(lines, "\n"))

	payload := map[string]interface{}{
		"files": map[string]interface{}{
			filename: map[string]string{
				"content": markdown,
			},
		},
	}

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

func AppendFallbackBooks(asins []string, original []KindleBook) []KindleBook {
	var result []KindleBook
	for _, asin := range asins {
		result = append(result, GetBook(asin, original))
	}
	return result
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

func LogAndNotify(message string) {
	log.Println(message)
	if err := PostToSlack(message); err != nil {
		AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err))
	}
}
