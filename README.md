# Kindle Bot

A set of AWS Lambda functions written in Go that monitors and notifies about Kindle book releases, discounts, and updates using the PA-API.

## Features

* Checks if paper books now have Kindle editions (via `cmd/paper-to-kindle-checker`)
* Detects sale prices of Kindle books (via `cmd/sale-checker`)
* Finds new releases from favorite authors (via `cmd/new-release-checker`)
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

2. **Configure local settings (for local development):**

   ```bash
   # Copy the example configuration file
   cp config.json.example config.json
   
   # Edit config.json with your actual credentials
   # Note: config.json is ignored by git for security
   ```

3. **Configure secrets in AWS SSM Parameter Store (for Lambda deployment):**

   * Parameters should be stored under `/myapp/plain/` and `/myapp/secure/`
   * Example keys:

     * `/myapp/plain/S3_BUCKET_NAME`
     * `/myapp/secure/AMAZON_ACCESS_KEY`

4. **Configure deployment settings:**

   ```bash
   # Copy the deployment configuration template
   cp .env.example .env
   
   # Edit .env with your AWS profile and Lambda function names
   ```

5. **Build and deploy Lambda functions:**

   ```bash
   # Deploy individual functions
   ./scripts/deploy.sh paper-to-kindle-checker
   ./scripts/deploy.sh new-release-checker
   ./scripts/deploy.sh sale-checker
   
   # Deploy all functions at once
   ./scripts/deploy.sh all
   
   # Build only (without deployment)
   ./scripts/deploy.sh paper-to-kindle-checker -b
   ./scripts/deploy.sh new-release-checker --build-only
   ```

6. **Enable tab completion (optional):**

   ```bash
   # For zsh users
   source scripts/deploy-completion.zsh
   echo "source $(pwd)/scripts/deploy-completion.zsh" >> ~/.zshrc
   
   # For bash users
   source scripts/deploy-completion.bash
   echo "source $(pwd)/scripts/deploy-completion.bash" >> ~/.bashrc
   
   # Now you can use tab completion:
   # ./scripts/deploy.sh <TAB> -> shows: paper-to-kindle-checker, new-release-checker, sale-checker, all
   # ./scripts/deploy.sh paper-to-kindle-checker <TAB> -> shows: -b, --build-only, -h, --help
   ```

5. **Configure CloudWatch schedule/event triggers as needed.**

## Project Structure

```
kindle_bot/
├── cmd/                                    # Main applications
│   ├── new-release-checker/               # New release monitoring
│   │   └── main.go
│   ├── paper-to-kindle-checker/           # Paper to Kindle conversion checker
│   │   └── main.go
│   └── sale-checker/                      # Sale monitoring
│       └── main.go
├── scripts/                               # Deployment and utility scripts
│   ├── deploy.sh                          # Lambda deployment script
│   ├── deploy-completion.bash             # Bash completion for deploy.sh
│   └── deploy-completion.zsh              # Zsh completion for deploy.sh
├── utils/                                 # Shared utility functions
│   ├── models.go                          # Data models
│   └── utils.go                           # Common utilities
├── .env.example                           # Environment configuration template
└── config.json.example                    # Configuration template
```

## Usage

### Local Development

Run applications locally for testing:

```bash
# Run new release checker
go run ./cmd/new-release-checker

# Run paper to kindle checker
go run ./cmd/paper-to-kindle-checker

# Run sale checker
go run ./cmd/sale-checker
```

### Building

Build all applications:

```bash
# Build for local use
go build ./cmd/new-release-checker
go build ./cmd/paper-to-kindle-checker
go build ./cmd/sale-checker

# Build for Lambda deployment (Linux)
GOOS=linux GOARCH=amd64 go build -o new-release-checker ./cmd/new-release-checker
GOOS=linux GOARCH=amd64 go build -o paper-to-kindle-checker ./cmd/paper-to-kindle-checker
GOOS=linux GOARCH=amd64 go build -o sale-checker ./cmd/sale-checker
```

## License

MIT

---

# Kindle Bot（日本語）

Go で書かれた AWS Lambda 関数のセットで、PA-API を利用して Kindle 本の新刊、セール、紙書籍から Kindle 版への移行などを検知し、通知します。

## 主な機能

* 紙書籍に Kindle 版が出たかを検出（`cmd/paper-to-kindle-checker`）
* Kindle 本の値下げを検出（`cmd/sale-checker`）
* 著者の新刊 Kindle 本を検出（`cmd/new-release-checker`）
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

2. **ローカル設定を構成（ローカル開発用）**

   ```bash
   # 設定ファイルのテンプレートをコピー
   cp config.json.example config.json
   
   # config.json を実際の認証情報で編集
   # 注意: config.json はセキュリティのため git で無視されます
   ```

3. **AWS SSM にシークレット情報を保存（Lambda デプロイ用）**

   * `/myapp/plain/` と `/myapp/secure/` 以下に設定します
   * 例：

     * `/myapp/plain/S3_BUCKET_NAME`
     * `/myapp/secure/AMAZON_ACCESS_KEY`

4. **デプロイ設定を構成**

   ```bash
   # デプロイ設定のテンプレートをコピー
   cp .env.example .env
   
   # .env を AWS プロファイルと Lambda 関数名で編集
   ```

5. **Lambda 関数をビルドしてデプロイ**

   ```bash
   # 個別の関数をデプロイ
   ./scripts/deploy.sh paper-to-kindle-checker
   ./scripts/deploy.sh new-release-checker
   ./scripts/deploy.sh sale-checker
   
   # 全関数を一括デプロイ
   ./scripts/deploy.sh all
   
   # ビルドのみ（デプロイなし）
   ./scripts/deploy.sh paper-to-kindle-checker -b
   ./scripts/deploy.sh new-release-checker --build-only
   ```

6. **タブ補完を有効にする（オプション）:**

   ```bash
   # zshユーザーの場合
   source scripts/deploy-completion.zsh
   echo "source $(pwd)/scripts/deploy-completion.zsh" >> ~/.zshrc
   
   # bashユーザーの場合
   source scripts/deploy-completion.bash
   echo "source $(pwd)/scripts/deploy-completion.bash" >> ~/.bashrc
   
   # これでタブ補完が使用可能:
   # ./scripts/deploy.sh <TAB> -> paper-to-kindle-checker, new-release-checker, sale-checker, all が表示
   # ./scripts/deploy.sh paper-to-kindle-checker <TAB> -> -b, --build-only, -h, --help が表示
   ```

5. **必要に応じて CloudWatch イベントを設定してください**

## プロジェクト構成

```
kindle_bot/
├── cmd/                                    # メインアプリケーション
│   ├── new-release-checker/               # 新刊監視
│   │   └── main.go
│   ├── paper-to-kindle-checker/           # 紙書籍→Kindle版チェッカー
│   │   └── main.go
│   └── sale-checker/                      # セール監視
│       └── main.go
├── scripts/                               # デプロイ・ユーティリティスクリプト
│   ├── deploy.sh                          # Lambda デプロイスクリプト
│   ├── deploy-completion.bash             # deploy.sh 用 Bash 補完
│   └── deploy-completion.zsh              # deploy.sh 用 Zsh 補完
├── utils/                                 # 共通ユーティリティ
│   ├── models.go                          # データモデル
│   └── utils.go                           # 共通機能
├── .env.example                           # 環境設定テンプレート
└── config.json.example                    # 設定ファイルのテンプレート
```

## 使用方法

### ローカル開発

テスト用にローカルでアプリケーションを実行：

```bash
# 新刊チェッカーを実行
go run ./cmd/new-release-checker

# 紙書籍→Kindle版チェッカーを実行
go run ./cmd/paper-to-kindle-checker

# セールチェッカーを実行
go run ./cmd/sale-checker
```

### ビルド

全アプリケーションをビルド：

```bash
# ローカル用ビルド
go build ./cmd/new-release-checker
go build ./cmd/paper-to-kindle-checker
go build ./cmd/sale-checker

# Lambda デプロイ用ビルド（Linux）
GOOS=linux GOARCH=amd64 go build -o new-release-checker ./cmd/new-release-checker
GOOS=linux GOARCH=amd64 go build -o paper-to-kindle-checker ./cmd/paper-to-kindle-checker
GOOS=linux GOARCH=amd64 go build -o sale-checker ./cmd/sale-checker
```

## ライセンス

MIT
