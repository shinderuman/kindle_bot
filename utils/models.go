package utils

import (
	"github.com/goark/pa-api/entity"
)

type Config struct {
	S3BucketName                      string `json:"S3BucketName"`
	S3UnprocessedObjectKey            string `json:"S3UnprocessedObjectKey"`
	S3PaperBooksObjectKey             string `json:"S3PaperBooksObjectKey"`
	S3AuthorsObjectKey                string `json:"S3AuthorsObjectKey"`
	S3ExcludedTitleKeywordsObjectKey  string `json:"S3ExcludedTitleKeywordsObjectKey"`
	S3NotifiedObjectKey               string `json:"S3NotifiedObjectKey"`
	S3UpcomingObjectKey               string `json:"S3UpcomingObjectKey"`
	S3PrevIndexNewReleaseObjectKey    string `json:"S3PrevIndexNewReleaseObjectKey"`
	S3PrevIndexPaperToKindleObjectKey string `json:"S3PrevIndexPaperToKindleObjectKey"`
	S3PrevIndexSaleCheckerObjectKey   string `json:"S3PrevIndexSaleCheckerObjectKey"`
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
	S3CheckerConfigObjectKey          string `json:"S3CheckerConfigObjectKey"`
}

type CheckerConfigs struct {
	ReportFailure        bool                       `json:"ReportFailure"`
	SaleChecker          SaleCheckerConfig          `json:"SaleChecker"`
	NewReleaseChecker    NewReleaseCheckerConfig    `json:"NewReleaseChecker"`
	PaperToKindleChecker PaperToKindleCheckerConfig `json:"PaperToKindleChecker"`
}

type SaleCheckerConfig struct {
	Enabled                     bool   `json:"Enabled"`
	GistID                      string `json:"GistID"`
	GistFilename                string `json:"GistFilename"`
	ExecutionIntervalMinutes    int    `json:"ExecutionIntervalMinutes"`
	GetItemsPaapiRetryCount     int    `json:"GetItemsPaapiRetryCount"`
	GetItemsInitialRetrySeconds int    `json:"GetItemsInitialRetrySeconds"`
	SaleThreshold               int    `json:"SaleThreshold"`
	PointPercent                int    `json:"PointPercent"`
	PriceChangeAmount           int    `json:"PriceChangeAmount"`
}

type NewReleaseCheckerConfig struct {
	Enabled                        bool    `json:"Enabled"`
	GistID                         string  `json:"GistID"`
	GistFilename                   string  `json:"GistFilename"`
	CycleDays                      float64 `json:"CycleDays"`
	SearchItemsPaapiRetryCount     int     `json:"SearchItemsPaapiRetryCount"`
	SearchItemsInitialRetrySeconds int     `json:"SearchItemsInitialRetrySeconds"`
	GetItemsPaapiRetryCount        int     `json:"GetItemsPaapiRetryCount"`
	GetItemsInitialRetrySeconds    int     `json:"GetItemsInitialRetrySeconds"`
}

type PaperToKindleCheckerConfig struct {
	Enabled                        bool    `json:"Enabled"`
	GistID                         string  `json:"GistID"`
	GistFilename                   string  `json:"GistFilename"`
	CycleDays                      float64 `json:"CycleDays"`
	SearchItemsPaapiRetryCount     int     `json:"SearchItemsPaapiRetryCount"`
	SearchItemsInitialRetrySeconds int     `json:"SearchItemsInitialRetrySeconds"`
	GetItemsPaapiRetryCount        int     `json:"GetItemsPaapiRetryCount"`
	GetItemsInitialRetrySeconds    int     `json:"GetItemsInitialRetrySeconds"`
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
