package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	appconfig "kindle_bot/internal/config"
	"kindle_bot/pkg/models"
)

func InitAWSConfig() (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(appconfig.EnvConfig.S3Region),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %v", err)
	}
	return cfg, nil
}

func FetchASINs(cfg aws.Config, objectKey string) ([]models.KindleBook, error) {
	body, err := GetS3Object(cfg, objectKey)
	if err != nil {
		return nil, err
	}

	var ASINs []models.KindleBook
	if err := json.Unmarshal(body, &ASINs); err != nil {
		return nil, err
	}

	return ASINs, nil
}

func GetS3Object(cfg aws.Config, objectKey string) ([]byte, error) {
	client := s3.NewFromConfig(cfg)

	input := &s3.GetObjectInput{
		Bucket: aws.String(appconfig.EnvConfig.S3BucketName),
		Key:    aws.String(objectKey),
	}

	resp, err := client.GetObject(context.TODO(), input)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func PutS3Object(cfg aws.Config, body, objectKey string) error {
	client := s3.NewFromConfig(cfg)

	_, err := client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(appconfig.EnvConfig.S3BucketName),
		Key:         aws.String(objectKey),
		Body:        strings.NewReader(body),
		ACL:         types.ObjectCannedACLPrivate,
		ContentType: aws.String("application/json"),
	})
	return err
}

func SaveASINs(cfg aws.Config, ASINs []models.KindleBook, objectKey string) error {
	prettyJSON, err := json.MarshalIndent(ASINs, "", "    ")
	if err != nil {
		return err
	}

	return PutS3Object(cfg, strings.ReplaceAll(string(prettyJSON), `\u0026`, "&"), objectKey)
}

func UniqueASINs(slice []models.KindleBook) []models.KindleBook {
	seen := make(map[string]struct{})
	result := []models.KindleBook{}

	for _, s := range slice {
		if _, exists := seen[s.ASIN]; !exists {
			seen[s.ASIN] = struct{}{}
			result = append(result, s)
		}
	}

	return result
}

func ChunkedASINs(books []models.KindleBook, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(books); i += size {
		end := i + size
		if end > len(books) {
			end = len(books)
		}
		var chunk []string
		for _, book := range books[i:end] {
			chunk = append(chunk, book.ASIN)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func SortByReleaseDate(books []models.KindleBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].ReleaseDate.Time.After(books[j].ReleaseDate.Time) {
			return true
		} else if books[i].ReleaseDate.Time.Equal(books[j].ReleaseDate.Time) {
			return books[i].Title < books[j].Title
		}
		return false
	})
}

func GetBook(ASIN string, slice []models.KindleBook) models.KindleBook {
	for _, s := range slice {
		if ASIN == s.ASIN {
			return s
		}
	}
	return models.KindleBook{}
}

func AppendFallbackBooks(asins []string, original []models.KindleBook) []models.KindleBook {
	var result []models.KindleBook
	for _, asin := range asins {
		result = append(result, GetBook(asin, original))
	}
	return result
}
