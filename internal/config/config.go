package config

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"kindle_bot/pkg/models"
)

var (
	EnvConfig     models.Config
	configInitErr error
	once          sync.Once
)

func InitConfig() error {
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

			for k, v := range secureParams {
				plainParams[k] = v
			}

			paramMap := plainParams
			EnvConfig = models.Config{
				S3BucketName:                     paramMap["S3_BUCKET_NAME"],
				S3UnprocessedObjectKey:           paramMap["S3_UNPROCESSED_OBJECT_KEY"],
				S3PaperBooksObjectKey:            paramMap["S3_PAPER_BOOKS_OBJECT_KEY"],
				S3AuthorsObjectKey:               paramMap["S3_AUTHORS_OBJECT_KEY"],
				S3ExcludedTitleKeywordsObjectKey: paramMap["S3_EXCLUDED_TITLE_KEYWORDS_OBJECT_KEY"],
				S3NotifiedObjectKey:              paramMap["S3_NOTIFIED_OBJECT_KEY"],
				S3UpcomingObjectKey:              paramMap["S3_UPCOMING_OBJECT_KEY"],
				S3PrevIndexObjectKey:             paramMap["S3_PREV_INDEX_OBJECT_KEY"],
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
				GitHubToken:                      paramMap["GITHUB_TOKEN"],
			}
		})
	} else {
		data, err := ioutil.ReadFile("config.json")
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, &EnvConfig); err != nil {
			return err
		}
	}

	return configInitErr
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
