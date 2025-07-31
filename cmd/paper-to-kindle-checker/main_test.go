package main

import (
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove volume number and publisher info",
			input:    "勇者に全部奪われた俺は勇者の母親とパーティを組みました! 6 (MFC)",
			expected: "勇者に全部奪われた俺は勇者の母親とパーティを組みました!",
		},
		{
			name:     "Remove long volume number and publisher info",
			input:    "左遷された無能王子は実力を隠したい6 ~二度転生した最強賢者、今世では楽したいので手を抜いてたら、王家を追放された。今更帰ってこいと言われても遅い、領民に実力がバレて、実家に帰してくれないから……~ (電撃コミックスNEXT)",
			expected: "左遷された無能王子は実力を隠したい",
		},
		{
			name:     "Remove brackets and volume number",
			input:    "悪役令嬢の兄に転生しました【電子単行本】　7 (ヤングチャンピオン・コミックス)",
			expected: "悪役令嬢の兄に転生しました",
		},
		{
			name:     "Remove complex title with colon and volume number",
			input:    "異世界クラフトぐらし～自由気ままな生産職のほのぼのスローライフ～（コミック） ： 8 (モンスターコミックス)",
			expected: "異世界クラフトぐらし～自由気ままな生産職のほのぼのスローライフ～",
		},
		{
			name:     "Remove parentheses with volume number",
			input:    "村人ですが何か？(16) (ドラゴンコミックスエイジ)",
			expected: "村人ですが何か？",
		},
		{
			name:     "Remove volume number with spaces",
			input:    "監禁王　６ (ドラゴンコミックスエイジ)",
			expected: "監禁王",
		},
		{
			name:     "Remove parentheses with Japanese number",
			input:    "最弱貴族に転生したので悪役たちを集めてみた（２） (シリウスコミックス)",
			expected: "最弱貴族に転生したので悪役たちを集めてみた",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanTitle(tt.input)
			if result != tt.expected {
				t.Errorf("cleanTitle(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
