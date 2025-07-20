package notification

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/mattn/go-mastodon"
	"github.com/slack-go/slack"

	appconfig "kindle_bot/internal/config"
)

func AlertToSlack(err error, withMention bool) error {
	if withMention {
		return PostToSlack(fmt.Sprintf("<@U0MHY7ATX> %s\n```%v```", getFilename(), err), appconfig.EnvConfig.SlackErrorChannel)
	} else {
		return PostToSlack(fmt.Sprintf("%s\n```%v```", getFilename(), err), appconfig.EnvConfig.SlackErrorChannel)
	}
}

func PostToSlack(message string, targetChannel string) error {
	api := slack.New(appconfig.EnvConfig.SlackBotToken)

	_, _, err := api.PostMessage(
		targetChannel,
		slack.MsgOptionText(message, false),
	)
	return err
}

func TootMastodon(message string) (*mastodon.Status, error) {
	c := mastodon.NewClient(&mastodon.Config{
		appconfig.EnvConfig.MastodonServer,
		appconfig.EnvConfig.MastodonClientID,
		appconfig.EnvConfig.MastodonClientSecret,
		appconfig.EnvConfig.MastodonAccessToken,
	})

	return c.PostStatus(context.Background(), &mastodon.Toot{Status: message, Visibility: "public"})
}

func LogAndNotify(message string, sendToSlack bool) {
	log.Println(message)
	if sendToSlack {
		if _, err := TootMastodon(message); err != nil {
			AlertToSlack(fmt.Errorf("Failed to post to Mastodon: %v", err), false)
		}
	}
	if err := PostToSlack(message, appconfig.EnvConfig.SlackNoticeChannel); err != nil {
		AlertToSlack(fmt.Errorf("Failed to post to Slack: %v", err), false)
	}
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
				return filepath.Base(prevFrame.File)
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
