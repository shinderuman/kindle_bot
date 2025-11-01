package main

import (
	"encoding/json"
	"fmt"
	"io"
	"kindle_bot/utils"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	apiBaseURL = "https://affiliate.amazon.co.jp/reporting/table"

	// Regex patterns for token extraction
	bearerTokenPattern   = `"authorization":\s*"Bearer\s+([^"]+)"`
	csrfTokenPattern     = `"x-csrf-token":\s*"([^"]+)"`
	cookiePattern        = `"cookie":\s*"([^"]+)"`
	customerIDPattern    = `"customerid":\s*"([^"]+)"`
	marketplaceIDPattern = `"marketplaceid":\s*"([^"]+)"`
	programIDPattern     = `"programid":\s*"([^"]+)"`
	storeIDPattern       = `"storeid":\s*"([^"]+)"`
)

type AuthConfig struct {
	BearerToken   string
	CSRFToken     string
	Cookie        string
	CustomerID    string
	MarketplaceID string
	ProgramID     string
	StoreID       string
}

type Record struct {
	ProductTitle       string `json:"product_title"`
	ASIN               string `json:"asin"`
	ShippedItems       string `json:"shipped_items"`
	CommissionEarnings string `json:"commission_earnings"`
	Revenue            string `json:"revenue"`
	Price              string `json:"price"`
	FeeRate            string `json:"fee_rate"`
	ReturnedItems      string `json:"returned_items"`
	ReturnedRevenue    string `json:"returned_revenue"`
	ReturnedEarnings   string `json:"returned_earnings"`
}

type ReportResponse struct {
	Records []Record `json:"records"`
}

func main() {
	utils.Run(process)
}

func process() error {
	auth, err := loadAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to load authentication config: %w", err)
	}

	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	log.Printf("Checking earnings for date: %s", date)

	report, err := fetchReport(auth, date)
	if err != nil {
		return fmt.Errorf("failed to fetch report: %w", err)
	}

	message := generateEarningsReport(report.Records, date)
	if len(message) == 0 {
		log.Printf("No earnings found for %s", date)
		return nil
	}

	log.Printf("Earnings report:\n%s", message)

	return sendNotification(message)
}

func loadAuthConfig() (*AuthConfig, error) {
	cfg, err := utils.InitAWSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to init AWS config: %w", err)
	}

	content, err := utils.GetS3Object(cfg, utils.EnvConfig.S3AmazonAffiliateAuthObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read fetch.js from S3: %w", err)
	}

	return parseAuthTokens(string(content))
}

func parseAuthTokens(text string) (*AuthConfig, error) {
	tokens := map[string]string{
		"BearerToken":   bearerTokenPattern,
		"CSRFToken":     csrfTokenPattern,
		"Cookie":        cookiePattern,
		"CustomerID":    customerIDPattern,
		"MarketplaceID": marketplaceIDPattern,
		"ProgramID":     programIDPattern,
		"StoreID":       storeIDPattern,
	}

	values := make(map[string]string)
	for field, pattern := range tokens {
		value, err := extractToken(text, pattern)
		if err != nil {
			return nil, fmt.Errorf("%s not found in fetch.js", field)
		}
		values[field] = value
	}

	return &AuthConfig{
		BearerToken:   values["BearerToken"],
		CSRFToken:     values["CSRFToken"],
		Cookie:        values["Cookie"],
		CustomerID:    values["CustomerID"],
		MarketplaceID: values["MarketplaceID"],
		ProgramID:     values["ProgramID"],
		StoreID:       values["StoreID"],
	}, nil
}

func extractToken(text, pattern string) (string, error) {
	regex := regexp.MustCompile(pattern)
	match := regex.FindStringSubmatch(text)
	if len(match) < 2 {
		return "", fmt.Errorf("token not found with pattern: %s", pattern)
	}
	return match[1], nil
}

func fetchReport(auth *AuthConfig, date string) (*ReportResponse, error) {
	req, err := buildAPIRequest(auth, date)
	if err != nil {
		return nil, err
	}

	return executeAPIRequest(req)
}

func buildAPIRequest(auth *AuthConfig, date string) (*http.Request, error) {
	params := buildQueryParams(auth, date)
	fullURL := apiBaseURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("request creation failed: %w", err)
	}

	setRequestHeaders(auth, req)
	return req, nil
}

func buildQueryParams(auth *AuthConfig, date string) url.Values {
	params := url.Values{}
	params.Set("query[type]", "earnings")
	params.Set("query[start_date]", date)
	params.Set("query[end_date]", date)
	params.Set("query[tag_id]", "all")
	params.Set("query[order]", "desc")
	params.Set("query[device_type]", "all")
	params.Set("query[last_accessed_row_index]", "0")
	params.Set("query[group_by]", "day")
	params.Set("query[columns]", "product_title,price,fee_rate,shipped_items,revenue,commission_earnings,asin,returned_items,returned_revenue,returned_earnings")
	params.Set("query[group]", date)
	params.Set("query[skip]", "0")
	params.Set("query[next_token]", "")
	params.Set("query[sort]", "shipped_items")
	params.Set("query[limit]", "25")
	params.Set("store_id", auth.StoreID)
	return params
}

func setRequestHeaders(auth *AuthConfig, req *http.Request) {
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	req.Header.Set("Cookie", auth.Cookie)
	req.Header.Set("CustomerID", auth.CustomerID)
	req.Header.Set("Language", "ja_JP")
	req.Header.Set("Locale", "ja_JP")
	req.Header.Set("MarketplaceID", auth.MarketplaceID)
	req.Header.Set("ProgramID", auth.ProgramID)
	req.Header.Set("Referer", "https://affiliate.amazon.co.jp/p/reporting/earnings?ac-ms-src=summaryforthismonth")
	req.Header.Set("Roles", "Primary")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("StoreID", auth.StoreID)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("X-CSRF-Token", auth.CSRFToken)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
}

func executeAPIRequest(req *http.Request) (*ReportResponse, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("authentication failed (status 403). Authentication tokens have expired, please update the fetch.js file")
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var report ReportResponse
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &report, nil
}

func generateEarningsReport(records []Record, date string) string {
	var earningsMessages []string

	for _, record := range records {
		if hasCommissionEarnings(record) {
			earningsMessages = append(earningsMessages, formatEarningsMessage(record))
		}
	}

	if len(earningsMessages) == 0 {
		return ""
	}

	return fmt.Sprintf(`売上報告 (%s)

%s`, date, strings.Join(earningsMessages, "---\n"))
}

func formatEarningsMessage(record Record) string {
	return fmt.Sprintf(`📚 %s
ASIN: %s
紹介料: %s円
出荷数: %s
売上: %s円
`, record.ProductTitle, record.ASIN, record.CommissionEarnings, record.ShippedItems, record.Revenue)
}

func hasCommissionEarnings(record Record) bool {
	earnings, err := strconv.ParseFloat(record.CommissionEarnings, 64)
	if err != nil {
		return false
	}
	return earnings > 0
}

func sendNotification(message string) error {
	if err := utils.PostToSlack(message, utils.EnvConfig.SlackNoticeChannel); err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	return nil
}
