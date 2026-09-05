package message

import (
	"testing"
)

func TestConvertYunhuEmoji(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "今天天气真好[.滑稽]，哈哈[.笑哭]",
			expected: "今天天气真好🤪，哈哈😂",
		},
		{
			input:    "给你点个赞[.点赞]，爱你[.比心]！",
			expected: "给你点个赞👍，爱你🫰！",
		},
		{
			input:    "请你吃[.汉堡]和[.披萨]，再来一杯[.鸡尾酒]",
			expected: "请你吃🍔和🍕，再来一杯🍸",
		},
		{
			input:    "动物园里有[.北极熊]和[.霸王龙]以及[.生气的猫]",
			expected: "动物园里有🐻‍❄️和🦖以及😠",
		},
		{
			input:    "方向键：[.上箭头][.下箭头][.左箭头][.右箭头]",
			expected: "方向键：⬆️⬇️⬅️➡️",
		},
		{
			input:    "普通文本不包含表情",
			expected: "普通文本不包含表情",
		},
		{
			input:    "不存在的表情代码[.不存在的表情]",
			expected: "不存在的表情代码[.不存在的表情]",
		},
	}

	for _, tc := range tests {
		got := ConvertYunhuEmoji(tc.input)
		if got != tc.expected {
			t.Errorf("ConvertYunhuEmoji(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
