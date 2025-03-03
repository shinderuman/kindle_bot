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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	paapi5 "github.com/goark/pa-api"
	"github.com/goark/pa-api/entity"
	"github.com/goark/pa-api/query"
	"github.com/mattn/go-mastodon"
	"github.com/slack-go/slack"
)

var EnvConfig Config

func InitConfig() error {
	if IsLambda() {
		EnvConfig = Config{
			S3BucketName:           os.Getenv("S3_BUCKET_NAME"),
			S3UnprocessedObjectKey: os.Getenv("S3_UNPROCESSED_OBJECT_KEY"),
			S3PaperBooksObjectKey:  os.Getenv("S3_PAPER_BOOKS_OBJECT_KEY"),
			S3OngoingObjectKey:     os.Getenv("S3_ONGOING_OBJECT_KEY"),
			S3NotifiedObjectKey:    os.Getenv("S3_NOTIFIED_OBJECT_KEY"),
			S3Region:               os.Getenv("S3_REGION"),
			AmazonPartnerTag:       os.Getenv("AMAZON_PARTNER_TAG"),
			AmazonAccessKey:        os.Getenv("AMAZON_ACCESS_KEY"),
			AmazonSecretKey:        os.Getenv("AMAZON_SECRET_KEY"),
			MastodonServer:         os.Getenv("MASTODON_SERVER"),
			MastodonClientID:       os.Getenv("MASTODON_CLIENT_ID"),
			MastodonClientSecret:   os.Getenv("MASTODON_CLIENT_SECRETT"),
			MastodonAccessToken:    os.Getenv("MASTODON_ACCESS_TOKEN"),
			SlackBotToken:          os.Getenv("SLACK_BOT_TOKEN"),
			SlackNoticeChannel:     os.Getenv("SLACK_NOTICE_CHANNEL"),
			SlackErrorChannel:      os.Getenv("SLACK_ERROR_CHANNEL"),
			GistID:                 os.Getenv("GIST_ID"),
			GitHubToken:            os.Getenv("GITHUB_TOKEN"),
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

func IsLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

func InitSession() (*session.Session, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(EnvConfig.S3Region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}
	return sess, nil
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

func FetchASINs(sess *session.Session, objectKey string) ([]KindleBook, error) {
	svc := s3.New(sess)

	resp, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(EnvConfig.S3BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ASINs []KindleBook
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

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

func PrintPrettyJSON(v any) {
	prettyJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}

	fmt.Println(strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"))
}

func requestWithBackoff[T paapi5.Query](client paapi5.Client, q T) ([]byte, error) {
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		body, err := client.Request(q)
		if err == nil {
			return body, nil
		}

		if isRateLimitError(err) {
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

func isRateLimitError(err error) bool {
	return strings.Contains(err.Error(), "bad HTTP status: status 429")
}

func SaveASINs(sess *session.Session, ASINs []KindleBook, objectKey string) error {
	svc := s3.New(sess)

	prettyJSON, err := json.MarshalIndent(ASINs, "", "  ")
	if err != nil {
		return err
	}

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(EnvConfig.S3BucketName),
		Key:         aws.String(objectKey),
		Body:        aws.ReadSeekCloser(strings.NewReader(strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"))),
		ACL:         aws.String("private"),
		ContentType: aws.String("application/json"),
	})
	return err
}

func AlertToSlack(err error, withMention bool) error {
	log.Println(err)
	if withMention {
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

func GetBook(ASIN string, slice []KindleBook) KindleBook {
	for _, s := range slice {
		if ASIN == s.ASIN {
			return s
		}
	}
	return KindleBook{}
}
