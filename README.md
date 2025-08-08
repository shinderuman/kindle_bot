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

## Execution Intervals and Environment Variables

### Recommended Execution Intervals

| Program | CloudWatch Interval | Internal Cycle | Purpose |
|---------|-------------------|----------------|---------|
| `new-release-checker` | 1 minute | 7 days (weekly) | Check for new releases from authors |
| `paper-to-kindle-checker` | 1 minute | 1 day (daily) | Check if paper books have Kindle editions |
| `sale-checker` | Any interval | Sequential processing | Monitor Kindle book sales with 10-book batches |

### Environment Variables

Each program supports environment variables to customize execution behavior:

#### new-release-checker
- `NEW_RELEASE_PAAPI_RETRY_COUNT` (default: 3) - SearchItems API retry count for author searches
- `NEW_RELEASE_CYCLE_DAYS` (default: 7.0) - Cycle duration in days for author processing

#### paper-to-kindle-checker  
- `PAPER_TO_KINDLE_PAAPI_RETRY_COUNT` (default: 5) - SearchItems API retry count for Kindle edition searches
- `PAPER_TO_KINDLE_CYCLE_DAYS` (default: 1.0) - Cycle duration in days for book processing

#### sale-checker
- No environment variables required
- Processes 10 books per execution in file order
- Progress is automatically saved to S3 for continuation

#### Global (utils package)
- `GET_ITEMS_PAAPI_RETRY_COUNT` (default: 3) - PA-API retry count for GetItems requests

### Sequential Processing (sale-checker)

The sale-checker implements sequential batch processing to efficiently monitor book sales:

- **Batch Processing**: Processes exactly 10 books per execution
- **Progress Tracking**: Saves progress to S3 for automatic continuation
- **Sequential Order**: Processes books in file order
- **Automatic Continuation**: Next execution continues from where the previous one left off
- **Automatic Reset**: When reaching the end of the book list, automatically starts from the beginning

**Configuration Example**:
```bash
# CloudWatch Events: Any interval (e.g., 30 minutes)
# CloudWatch: rate(30 minutes)
```

**Note**: Set the execution interval considering PA-API retry processing time (up to several minutes). If the interval is too short, concurrent execution may occur and cause race conditions.

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

Build applications using the deployment script (recommended):

```bash
# Build individual functions for Lambda deployment
./scripts/deploy.sh paper-to-kindle-checker --build-only
./scripts/deploy.sh new-release-checker -b
./scripts/deploy.sh sale-checker --build-only

# Build all functions at once
./scripts/deploy.sh all --build-only
```

Or build manually:

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

## 実行間隔と環境変数

### 推奨実行間隔

| プログラム | CloudWatch間隔 | 内部サイクル | 目的 |
|-----------|---------------|-------------|------|
| `new-release-checker` | 1分 | 7日（週次） | 著者の新刊チェック |
| `paper-to-kindle-checker` | 1分 | 1日（日次） | 紙書籍のKindle版チェック |
| `sale-checker` | 任意の間隔 | 順次処理 | Kindle本のセール監視（10件ずつバッチ処理） |

### 環境変数

各プログラムは環境変数で実行動作をカスタマイズできます：

#### new-release-checker
- `NEW_RELEASE_PAAPI_RETRY_COUNT` (デフォルト: 3) - 著者検索時のSearchItems APIリトライ回数
- `NEW_RELEASE_CYCLE_DAYS` (デフォルト: 7.0) - 著者処理のサイクル日数

#### paper-to-kindle-checker  
- `PAPER_TO_KINDLE_PAAPI_RETRY_COUNT` (デフォルト: 5) - Kindle版検索時のSearchItems APIリトライ回数
- `PAPER_TO_KINDLE_CYCLE_DAYS` (デフォルト: 1.0) - 書籍処理のサイクル日数

#### sale-checker
- 環境変数は不要
- ファイル順で1回につき10件を処理
- 進捗は自動的にS3に保存され、次回実行時に継続

#### 全体共通 (utilsパッケージ)
- `GET_ITEMS_PAAPI_RETRY_COUNT` (デフォルト: 3) - GetItemsリクエストのPA-APIリトライ回数

### 順次処理 (sale-checker)

sale-checkerは順次バッチ処理を実装し、効率的に書籍のセール監視を行います：

- **バッチ処理**: 1回の実行で正確に10件を処理
- **進捗追跡**: S3に進捗を保存して自動継続
- **順次処理**: ファイル順で10件ずつ処理
- **自動継続**: 次回実行時は前回の続きから処理
- **自動リセット**: 書籍リストの最後に到達すると、自動的に最初から開始

**設定例**:
```bash
# CloudWatch Events: 任意の間隔（例：30分）
# CloudWatch: rate(30 minutes)
```

**注意**: 実行間隔はPA-APIのリトライ処理時間（最大数分）を考慮して設定してください。間隔が短すぎると同時実行が発生し、競合状態を引き起こす可能性があります。

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

デプロイスクリプトを使用したビルド（推奨）：

```bash
# 個別の関数をLambdaデプロイ用にビルド
./scripts/deploy.sh paper-to-kindle-checker --build-only
./scripts/deploy.sh new-release-checker -b
./scripts/deploy.sh sale-checker --build-only

# 全関数を一括ビルド
./scripts/deploy.sh all --build-only
```

または手動でビルド：

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
