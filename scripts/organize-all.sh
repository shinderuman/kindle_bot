#!/bin/bash

set -e

# スクリプトのディレクトリを取得
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "🔧 Organizing all checker data..."

# チェッカーリスト
checkers=("new-release-checker" "paper-to-kindle-checker" "sale-checker")

for checker in "${checkers[@]}"; do
    echo "📋 Organizing $checker..."
    cd "$PROJECT_DIR" && go run "cmd/$checker/main.go" -o
done

echo "🎉 All checkers organized successfully!"