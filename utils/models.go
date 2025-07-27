package utils

import (
	"github.com/goark/pa-api/entity"
)

type Config struct {
	S3BucketName                      string `json:"S3BucketName"`
	S3UnprocessedObjectKey            string `json:"S3UnprocessedObjectKey"`
	S3PaperBooksObjectKey             string `json:"S3PaperBooksObjectKey"`
	S3OngoingObjectKey                string `json:"S3OngoingObjectKey"`
	S3AuthorsObjectKey                string `json:"S3AuthorsObjectKey"`
	S3ExcludedTitleKeywordsObjectKey  string `json:"S3ExcludedTitleKeywordsObjectKey"`
	S3NotifiedObjectKey               string `json:"S3NotifiedObjectKey"`
	S3UpcomingObjectKey               string `json:"S3UpcomingObjectKey"`
	S3PrevIndexNewReleaseObjectKey    string `json:"S3PrevIndexNewReleaseObjectKey"`
	S3PrevIndexPaperToKindleObjectKey string `json:"S3PrevIndexPaperToKindleObjectKey"`
	S3Region                          string `json:"S3Region"`
	AmazonPartnerTag                  string `json:"AmazonPartnerTag"`
	AmazonAccessKey                   string `json:"AmazonAccessKey"`
	AmazonSecretKey                   string `json:"AmazonSecretKey"`
	MastodonServer                    string `json:"MastodonServer"`
	MastodonClientID                  string `json:"MastodonClientID"`
	MastodonClientSecret              string `json:"MastodonClientSecret"`
	MastodonAccessToken               string `json:"MastodonAccessToken"`
	SlackBotToken                     string `json:"SlackBotToken"`
	SlackNoticeChannel                string `json:"SlackNoticeChannel"`
	SlackErrorChannel                 string `json:"SlackErrorChannel"`
	GitHubToken                       string `json:"GitHubToken"`
}

type KindleBook struct {
	ASIN         string      `json:"ASIN"`
	Title        string      `json:"Title"`
	ReleaseDate  entity.Date `json:"ReleaseDate"`
	CurrentPrice float64     `json:"CurrentPrice"`
	MaxPrice     float64     `json:"MaxPrice"`
	URL          string      `json:"URL"`
}

type GistFileContent struct {
	Content string `json:"content"`
}

type GistFiles map[string]GistFileContent

type GistPayload struct {
	Files GistFiles `json:"files"`
}
