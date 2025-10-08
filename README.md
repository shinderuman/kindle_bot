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

## Execution Intervals and Configuration Management

### Execution Architecture

All programs use a unified execution architecture:
- **CloudWatch Events**: Set to run every minute (1 minute interval)
- **Internal Control**: Each program uses configuration-based interval control
- **Benefits**: Dynamic interval adjustment without Lambda cron changes

### Default Configuration Intervals

| Program | Default Interval | Configuration Field | Purpose |
|---------|-----------------|-------------------|---------|
| `new-release-checker` | 7 days | `CycleDays` | Check for new releases from authors |
| `paper-to-kindle-checker` | 1 day | `CycleDays` | Check if paper books have Kindle editions |
| `sale-checker` | 2 minutes | `ExecutionIntervalMinutes` | Monitor Kindle book sales with 10-book batches |

### Configuration Management

Each program uses S3-based JSON configuration for dynamic settings management:

#### Configuration File Location
- **S3 Object Key**: Defined by `S3CheckerConfigObjectKey` in config.json (e.g., "checker_configs.json")
- **Format**: JSON file containing configuration for all checkers
- **Benefits**: Dynamic configuration changes without code redeployment

#### Dynamic Control Features
- **Enable/Disable**: Each checker can be individually enabled or disabled using the `Enabled` flag
- **Maintenance Mode**: Temporarily disable problematic checkers without redeployment
- **Selective Operation**: Run only specific checkers during testing or troubleshooting

#### Example Configuration JSON
```json
{
  "SaleChecker": {
    "Enabled": true,
    "GistID": "your-sale-checker-gist-id",
    "GistFilename": "sale-books.md",
    "ExecutionIntervalMinutes": 2,
    "GetItemsPaapiRetryCount": 3,
    "GetItemsInitialRetrySeconds": 30
  },
  "NewReleaseChecker": {
    "Enabled": true,
    "GistID": "your-new-release-checker-gist-id",
    "GistFilename": "authors.md",
    "CycleDays": 7.0,
    "SearchItemsPaapiRetryCount": 3,
    "SearchItemsInitialRetrySeconds": 2,
    "GetItemsPaapiRetryCount": 3,
    "GetItemsInitialRetrySeconds": 2
  },
  "PaperToKindleChecker": {
    "Enabled": true,
    "GistID": "your-paper-to-kindle-checker-gist-id",
    "GistFilename": "paper-books.md",
    "CycleDays": 1.0,
    "SearchItemsPaapiRetryCount": 5,
    "SearchItemsInitialRetrySeconds": 2,
    "GetItemsPaapiRetryCount": 5,
    "GetItemsInitialRetrySeconds": 2
  }
}
```

#### Configuration Structure

**sale-checker**
- `Enabled` (default: true) - Enable/disable checker execution
- `GistID` - GitHub Gist ID for sale book list
- `GistFilename` - Gist filename for sale book list
- `ExecutionIntervalMinutes` (default: 2) - Execution interval in minutes (must be divisor of 60)
- `GetItemsPaapiRetryCount` (default: 3) - PA-API retry count for GetItems requests
- `GetItemsInitialRetrySeconds` (default: 30) - Initial retry delay for GetItems requests

**new-release-checker**
- `Enabled` (default: true) - Enable/disable checker execution
- `GistID` - GitHub Gist ID for author list
- `GistFilename` - Gist filename for author list
- `CycleDays` (default: 7.0) - Cycle duration in days for author processing
- `SearchItemsPaapiRetryCount` (default: 3) - SearchItems API retry count for author searches
- `SearchItemsInitialRetrySeconds` (default: 2) - Initial retry delay for SearchItems requests
- `GetItemsPaapiRetryCount` (default: 3) - GetItems API retry count
- `GetItemsInitialRetrySeconds` (default: 2) - Initial retry delay for GetItems requests

**paper-to-kindle-checker**
- `Enabled` (default: true) - Enable/disable checker execution
- `GistID` - GitHub Gist ID for paper book list
- `GistFilename` - Gist filename for paper book list
- `CycleDays` (default: 1.0) - Cycle duration in days for book processing
- `SearchItemsPaapiRetryCount` (default: 5) - SearchItems API retry count for Kindle edition searches
- `SearchItemsInitialRetrySeconds` (default: 2) - Initial retry delay for SearchItems requests
- `GetItemsPaapiRetryCount` (default: 5) - GetItems API retry count
- `GetItemsInitialRetrySeconds` (default: 2) - Initial retry delay for GetItems requests

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

## 実行間隔と設定管理

### 実行アーキテクチャ

全プログラムで統一された実行アーキテクチャを使用：
- **CloudWatch Events**: 毎分実行に設定（1分間隔）
- **内部制御**: 各プログラムが設定ベースの間隔制御を使用
- **利点**: Lambdaのcron設定変更なしで動的な間隔調整が可能

### デフォルト設定間隔

| プログラム | デフォルト間隔 | 設定フィールド | 目的 |
|-----------|---------------|---------------|------|
| `new-release-checker` | 7日 | `CycleDays` | 著者の新刊チェック |
| `paper-to-kindle-checker` | 1日 | `CycleDays` | 紙書籍のKindle版チェック |
| `sale-checker` | 2分 | `ExecutionIntervalMinutes` | Kindle本のセール監視（10件ずつバッチ処理） |

### 設定管理

各プログラムは動的設定管理のためにS3ベースのJSON設定を使用します：

#### 設定ファイルの場所
- **S3オブジェクトキー**: config.jsonの`S3CheckerConfigObjectKey`で定義（例: "checker_configs.json"）
- **形式**: 全checkerの設定を含むJSONファイル
- **利点**: コードの再デプロイなしで動的な設定変更が可能

#### 動的制御機能
- **有効/無効制御**: `Enabled`フラグで各checkerを個別に有効/無効化可能
- **メンテナンスモード**: 問題のあるcheckerを再デプロイなしで一時的に無効化
- **選択的実行**: テストやトラブルシューティング時に特定のcheckerのみ実行

#### 設定JSONの例
```json
{
  "SaleChecker": {
    "Enabled": true,
    "GistID": "your-sale-checker-gist-id",
    "GistFilename": "sale-books.md",
    "ExecutionIntervalMinutes": 2,
    "GetItemsPaapiRetryCount": 3,
    "GetItemsInitialRetrySeconds": 30
  },
  "NewReleaseChecker": {
    "Enabled": true,
    "GistID": "your-new-release-checker-gist-id",
    "GistFilename": "authors.md",
    "CycleDays": 7.0,
    "SearchItemsPaapiRetryCount": 3,
    "SearchItemsInitialRetrySeconds": 2,
    "GetItemsPaapiRetryCount": 3,
    "GetItemsInitialRetrySeconds": 2
  },
  "PaperToKindleChecker": {
    "Enabled": true,
    "GistID": "your-paper-to-kindle-checker-gist-id",
    "GistFilename": "paper-books.md",
    "CycleDays": 1.0,
    "SearchItemsPaapiRetryCount": 5,
    "SearchItemsInitialRetrySeconds": 2,
    "GetItemsPaapiRetryCount": 5,
    "GetItemsInitialRetrySeconds": 2
  }
}
```

#### 設定構造

**sale-checker**
- `Enabled` (デフォルト: true) - checkerの実行有効/無効
- `GistID` - セール書籍リスト用のGitHub Gist ID
- `GistFilename` - セール書籍リスト用のGistファイル名
- `ExecutionIntervalMinutes` (デフォルト: 2) - 実行間隔（分単位、60の約数である必要がある）
- `GetItemsPaapiRetryCount` (デフォルト: 3) - GetItemsリクエストのPA-APIリトライ回数
- `GetItemsInitialRetrySeconds` (デフォルト: 30) - GetItemsリクエストの初期リトライ遅延秒数

**new-release-checker**
- `Enabled` (デフォルト: true) - checkerの実行有効/無効
- `GistID` - 著者リスト用のGitHub Gist ID
- `GistFilename` - 著者リスト用のGistファイル名
- `CycleDays` (デフォルト: 7.0) - 著者処理のサイクル日数
- `SearchItemsPaapiRetryCount` (デフォルト: 3) - 著者検索時のSearchItems APIリトライ回数
- `SearchItemsInitialRetrySeconds` (デフォルト: 2) - SearchItemsリクエストの初期リトライ遅延秒数
- `GetItemsPaapiRetryCount` (デフォルト: 3) - GetItems APIリトライ回数
- `GetItemsInitialRetrySeconds` (デフォルト: 2) - GetItemsリクエストの初期リトライ遅延秒数

**paper-to-kindle-checker**
- `Enabled` (デフォルト: true) - checkerの実行有効/無効
- `GistID` - 紙書籍リスト用のGitHub Gist ID
- `GistFilename` - 紙書籍リスト用のGistファイル名
- `CycleDays` (デフォルト: 1.0) - 書籍処理のサイクル日数
- `SearchItemsPaapiRetryCount` (デフォルト: 5) - Kindle版検索時のSearchItems APIリトライ回数
- `SearchItemsInitialRetrySeconds` (デフォルト: 2) - SearchItemsリクエストの初期リトライ遅延秒数
- `GetItemsPaapiRetryCount` (デフォルト: 5) - GetItems APIリトライ回数
- `GetItemsInitialRetrySeconds` (デフォルト: 2) - GetItemsリクエストの初期リトライ遅延秒数

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
