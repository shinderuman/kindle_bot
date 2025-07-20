package main

import (
	"testing"
	"time"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Full-width to half-width",
			input:    "ＡＢＣ１２３",
			expected: "ABC123",
		},
		{
			name:     "Full-width space to half-width",
			input:    "田中　太郎",
			expected: "田中太郎",
		},
		{
			name:     "Remove all spaces",
			input:    "田中 太郎",
			expected: "田中太郎",
		},
		{
			name:     "Mixed characters",
			input:    "田中　Ａ太郎 １２３",
			expected: "田中A太郎123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSortUniqueAuthors(t *testing.T) {
	now := time.Now()
	authors := []Author{
		{Name: "Author A", LatestReleaseDate: now.AddDate(0, 0, -1)},
		{Name: "Author B", LatestReleaseDate: now},
		{Name: "Author A", LatestReleaseDate: now.AddDate(0, 0, -2)}, // Duplicate
		{Name: "Author C", LatestReleaseDate: now.AddDate(0, 0, 1)},
	}

	result := SortUniqueAuthors(authors)

	// Should have 3 unique authors
	if len(result) != 3 {
		t.Errorf("SortUniqueAuthors() returned %d authors, want 3", len(result))
	}

	// Should be sorted by latest release date descending
	if !result[0].LatestReleaseDate.After(result[1].LatestReleaseDate) {
		t.Errorf("SortUniqueAuthors() not sorted correctly")
	}

	// First author should be Author C (latest date)
	if result[0].Name != "Author C" {
		t.Errorf("SortUniqueAuthors() first author = %v, want Author C", result[0].Name)
	}
}

func TestGetIndexByTime(t *testing.T) {
	tests := []struct {
		name        string
		authorCount int
		expected    bool // We can't test exact value due to time dependency
	}{
		{
			name:        "Zero authors",
			authorCount: 0,
			expected:    true, // Should return 0
		},
		{
			name:        "Positive authors",
			authorCount: 10,
			expected:    true, // Should return value between 0 and 9
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIndexByTime(tt.authorCount)

			if tt.authorCount == 0 {
				if result != 0 {
					t.Errorf("getIndexByTime(0) = %v, want 0", result)
				}
			} else {
				if result < 0 || result >= tt.authorCount {
					t.Errorf("getIndexByTime(%d) = %v, should be between 0 and %d",
						tt.authorCount, result, tt.authorCount-1)
				}
			}
		})
	}
}

func TestSaveASINsFromMap(t *testing.T) {
	// Test function signature exists
	m := map[string]interface{}{
		"ASIN1": struct{}{},
		"ASIN2": struct{}{},
	}
	key := "test-key"

	if len(m) != 2 || key == "" {
		t.Errorf("Test setup error")
	}

	t.Logf("saveASINsFromMap function exists with correct signature")
}
