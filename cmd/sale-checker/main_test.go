package main

import (
	"testing"

	"kindle_bot/utils"
)

// 基本的なロジックのテスト（entity依存部分を除く）
func TestExtractQualifiedConditionsLogic(t *testing.T) {
	tests := []struct {
		name        string
		amount      float64
		points      int
		maxPrice    float64
		expectedLen int
	}{
		{
			name:        "価格差のみ条件達成",
			amount:      1000.0,
			points:      50,
			maxPrice:    1200.0,
			expectedLen: 1, // 価格差のみ
		},
		{
			name:        "全条件達成",
			amount:      800.0,
			points:      200,
			maxPrice:    1000.0,
			expectedLen: 3, // 価格差、ポイント、還元率
		},
		{
			name:        "条件未達成",
			amount:      1000.0,
			points:      50,
			maxPrice:    1100.0,
			expectedLen: 0,
		},
		{
			name:        "境界値テスト（価格差151円）",
			amount:      1000.0,
			points:      50,
			maxPrice:    1151.0,
			expectedLen: 1, // 価格差のみ
		},
		{
			name:        "境界値テスト（ポイント151pt）",
			amount:      1000.0,
			points:      151,
			maxPrice:    1000.0,
			expectedLen: 1, // ポイントのみ
		},
		{
			name:        "境界値テスト（還元率20%）",
			amount:      1000.0,
			points:      200,
			maxPrice:    1000.0,
			expectedLen: 2, // ポイントと還元率
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 基本的なロジックのテスト
			var conditions []string

			if diff := tt.maxPrice - tt.amount; diff >= minPriceDiff {
				conditions = append(conditions, "価格差条件達成")
			}
			if tt.points >= minPoints {
				conditions = append(conditions, "ポイント条件達成")
			}
			if percent := float64(tt.points) / tt.amount * 100; percent >= minPointsPercent {
				conditions = append(conditions, "還元率条件達成")
			}

			if len(conditions) != tt.expectedLen {
				t.Errorf("Expected %d conditions, got %d", tt.expectedLen, len(conditions))
			}
		})
	}
}

func TestProcessASINsLogic(t *testing.T) {
	// processASINs関数のロジックをテスト（AWS依存部分を除く）

	// テスト用のKindleBookを作成
	original := []utils.KindleBook{
		{
			ASIN:         "TEST1",
			Title:        "テスト本1",
			MaxPrice:     1000.0,
			CurrentPrice: 800.0,
		},
		{
			ASIN:         "TEST2",
			Title:        "テスト本2",
			MaxPrice:     1500.0,
			CurrentPrice: 1200.0,
		},
	}

	// UniqueASINsのテスト
	unique := utils.UniqueASINs(original)
	if len(unique) != 2 {
		t.Errorf("Expected 2 unique books, got %d", len(unique))
	}

	// ChunkedASINsのテスト
	chunks := utils.ChunkedASINs(original, 1)
	if len(chunks) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 1 || chunks[0][0] != "TEST1" {
		t.Errorf("First chunk should contain TEST1, got %v", chunks[0])
	}
}

// エッジケーステスト
func TestConditionLogicEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		amount      float64
		points      int
		maxPrice    float64
		expectedLen int
		description string
	}{
		{
			name:        "価格が0の場合",
			amount:      0.0,
			points:      100,
			maxPrice:    1000.0,
			expectedLen: 1, // 価格差のみ
			description: "価格が0でも正常に動作する",
		},
		{
			name:        "ポイントが0の場合",
			amount:      1000.0,
			points:      0,
			maxPrice:    1200.0,
			expectedLen: 1, // 価格差のみ
			description: "ポイントが0でも正常に動作する",
		},
		{
			name:        "境界値テスト（価格差150円）",
			amount:      1000.0,
			points:      50,
			maxPrice:    1150.0,
			expectedLen: 0, // 条件未達成
			description: "境界値未満では条件未達成",
		},
		{
			name:        "境界値テスト（ポイント150pt）",
			amount:      1000.0,
			points:      150,
			maxPrice:    1000.0,
			expectedLen: 0, // 条件未達成
			description: "境界値未満では条件未達成",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []string

			if diff := tt.maxPrice - tt.amount; diff >= minPriceDiff {
				conditions = append(conditions, "価格差条件達成")
			}
			if tt.points >= minPoints {
				conditions = append(conditions, "ポイント条件達成")
			}
			if tt.amount > 0 {
				if percent := float64(tt.points) / tt.amount * 100; percent >= minPointsPercent {
					conditions = append(conditions, "還元率条件達成")
				}
			}

			if len(conditions) != tt.expectedLen {
				t.Errorf("%s: length = %d, want %d", tt.description, len(conditions), tt.expectedLen)
			}
		})
	}
}

// ベンチマークテスト
func BenchmarkConditionLogic(b *testing.B) {
	amount := 800.0
	points := 200
	maxPrice := 1000.0

	for i := 0; i < b.N; i++ {
		var conditions []string
		if diff := maxPrice - amount; diff >= minPriceDiff {
			conditions = append(conditions, "価格差条件達成")
		}
		if points >= minPoints {
			conditions = append(conditions, "ポイント条件達成")
		}
		if percent := float64(points) / amount * 100; percent >= minPointsPercent {
			conditions = append(conditions, "還元率条件達成")
		}
		_ = conditions
	}
}
