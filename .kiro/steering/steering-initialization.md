# Steering初期化ガイドライン

## 作業開始時の必須プロセス

### ユーザーレベルSteering確認
作業を開始する前に、必ず以下のコマンドでユーザーレベルのsteeringルールを確認し、遵守すること：

```bash
ls -la ~/.kiro/steering
```

すべてのsteeringファイルの内容を確認：
```bash
cat ~/.kiro/steering/coding-standards.md
cat ~/.kiro/steering/communication-guidelines.md
cat ~/.kiro/steering/git-commands.md
cat ~/.kiro/steering/eslint-usage.md
cat ~/.kiro/steering/steering-management.md
```

### 必須確認項目
1. **コーディング規約の確認**
   - Go言語のビルド確認方法
   - 関数の配置順序ルール
   - 変数使用ガイドライン

2. **コミュニケーションガイドラインの確認**
   - 禁止用語（「完璧」「完全」など）
   - プロフェッショナルな言葉遣い
   - 作業範囲の厳格な遵守

3. **Gitコマンドルールの確認**
   - `--no-pager`オプションの必須使用
   - コミットポリシー（明示的指示のみ）
   - コミットメッセージ作成ルール

4. **ESLint使用方法の確認**（該当プロジェクトの場合）
   - ユーザーレベル設定ファイルの使用

5. **Steering管理ルールの確認**
   - ファイル変更時の一貫性チェック

### 実施タイミング
- **新しいセッション開始時**：必ず最初に実行
- **作業内容変更時**：関連するsteeringルールを再確認
- **不明な点がある時**：該当するsteeringファイルを参照

### 遵守の重要性
- すべてのsteeringルールは絶対遵守
- 特にgit-commands.mdのコミットポリシーは例外なく従う
- ルール違反は重大な問題として扱われる

このプロセスにより、一貫した品質と作業フローが保証される。

# Steering Initialization Guidelines

## Mandatory Process at Work Start

### User-Level Steering Verification
Before starting any work, always verify and comply with user-level steering rules using the following commands:

```bash
ls -la ~/.kiro/steering
```

Check all steering file contents:
```bash
cat ~/.kiro/steering/coding-standards.md
cat ~/.kiro/steering/communication-guidelines.md
cat ~/.kiro/steering/git-commands.md
cat ~/.kiro/steering/eslint-usage.md
cat ~/.kiro/steering/steering-management.md
```

### Required Verification Items
1. **Coding Standards Verification**
   - Go language build verification methods
   - Function placement order rules
   - Variable usage guidelines

2. **Communication Guidelines Verification**
   - Prohibited terms ("perfect", "complete", etc.)
   - Professional language usage
   - Strict adherence to work scope

3. **Git Command Rules Verification**
   - Mandatory use of `--no-pager` option
   - Commit policy (explicit instruction only)
   - Commit message creation rules

4. **ESLint Usage Verification** (for applicable projects)
   - Use of user-level configuration files

5. **Steering Management Rules Verification**
   - Consistency checks when modifying files

### Implementation Timing
- **New session start**: Always execute first
- **Work content changes**: Re-verify relevant steering rules
- **When unclear**: Reference applicable steering files

### Importance of Compliance
- All steering rules must be absolutely followed
- Especially git-commands.md commit policies must be followed without exception
- Rule violations are treated as serious issues

This process ensures consistent quality and workflow.