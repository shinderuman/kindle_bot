package main

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/goark/pa-api/entity"

	"kindle_bot/utils"
)

const (
	gistID       = "571a55fc0f9e56156cae277ded0cf09c"
	gistFilename = "わいのセールになってほしい本.md"
)

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	originalBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch unprocessed ASINs: %w", err)
	}

	upcomingBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch upcoming ASINs: %w", err)
	}

	allBooks := utils.UniqueASINs(append(originalBooks, upcomingBooks...))
	segmentBooks, startIndex, endIndex := getNextProcessingSegment(cfg, allBooks)

	processedBooks, err := checkBooksForSales(cfg, segmentBooks)
	if err != nil {
		return fmt.Errorf("PA API processing failed: %v", err)
	}

	if err := utils.PutS3Object(cfg, fmt.Sprintf("%d", startIndex+len(processedBooks)), utils.EnvConfig.S3PrevIndexSaleCheckerObjectKey); err != nil {
		return fmt.Errorf("failed to save progress index: %w", err)
	}

	updatedBooks := replaceProcessedSegment(allBooks, processedBooks, startIndex, endIndex)

	utils.SortByReleaseDate(updatedBooks)
	if reflect.DeepEqual(originalBooks, updatedBooks) {
		log.Println("No changes detected in book data, skipping file updates")
		return nil
	}

	log.Println("Changes detected in book data, proceeding with file updates")
	if err := utils.SaveASINs(cfg, updatedBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save unprocessed ASINs: %w", err)
	}

	if err := utils.UpdateBookGist(gistID, gistFilename, updatedBooks); err != nil {
		return fmt.Errorf("error update gist: %s", err)
	}

	if err := clearUpcomingBooksIfUnchanged(cfg, upcomingBooks); err != nil {
		return fmt.Errorf("failed to clear upcoming books: %w", err)
	}

	return nil
}

func getNextProcessingSegment(cfg aws.Config, books []utils.KindleBook) ([]utils.KindleBook, int, int) {
	if len(books) == 0 {
		return books, 0, 0
	}

	startIndex := getLastProcessedIndex(cfg)
	if startIndex >= len(books) {
		startIndex = 0
	}

	endIndex := min(startIndex+10, len(books))

	segment := books[startIndex:endIndex]

	log.Printf("Processing books %d-%d of %d total (segment size: %d)",
		startIndex+1, endIndex, len(books), len(segment))

	for i, book := range segment {
		log.Printf("[Queue] %d/%d: %s | %s | %s",
			startIndex+i+1, len(books), book.ReleaseDate.Format("2006-01-02"), book.Title, book.URL)
	}

	return segment, startIndex, endIndex
}

func getLastProcessedIndex(cfg aws.Config) int {
	data, err := utils.GetS3Object(cfg, utils.EnvConfig.S3PrevIndexSaleCheckerObjectKey)
	if err != nil {
		return 0
	}

	var index int
	if _, err := fmt.Sscanf(string(data), "%d", &index); err != nil {
		return 0
	}
	return index
}

func checkBooksForSales(cfg aws.Config, segmentBooks []utils.KindleBook) ([]utils.KindleBook, error) {
	client := utils.CreateClient()

	var processedBooks []utils.KindleBook

	var asins []string
	for _, book := range segmentBooks {
		asins = append(asins, book.ASIN)
	}
	resp, err := utils.GetItems(cfg, client, asins, 30)
	if err != nil {
		utils.PutMetric(cfg, "KindleBot/SaleChecker", "APIFailure")
		return segmentBooks, err
	}

	utils.PutMetric(cfg, "KindleBot/SaleChecker", "APISuccess")

	checkMissingASINs(segmentBooks, resp.ItemsResult.Items)

	for _, item := range resp.ItemsResult.Items {
		if !isKindle(item) {
			utils.AlertToSlack(fmt.Errorf(
				"the item category is not a Kindle版.\nASIN: %s\nTitle: %s\nCategory: %s\nURL: %s",
				item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
			), false)
			continue
		}

		book := utils.GetBook(item.ASIN, segmentBooks)

		if item.Offers == nil || item.Offers.Listings == nil || len(*item.Offers.Listings) == 0 || (*item.Offers.Listings)[0].Price == nil {
			utils.AlertToSlack(fmt.Errorf(
				"price information not available for item.\nASIN: %s\nTitle: %s\nURL: %s",
				item.ASIN, item.ItemInfo.Title.DisplayValue, item.DetailPageURL,
			), false)

			processedBooks = append(processedBooks, book)
			continue
		}

		maxPrice := max(book.MaxPrice, (*item.Offers.Listings)[0].Price.Amount)

		if conditions := extractSaleConditions(item, maxPrice); len(conditions) > 0 {
			utils.LogAndNotify(formatSlackMessage(item, conditions), true)
		} else {
			updatedBook := utils.MakeBook(item, maxPrice)
			processedBooks = append(processedBooks, updatedBook)
		}
	}

	return processedBooks, nil
}

func isKindle(item entity.Item) bool {
	return item.ItemInfo.Classifications.Binding.DisplayValue == "Kindle版"
}

func checkMissingASINs(requestedBooks []utils.KindleBook, responseItems []entity.Item) {
	if len(requestedBooks) == len(responseItems) {
		return
	}

	responseASINs := make(map[string]bool)
	for _, item := range responseItems {
		responseASINs[item.ASIN] = true
	}

	for _, book := range requestedBooks {
		if !responseASINs[book.ASIN] {
			utils.AlertToSlack(fmt.Errorf(
				"book not found in GetItems response.\nASIN: %s\nTitle: %s\nRelease Date: %s\nURL: %s\nRequested count: %d\nResponse count: %d",
				book.ASIN, book.Title, book.ReleaseDate.Format("2006-01-02"), book.URL, len(requestedBooks), len(responseItems),
			), false)
		}
	}
}

func extractSaleConditions(item entity.Item, maxPrice float64) []string {
	currentPrice := (*item.Offers.Listings)[0].Price.Amount
	loyaltyPoints := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string
	if priceDiff := maxPrice - currentPrice; priceDiff >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅ 最高額との価格差 %.0f円", priceDiff))
	}
	if loyaltyPoints >= 151 {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント %dpt", loyaltyPoints))
	}
	if pointPercent := float64(loyaltyPoints) / currentPrice * 100; pointPercent >= 20 {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント還元 %.1f%%", pointPercent))
	}

	return conditions
}

func formatSlackMessage(item entity.Item, conditions []string) string {
	return fmt.Sprintf(
		"📚 セール情報: %s\n条件達成: %s\n%s",
		item.ItemInfo.Title.DisplayValue,
		strings.Join(conditions, " "),
		item.DetailPageURL,
	)
}

func replaceProcessedSegment(allBooks, processedBooks []utils.KindleBook, startIndex, endIndex int) []utils.KindleBook {
	result := allBooks[:startIndex]
	result = append(result, processedBooks...)
	result = append(result, allBooks[endIndex:]...)
	return result
}

func clearUpcomingBooksIfUnchanged(cfg aws.Config, upcomingBooks []utils.KindleBook) error {
	currentUpcoming, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UpcomingObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch current upcoming ASINs for cleanup: %w", err)
	}

	if reflect.DeepEqual(upcomingBooks, currentUpcoming) {
		if err := utils.SaveASINs(cfg, []utils.KindleBook{}, utils.EnvConfig.S3UpcomingObjectKey); err != nil {
			return fmt.Errorf("failed to clear upcoming ASINs: %w", err)
		}
		log.Printf("Cleared %d upcoming books", len(upcomingBooks))
	} else {
		log.Printf("Upcoming ASINs changed during processing (was %d, now %d), skipping clear to avoid race condition",
			len(upcomingBooks), len(currentUpcoming))
	}

	return nil
}
