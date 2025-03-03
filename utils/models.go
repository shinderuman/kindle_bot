package utils

import (
	"github.com/goark/pa-api/entity"
)

type Config struct {
	S3BucketName           string `json:"S3BucketName"`
	S3UnprocessedObjectKey string `json:"S3UnprocessedObjectKey"`
	S3PaperBooksObjectKey  string `json:"S3PaperBooksObjectKey"`
	S3OngoingObjectKey     string `json:"S3OngoingObjectKey"`
	S3NotifiedObjectKey    string `json:"S3NotifiedObjectKey"`
	S3Region               string `json:"S3Region"`
	AmazonPartnerTag       string `json:"AmazonPartnerTag"`
	AmazonAccessKey        string `json:"AmazonAccessKey"`
	AmazonSecretKey        string `json:"AmazonSecretKey"`
	MastodonServer         string `json:"MastodonServer"`
	MastodonClientID       string `json:"MastodonClientID"`
	MastodonClientSecret   string `json:"MastodonClientSecret"`
	MastodonAccessToken    string `json:"MastodonAccessToken"`
	SlackBotToken          string `json:"SlackBotToken"`
	SlackNoticeChannel     string `json:"SlackNoticeChannel"`
	SlackErrorChannel      string `json:"SlackErrorChannel"`
	GistID                 string `json:"GistID"`
	GitHubToken            string `json:"GitHubToken"`
}

type KindleBook struct {
	ASIN         string      `json:"ASIN"`
	Title        string      `json:"Title"`
	ReleaseDate  entity.Date `json:"ReleaseDate"`
	CurrentPrice float64     `json:"CurrentPrice"`
	MaxPrice     float64     `json:"MaxPrice"`
	URL          string      `json:"URL"`
}
