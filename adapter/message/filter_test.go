package message

import (
	"amer/config"
	"testing"
)

func TestDetectRepeatedCharacters(t *testing.T) {
	if !DetectRepeatedCharacters("aaaaaaaaaaa", 10) {
		t.Errorf("应该检测到重复字符")
	}

	if DetectRepeatedCharacters("hello world", 10) {
		t.Errorf("不应该误判普通字符串")
	}
}

func TestReplaceBlockedWords(t *testing.T) {
	config.AppConfig.BlockedWords = map[string][]string{
		"test": {"傻逼"},
	}

	cleaned := ReplaceBlockedWords("你是个傻逼吗")
	expected := "你是个**吗"
	if cleaned != expected {
		t.Errorf("期望得到 %s，实际得到 %s", expected, cleaned)
	}
}
