package main

import (
	"flag"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/goark/pa-api/entity"

	"kindle_bot/utils"
)

var (
	organize bool
)

func init() {
	flag.BoolVar(&organize, "organize", false, "Organize and sort the book list")
	flag.BoolVar(&organize, "o", false, "Organize and sort the book list (shorthand)")
}

func main() {
	flag.Parse()
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	checkerConfigs, err := utils.FetchCheckerConfigs(cfg)
	if err != nil {
		return fmt.Errorf("failed to fetch checker configs: %w", err)
	}

	if shouldOrganizeList() {
		return organizeBookList(cfg, checkerConfigs)
	}

	if !checkerConfigs.SaleChecker.Enabled && utils.IsLambda() {
		log.Printf("SaleChecker is disabled, skipping execution")
		return nil
	}

	if utils.IsLambda() {
		now := time.Now()
		intervalMinutes := checkerConfigs.SaleChecker.ExecutionIntervalMinutes
		if intervalMinutes > 0 && now.Minute()%intervalMinutes != 0 {
			log.Printf("Skipping execution: current minute %d is not divisible by interval %d", now.Minute(), intervalMinutes)
			return nil
		}
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

	processedBooks, err := checkBooksForSales(cfg, segmentBooks, checkerConfigs)
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
	logBookChanges(originalBooks, updatedBooks)

	if err := utils.SaveASINs(cfg, updatedBooks, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save unprocessed ASINs: %w", err)
	}

	if err := utils.UpdateBookGist(checkerConfigs.SaleChecker.GistID, checkerConfigs.SaleChecker.GistFilename, updatedBooks); err != nil {
		return fmt.Errorf("error update gist: %s", err)
	}

	if err := clearUpcomingBooksIfUnchanged(cfg, upcomingBooks); err != nil {
		return fmt.Errorf("failed to clear upcoming books: %w", err)
	}

	return nil
}

func shouldOrganizeList() bool {
	return organize
}

func organizeBookList(cfg aws.Config, checkerConfigs *utils.CheckerConfigs) error {
	originalBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch books from S3: %w", err)
	}

	if len(originalBooks) == 0 {
		fmt.Println("No books found")
		return nil
	}

	books := utils.UniqueASINs(originalBooks)
	utils.SortByReleaseDate(books)

	if reflect.DeepEqual(originalBooks, books) {
		fmt.Println("No changes needed")
		return nil
	}

	if err := utils.SaveASINs(cfg, books, utils.EnvConfig.S3UnprocessedObjectKey); err != nil {
		return fmt.Errorf("failed to save books to S3: %w", err)
	}

	if err := utils.UpdateBookGist(checkerConfigs.SaleChecker.GistID, checkerConfigs.SaleChecker.GistFilename, books); err != nil {
		return fmt.Errorf("failed to update gist: %w", err)
	}

	fmt.Printf("Organized %d books\n", len(books))
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

func checkBooksForSales(cfg aws.Config, segmentBooks []utils.KindleBook, checkerConfigs *utils.CheckerConfigs) ([]utils.KindleBook, error) {
	client := utils.CreateClient()

	var processedBooks []utils.KindleBook

	var asins []string
	for _, book := range segmentBooks {
		if book.ASIN == "" {
			utils.AlertToSlack(fmt.Errorf("empty ASIN found in book: Title=%s, URL=%s", book.Title, book.URL), false)
			continue
		}
		asins = append(asins, book.ASIN)
	}
	resp, err := utils.GetItems(cfg, client, asins, checkerConfigs.SaleChecker.GetItemsInitialRetrySeconds, checkerConfigs.SaleChecker.GetItemsPaapiRetryCount)
	if err != nil {
		utils.PutMetric(cfg, "KindleBot/SaleChecker", "APIFailure")
		return segmentBooks, err
	}

	utils.PutMetric(cfg, "KindleBot/SaleChecker", "APISuccess")

	checkMissingASINs(segmentBooks, resp.ItemsResult.Items)

	for _, item := range resp.ItemsResult.Items {
		if !isKindle(item) {
			utils.AlertToSlack(fmt.Errorf(strings.TrimSpace(`
the item category is not a Kindle版.
ASIN: %s
Title: %s
Category: %s
URL: %s`),
				item.ASIN, item.ItemInfo.Title.DisplayValue, item.ItemInfo.Classifications.Binding.DisplayValue, item.DetailPageURL,
			), false)
			continue
		}

		book := utils.GetBook(item.ASIN, segmentBooks)

		if item.Offers == nil || item.Offers.Listings == nil || len(*item.Offers.Listings) == 0 || (*item.Offers.Listings)[0].Price == nil {
			utils.AlertToSlack(fmt.Errorf(strings.TrimSpace(`
price information not available for item.
ASIN: %s
Title: %s
URL: %s`),
				item.ASIN, item.ItemInfo.Title.DisplayValue, item.DetailPageURL,
			), false)

			processedBooks = append(processedBooks, book)
			continue
		}

		maxPrice := max(book.MaxPrice, (*item.Offers.Listings)[0].Price.Amount)

		if conditions := extractSaleConditions(item, maxPrice, checkerConfigs); len(conditions) > 0 {
			utils.LogAndNotify(formatSlackMessage(item, conditions), true)
		} else {
			updatedBook := utils.MakeBook(item, maxPrice)
			if priceChangeMsg := checkPriceChange(book, updatedBook, checkerConfigs); priceChangeMsg != "" {
				utils.LogAndNotify(priceChangeMsg, true)
			}
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
			utils.AlertToSlack(fmt.Errorf(strings.TrimSpace(`
book not found in GetItems response.
ASIN: %s
Title: %s
Release Date: %s
URL: %s
Requested count: %d
Response count: %d`),
				book.ASIN, book.Title, book.ReleaseDate.Format("2006-01-02"), book.URL, len(requestedBooks), len(responseItems),
			), false)
		}
	}
}

func extractSaleConditions(item entity.Item, maxPrice float64, checkerConfigs *utils.CheckerConfigs) []string {
	currentPrice := (*item.Offers.Listings)[0].Price.Amount
	loyaltyPoints := (*item.Offers.Listings)[0].LoyaltyPoints.Points

	var conditions []string
	if priceDiff := maxPrice - currentPrice; priceDiff >= float64(checkerConfigs.SaleChecker.SaleThreshold) {
		conditions = append(conditions, fmt.Sprintf("✅ 最高額との価格差 %.0f円", priceDiff))
	}
	if loyaltyPoints >= checkerConfigs.SaleChecker.SaleThreshold {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント %dpt", loyaltyPoints))
	}
	if pointPercentValue := float64(loyaltyPoints) / currentPrice * 100; pointPercentValue >= float64(checkerConfigs.SaleChecker.PointPercent) {
		conditions = append(conditions, fmt.Sprintf("✅ ポイント還元 %.1f%%", pointPercentValue))
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

func checkPriceChange(oldBook, newBook utils.KindleBook, checkerConfigs *utils.CheckerConfigs) string {
	if oldBook.CurrentPrice == 0 {
		return ""
	}

	priceDiff := newBook.CurrentPrice - oldBook.CurrentPrice

	baseMessage := fmt.Sprintf("%s\n価格変動: %.0f円 → %.0f円 (%.0f円)\n%s",
		newBook.Title, oldBook.CurrentPrice, newBook.CurrentPrice, priceDiff, newBook.URL)
	if priceDiff >= float64(checkerConfigs.SaleChecker.PriceChangeAmount) {
		return "📈 プチ値上がり情報: " + baseMessage
	} else if priceDiff <= -float64(checkerConfigs.SaleChecker.PriceChangeAmount) {
		return "📉 プチ値下がり情報: " + baseMessage
	} else {
		return ""
	}
}

func replaceProcessedSegment(allBooks, processedBooks []utils.KindleBook, startIndex, endIndex int) []utils.KindleBook {
	result := allBooks[:startIndex]
	result = append(result, processedBooks...)
	result = append(result, allBooks[endIndex:]...)
	return result
}

func logBookChanges(originalBooks, updatedBooks []utils.KindleBook) {
	originalMap := make(map[string]utils.KindleBook)
	for _, book := range originalBooks {
		originalMap[book.ASIN] = book
	}

	for _, newBook := range updatedBooks {
		oldBook, exists := originalMap[newBook.ASIN]
		if !exists {
			continue
		}

		compareAndLogBookChanges(oldBook, newBook)
	}
}
func compareAndLogBookChanges(oldBook, newBook utils.KindleBook) {
	oldVal := reflect.ValueOf(oldBook)
	newVal := reflect.ValueOf(newBook)
	bookType := reflect.TypeOf(oldBook)

	for i := 0; i < bookType.NumField(); i++ {
		field := bookType.Field(i)
		oldField := oldVal.Field(i).Interface()
		newField := newVal.Field(i).Interface()

		if !reflect.DeepEqual(oldField, newField) {
			log.Printf("[%s] %s - %s changed: %v → %v", newBook.ASIN, newBook.Title, field.Name, oldField, newField)
		}
	}
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
