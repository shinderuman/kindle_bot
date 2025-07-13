# Kindle Bot

A set of AWS Lambda functions written in Go that monitors and notifies about Kindle book releases, discounts, and updates using the PA-API.

## Features

* Checks if paper books now have Kindle editions (via `paper_to_kindle_checker.go`)
* Detects sale prices of Kindle books (via `sale_checker.go`)
* Finds new releases from favorite authors (via `new_release_checker.go`)
* Posts updates to Mastodon
* Sends alerts to Slack
* Stores data in S3 and tracks metrics in CloudWatch

## Requirements

* Go 1.20+
* AWS account with IAM permissions for Lambda, S3, SSM, CloudWatch
* PA-API (Amazon Product Advertising API) credentials
* Slack Bot Token and Channel IDs
* Mastodon access token and server details

## Setup

1. **Clone the repository:**

   ```bash
   git clone https://github.com/shinderuman/kindle_bot.git
   cd kindle_bot
   ```

2. **Configure secrets in AWS SSM Parameter Store:**

   * Parameters should be stored under `/myapp/plain/` and `/myapp/secure/`
   * Example keys:

     * `/myapp/plain/S3_BUCKET_NAME`
     * `/myapp/secure/AMAZON_ACCESS_KEY`

3. **Build and deploy each Lambda function (example):**

   ```bash
   GOOS=linux GOARCH=amd64 go build -o paper_to_kindle_checker paper_to_kindle_checker.go
   zip paper_to_kindle_checker.zip paper_to_kindle_checker
   aws lambda update-function-code --function-name paper_to_kindle_checker --zip-file fileb://paper_to_kindle_checker.zip
   ```

4. **Configure CloudWatch schedule/event triggers as needed.**

## File Overview

* `utils/` - shared utility functions (SSM, S3, PA-API, CloudWatch, Slack, Mastodon)
* `paper_to_kindle_checker.go` - main logic for detecting Kindle versions of paper books
* `sale_checker.go` - logic for detecting sale prices
* `new_release_checker.go` - logic for detecting new Kindle releases by authors

## License

MIT

---

# Kindle Bot（日本語）

Go で書かれた AWS Lambda 関数のセットで、PA-API を利用して Kindle 本の新刊、セール、紙書籍から Kindle 版への移行などを検知し、通知します。

## 主な機能

* 紙書籍に Kindle 版が出たかを検出（`paper_to_kindle_checker.go`）
* Kindle 本の値下げを検出（`sale_checker.go`）
* 著者の新刊 Kindle 本を検出（`new_release_checker.go`）
* Mastodon への投稿
* Slack への通知
* S3 によるデータ保存、CloudWatch によるメトリクス記録

## 必要な環境

* Go 1.20 以上
* AWS アカウント（Lambda, S3, SSM, CloudWatch への IAM 権限が必要）
* PA-API の認証情報（アクセスキーなど）
* Slack Bot のトークンとチャンネル ID
* Mastodon のアクセストークンとサーバー情報

## セットアップ手順

1. **リポジトリをクローン**

   ```bash
   git clone https://github.com/shinderuman/kindle_bot.git
   cd kindle_bot
   ```

2. **AWS SSM にシークレット情報を保存**

   * `/myapp/plain/` と `/myapp/secure/` 以下に設定します
   * 例：

     * `/myapp/plain/S3_BUCKET_NAME`
     * `/myapp/secure/AMAZON_ACCESS_KEY`

3. **各 Lambda 関数をビルドしてデプロイ**（例）

   ```bash
   GOOS=linux GOARCH=amd64 go build -o paper_to_kindle_checker paper_to_kindle_checker.go
   zip paper_to_kindle_checker.zip paper_to_kindle_checker
   aws lambda update-function-code --function-name paper_to_kindle_checker --zip-file fileb://paper_to_kindle_checker.zip
   ```

4. **必要に応じて CloudWatch イベントを設定してください**

## ファイル構成

* `utils/`：共通ユーティリティ（SSM, S3, PA-API, CloudWatch, Slack, Mastodon）
* `paper_to_kindle_checker.go`：紙書籍の Kindle 化を検出
* `sale_checker.go`：Kindle 本のセール価格を検出
* `new_release_checker.go`：著者の新刊 Kindle 本を検出

## ライセンス

MIT
