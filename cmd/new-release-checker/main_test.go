package main

import (
	"testing"
	"time"
)

func TestGetIndexByTime(t *testing.T) {
	tests := []struct {
		name        string
		authorCount int
		expected    bool // expectedが範囲内かどうかをテスト
	}{
		{
			name:        "正常なケース",
			authorCount: 10,
			expected:    true,
		},
		{
			name:        "作者数が0の場合",
			authorCount: 0,
			expected:    true, // 0を返すはず
		},
		{
			name:        "作者数が1の場合",
			authorCount: 1,
			expected:    true, // 0を返すはず
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIndexByTime(tt.authorCount)

			if tt.authorCount <= 0 {
				if result != 0 {
					t.Errorf("getIndexByTime(%d) = %d, want 0", tt.authorCount, result)
				}
			} else {
				if result < 0 || result >= tt.authorCount {
					t.Errorf("getIndexByTime(%d) = %d, want 0 <= result < %d", tt.authorCount, result, tt.authorCount)
				}
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "全角英数字の正規化",
			input:    "ＡＢＣ１２３",
			expected: "ABC123",
		},
		{
			name:     "全角スペースの正規化",
			input:    "田中　太郎",
			expected: "田中太郎",
		},
		{
			name:     "半角スペースの除去",
			input:    "田中 太郎",
			expected: "田中太郎",
		},
		{
			name:     "複合パターン",
			input:    "田中　Ａ太郎 １２３",
			expected: "田中A太郎123",
		},
		{
			name:     "空文字列",
			input:    "",
			expected: "",
		},
		{
			name:     "スペースのみ",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSortUniqueAuthors(t *testing.T) {
	// テスト用の時刻を作成
	time1 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)
	time3 := time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    []Author
		expected []Author
	}{
		{
			name: "重複する作者の除去",
			input: []Author{
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
				{Name: "作者B", URL: "url2", LatestReleaseDate: time2},
				{Name: "作者A", URL: "url3", LatestReleaseDate: time3}, // 重複
			},
			expected: []Author{
				{Name: "作者B", URL: "url2", LatestReleaseDate: time2},
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
			},
		},
		{
			name: "発売日順のソート",
			input: []Author{
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
				{Name: "作者B", URL: "url2", LatestReleaseDate: time3},
				{Name: "作者C", URL: "url3", LatestReleaseDate: time2},
			},
			expected: []Author{
				{Name: "作者B", URL: "url2", LatestReleaseDate: time3},
				{Name: "作者C", URL: "url3", LatestReleaseDate: time2},
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
			},
		},
		{
			name: "同じ発売日の場合は名前順",
			input: []Author{
				{Name: "作者C", URL: "url3", LatestReleaseDate: time1},
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
				{Name: "作者B", URL: "url2", LatestReleaseDate: time1},
			},
			expected: []Author{
				{Name: "作者A", URL: "url1", LatestReleaseDate: time1},
				{Name: "作者B", URL: "url2", LatestReleaseDate: time1},
				{Name: "作者C", URL: "url3", LatestReleaseDate: time1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SortUniqueAuthors(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("SortUniqueAuthors() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			for i, author := range result {
				if author.Name != tt.expected[i].Name ||
					author.URL != tt.expected[i].URL ||
					!author.LatestReleaseDate.Equal(tt.expected[i].LatestReleaseDate) {
					t.Errorf("SortUniqueAuthors()[%d] = %+v, want %+v", i, author, tt.expected[i])
				}
			}
		})
	}
}

// ベンチマークテスト
func BenchmarkNormalizeName(b *testing.B) {
	testName := "田中　Ａ太郎 １２３"
	for i := 0; i < b.N; i++ {
		normalizeName(testName)
	}
}

func BenchmarkGetIndexByTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getIndexByTime(100)
	}
}
