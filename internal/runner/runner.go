package runner

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	"kindle_bot/internal/config"
	"kindle_bot/internal/notification"
)

func Run(process func() error) {
	if err := config.InitConfig(); err != nil {
		log.Println("Error loading configuration:", err)
		return
	}

	handler := func(ctx context.Context) (string, error) {
		err := process()
		if err != nil {
			notification.AlertToSlack(err, false)
		}
		return "Processing complete", err
	}

	if config.IsLambda() {
		lambda.Start(handler)
	} else {
		handler(context.Background())
	}
}
