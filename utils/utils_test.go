package utils

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/goark/pa-api/entity"
)

func TestUniqueASINs(t *testing.T) {
	tests := []struct {
		name     string
		input    []KindleBook
		expected []KindleBook
	}{
		{
			name: "重複するASINを除去",
			input: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "ASIN2", Title: "本2"},
				{ASIN: "ASIN1", Title: "本1重複"},
				{ASIN: "ASIN3", Title: "本3"},
			},
			expected: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "ASIN2", Title: "本2"},
				{ASIN: "ASIN3", Title: "本3"},
			},
		},
		{
			name:     "空のスライス",
			input:    []KindleBook{},
			expected: []KindleBook{},
		},
		{
			name: "重複なし",
			input: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "ASIN2", Title: "本2"},
			},
			expected: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "ASIN2", Title: "本2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UniqueASINs(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("UniqueASINs() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestChunkedASINs(t *testing.T) {
	tests := []struct {
		name     string
		books    []KindleBook
		size     int
		expected [][]string
	}{
		{
			name: "正常なチャンク分割",
			books: []KindleBook{
				{ASIN: "ASIN1"},
				{ASIN: "ASIN2"},
				{ASIN: "ASIN3"},
				{ASIN: "ASIN4"},
				{ASIN: "ASIN5"},
			},
			size: 2,
			expected: [][]string{
				{"ASIN1", "ASIN2"},
				{"ASIN3", "ASIN4"},
				{"ASIN5"},
			},
		},
		{
			name: "サイズが本の数より大きい場合",
			books: []KindleBook{
				{ASIN: "ASIN1"},
				{ASIN: "ASIN2"},
			},
			size: 5,
			expected: [][]string{
				{"ASIN1", "ASIN2"},
			},
		},
		{
			name:     "空のスライス",
			books:    []KindleBook{},
			size:     2,
			expected: [][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChunkedASINs(tt.books, tt.size)

			// 空のスライスの場合は長さをチェック
			if len(tt.books) == 0 {
				if len(result) != 0 {
					t.Errorf("ChunkedASINs() for empty input should return empty slice, got %v", result)
				}
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ChunkedASINs() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSortByReleaseDate(t *testing.T) {
	time1 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)
	time3 := time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    []KindleBook
		expected []KindleBook
	}{
		{
			name: "発売日順でソート（新しい順）",
			input: []KindleBook{
				{ASIN: "ASIN1", Title: "本1", ReleaseDate: entity.Date{Time: time1}},
				{ASIN: "ASIN3", Title: "本3", ReleaseDate: entity.Date{Time: time3}},
				{ASIN: "ASIN2", Title: "本2", ReleaseDate: entity.Date{Time: time2}},
			},
			expected: []KindleBook{
				{ASIN: "ASIN3", Title: "本3", ReleaseDate: entity.Date{Time: time3}},
				{ASIN: "ASIN2", Title: "本2", ReleaseDate: entity.Date{Time: time2}},
				{ASIN: "ASIN1", Title: "本1", ReleaseDate: entity.Date{Time: time1}},
			},
		},
		{
			name: "同じ発売日の場合はタイトル順",
			input: []KindleBook{
				{ASIN: "ASIN2", Title: "本C", ReleaseDate: entity.Date{Time: time1}},
				{ASIN: "ASIN1", Title: "本A", ReleaseDate: entity.Date{Time: time1}},
				{ASIN: "ASIN3", Title: "本B", ReleaseDate: entity.Date{Time: time1}},
			},
			expected: []KindleBook{
				{ASIN: "ASIN1", Title: "本A", ReleaseDate: entity.Date{Time: time1}},
				{ASIN: "ASIN3", Title: "本B", ReleaseDate: entity.Date{Time: time1}},
				{ASIN: "ASIN2", Title: "本C", ReleaseDate: entity.Date{Time: time1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 元のスライスをコピーしてソート
			result := make([]KindleBook, len(tt.input))
			copy(result, tt.input)
			SortByReleaseDate(result)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("SortByReleaseDate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetBook(t *testing.T) {
	books := []KindleBook{
		{ASIN: "ASIN1", Title: "本1", CurrentPrice: 1000},
		{ASIN: "ASIN2", Title: "本2", CurrentPrice: 1500},
		{ASIN: "ASIN3", Title: "本3", CurrentPrice: 2000},
	}

	tests := []struct {
		name     string
		asin     string
		expected KindleBook
	}{
		{
			name:     "存在するASIN",
			asin:     "ASIN2",
			expected: KindleBook{ASIN: "ASIN2", Title: "本2", CurrentPrice: 1500},
		},
		{
			name:     "存在しないASIN",
			asin:     "ASIN999",
			expected: KindleBook{}, // ゼロ値
		},
		{
			name:     "空文字列",
			asin:     "",
			expected: KindleBook{}, // ゼロ値
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBook(tt.asin, books)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("GetBook() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsLambda(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "Lambda環境",
			envValue: "test-function",
			expected: true,
		},
		{
			name:     "非Lambda環境",
			envValue: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 環境変数を一時的に設定
			originalValue := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")

			if tt.envValue != "" {
				os.Setenv("AWS_LAMBDA_FUNCTION_NAME", tt.envValue)
			} else {
				os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
			}

			// テスト実行
			result := IsLambda()

			// 環境変数を元に戻す
			if originalValue != "" {
				os.Setenv("AWS_LAMBDA_FUNCTION_NAME", originalValue)
			} else {
				os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
			}

			if result != tt.expected {
				t.Errorf("IsLambda() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAppendFallbackBooks(t *testing.T) {
	original := []KindleBook{
		{ASIN: "ASIN1", Title: "本1"},
		{ASIN: "ASIN2", Title: "本2"},
		{ASIN: "ASIN3", Title: "本3"},
	}

	tests := []struct {
		name     string
		asins    []string
		expected []KindleBook
	}{
		{
			name:  "存在するASINのみ",
			asins: []string{"ASIN1", "ASIN3"},
			expected: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "ASIN3", Title: "本3"},
			},
		},
		{
			name:  "存在しないASINを含む",
			asins: []string{"ASIN1", "ASIN999"},
			expected: []KindleBook{
				{ASIN: "ASIN1", Title: "本1"},
				{ASIN: "", Title: ""}, // ゼロ値
			},
		},
		{
			name:     "空のASINリスト",
			asins:    []string{},
			expected: []KindleBook{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendFallbackBooks(tt.asins, original)

			// 空のASINリストの場合は長さをチェック
			if len(tt.asins) == 0 {
				if len(result) != 0 {
					t.Errorf("AppendFallbackBooks() for empty input should return empty slice, got %v", result)
				}
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("AppendFallbackBooks() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ベンチマークテスト
func BenchmarkUniqueASINs(b *testing.B) {
	books := make([]KindleBook, 1000)
	for i := 0; i < 1000; i++ {
		books[i] = KindleBook{
			ASIN:  fmt.Sprintf("ASIN%d", i%100), // 重複を作る
			Title: fmt.Sprintf("本%d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UniqueASINs(books)
	}
}

func BenchmarkSortByReleaseDate(b *testing.B) {
	books := make([]KindleBook, 100)
	for i := 0; i < 100; i++ {
		books[i] = KindleBook{
			ASIN:        fmt.Sprintf("ASIN%d", i),
			Title:       fmt.Sprintf("本%d", i),
			ReleaseDate: entity.Date{Time: time.Now().Add(time.Duration(i) * time.Hour)},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testBooks := make([]KindleBook, len(books))
		copy(testBooks, books)
		SortByReleaseDate(testBooks)
	}
}

func BenchmarkGetBook(b *testing.B) {
	books := make([]KindleBook, 1000)
	for i := 0; i < 1000; i++ {
		books[i] = KindleBook{
			ASIN:  fmt.Sprintf("ASIN%d", i),
			Title: fmt.Sprintf("本%d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBook("ASIN500", books)
	}
}
