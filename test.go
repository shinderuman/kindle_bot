package main

import (
	"context"
	_ "encoding/json"
	_ "fmt"
	"log"
	"net/http"

	paapi5 "github.com/goark/pa-api"
	_ "github.com/goark/pa-api/query"

	"kindle_bot/utils"
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
	// prettyJSON, err := json.MarshalIndent(nil, "", "  ")
	// if err != nil {
	// 	return err
	// }
	// fmt.Println(string(prettyJSON))

	client := paapi5.New(
		paapi5.WithMarketplace(paapi5.LocaleJapan),
	).CreateClient(utils.EnvConfig.AmazonPartnerTag, utils.EnvConfig.AmazonAccessKey, utils.EnvConfig.AmazonSecretKey, paapi5.WithHttpClient(&http.Client{}))

	res, err := utils.GetItems(client, []string{"B0BHHJFR95"})
	// q := query.NewSearchItems(client.Marketplace(), client.PartnerTag(), client.PartnerType()).
	// 	// Search(query.Title, "ヤングジャンプ").
	// 	// Search(query.Keywords, "Kindle版 ヤングジャンプ編集部").
	// 	Search(query.Title, "ヤングガンガン").
	// 	Search(query.Keywords, "Kindle版 ヤングガンガン編集部").
	// 	Search(query.SearchIndex, "KindleStore").
	// 	Search(query.SortBy, "NewestArrivals").
	// 	EnableItemInfo().
	// 	EnableOffers()

	// res, err := utils.SearchItems(client, q)
	if err != nil {
		return err
	}
	utils.PrintPrettyJSON(res)
	// fmt.Println(len(res.SearchResult.Items))
	return nil
}
