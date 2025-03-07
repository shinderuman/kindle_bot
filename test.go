package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	paapi5 "github.com/goark/pa-api"

	"kindle_bot/utils"
)

const (
	youngJump = "ヤングジャンプ"
)

func main() {
	if err := utils.InitConfig(); err != nil {
		log.Println("Error loading configuration:", err)
		return
	}

	if err := process(); err != nil {
		log.Println(err)
	}
}

func handler(ctx context.Context) (string, error) {
	return utils.Handler(ctx, process)
}

func process() error {
	client := paapi5.New(
		paapi5.WithMarketplace(paapi5.LocaleJapan),
	).CreateClient(utils.EnvConfig.AmazonPartnerTag, utils.EnvConfig.AmazonAccessKey, utils.EnvConfig.AmazonSecretKey, paapi5.WithHttpClient(&http.Client{}))

	// response, err := utils.GetItems(client, []string{"B0DZ24RT8R"})
	res, err := utils.SearchItems(client, "紅い霧の中から")
	if err != nil {
		return err
	}
	utils.PrintPrettyJSON(res)
	fmt.Println(len(res.SearchResult.Items))
	return nil
}
