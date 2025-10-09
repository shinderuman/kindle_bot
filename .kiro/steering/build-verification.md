# ビルド確認ガイドライン

## プロジェクト固有のビルドスクリプト使用

### 必須ビルド確認方法
このプロジェクトでは、コード変更後のビルド確認に専用スクリプトを使用すること：

```bash
./scripts/build-check.sh
```

### スクリプトの機能
- 全てのコマンド（new-release-checker、paper-to-kindle-checker、sale-checker）を一括ビルド
- 実行ファイルを生成せずにコンパイルエラーのみをチェック
- 作業ディレクトリを汚さない
- ビルド結果を分かりやすく表示

### 使用タイミング
- **コード変更後**：必ずビルドスクリプトを実行
- **関数追加・修正後**：コンパイルエラーがないことを確認
- **リファクタリング後**：全てのコマンドが正常にビルドされることを確認
- **コミット前**：最終確認として実行

### 禁止事項
- **個別の`go build -o /dev/null`コマンドの使用は避ける**
- **スクリプトを使用せずに手動でビルド確認することは非効率**
- **一部のコマンドのみのビルド確認は不十分**

### 利点
- 一貫したビルド確認プロセス
- 全コマンドの確認漏れ防止
- 効率的な作業フロー
- メンテナンス性の向上

このスクリプトにより、プロジェクト全体の品質と一貫性が保証される。

# Build Verification Guidelines

## Project-Specific Build Script Usage

### Required Build Verification Method
This project uses a dedicated script for build verification after code changes:

```bash
./scripts/build-check.sh
```

### Script Features
- Batch build of all commands (new-release-checker, paper-to-kindle-checker, sale-checker)
- Check compilation errors without generating executable files
- Keep working directory clean
- Display build results clearly

### Usage Timing
- **After code changes**: Always run the build script
- **After function additions/modifications**: Verify no compilation errors
- **After refactoring**: Confirm all commands build successfully
- **Before commits**: Run as final verification

### Prohibited Actions
- **Avoid using individual `go build -o /dev/null` commands**
- **Manual build verification without the script is inefficient**
- **Partial command build verification is insufficient**

### Benefits
- Consistent build verification process
- Prevention of missed command verification
- Efficient workflow
- Improved maintainability

This script ensures project-wide quality and consistency.