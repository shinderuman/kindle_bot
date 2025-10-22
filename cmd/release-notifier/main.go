package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"kindle_bot/utils"
)

func main() {
	utils.Run(process)
}

func process() error {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return err
	}

	today := time.Now().In(time.FixedZone("JST", 9*60*60))
	log.Printf("Checking for books released on %s", today.Format("2006-01-02"))

	allBooks, err := getAllBooks(cfg)
	if err != nil {
		return err
	}

	processAndNotifyTodayBooks(allBooks, today)

	return nil
}

func getAllBooks(cfg aws.Config) ([]utils.KindleBook, error) {
	notifiedBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3NotifiedObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get books from notified ASINs: %w", err)
	}

	unprocessedBooks, err := utils.FetchASINs(cfg, utils.EnvConfig.S3UnprocessedObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get books from unprocessed ASINs: %w", err)
	}

	return append(notifiedBooks, unprocessedBooks...), nil
}

func processAndNotifyTodayBooks(books []utils.KindleBook, today time.Time) {
	seen := make(map[string]struct{})

	for _, book := range books {
		bookDate := book.ReleaseDate.Time.In(time.FixedZone("JST", 9*60*60))
		if !isSameDate(bookDate, today) {
			continue
		}

		if _, exists := seen[book.ASIN]; exists {
			continue
		}

		seen[book.ASIN] = struct{}{}

		log.Printf("Notifying book [%s]: %s - %s", bookDate.Format("2006-01-02"), book.Title, book.URL)
		utils.LogAndNotify(formatSingleBookMessage(book), true)
	}
}

func isSameDate(date1, date2 time.Time) bool {
	y1, m1, d1 := date1.Date()
	y2, m2, d2 := date2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func formatSingleBookMessage(book utils.KindleBook) string {
	return fmt.Sprintf("📚 本日発売の書籍\n%s\n%s", book.Title, book.URL)
}
