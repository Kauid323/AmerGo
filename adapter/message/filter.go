package message

import (
	"amer/config"
	"amer/db"
	"fmt"
	"log"
	"strings"
	"time"
)

func DetectRepeatedCharacters(message string, threshold int) bool {
	if threshold <= 0 {
		threshold = 10
	}

	runes := []rune(message)
	if len(runes) == 0 {
		return false
	}

	// 1. 检测连续相同字符
	count := 1
	var lastChar rune = -1

	for _, r := range runes {
		if r == lastChar {
			count++
			if count >= threshold {
				return true
			}
		} else {
			lastChar = r
			count = 1
		}
	}

	// 2. 检测大量连续空白字符
	spaceCount := 0
	for _, r := range runes {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			spaceCount++
			if spaceCount >= threshold {
				return true
			}
		} else {
			spaceCount = 0
		}
	}

	return false
}

func ReplaceBlockedWords(message string) string {
	blockedMap := config.GetBlockedWords()
	for _, words := range blockedMap {
		for _, word := range words {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			if strings.Contains(message, word) {
				mask := strings.Repeat("*", len([]rune(word)))
				message = strings.ReplaceAll(message, word, mask)
			}
		}
	}
	return message
}

func ContainsBlockedWords(message string) (bool, string) {
	cleaned := ReplaceBlockedWords(message)
	return cleaned != message, cleaned
}

func HandleViolation(platform, groupID, userID, userNickname, reason string) {
	today := time.Now().Format("2006-01-02")
	violationKey := fmt.Sprintf("violation:%s:%s:%s", platform, userID, today)

	count, _ := db.RDB.Get(db.Ctx, violationKey).Int()

	duration := 0
	if count == 0 {
		_ = db.RDB.Set(db.Ctx, violationKey, 1, 24*time.Hour).Err()
		log.Printf("[Filter] 用户 %s 首次违规，记录但未封禁", userID)
		return
	} else {
		duration = count * 60
		_ = db.RDB.Incr(db.Ctx, violationKey).Err()
	}

	if err := db.AddToBlacklist(userID, reason, duration); err == nil {
		durationText := "永久"
		if duration > 0 {
			durationText = fmt.Sprintf("大概 %d 秒", duration)
		}

		notifyText := fmt.Sprintf(
			"【ฅ喵呜·封禁通知ฅ】\n✦%s (ID: %s) 的小鱼干被没收啦~\n从现在起不会同步这个用户的消息了喵！\n✦封禁原因：%s\n✦持续时间：%s喵~",
			userNickname, userID, reason, durationText,
		)

		notifyHTML := fmt.Sprintf(
			`<div style="background-color: #f9f9f9; padding: 5px; border-radius: 5px;">%s (ID: %s) 的小鱼干被没收啦~<p style="font-size: 12px; color: #8b0000; margin: 5px 0;">从现在起不会同步这个用户的消息了喵！</p><p style="font-size: 12px; color: #333; margin: 5px 0;">✦封禁原因：%s</p><p style="font-size: 12px; color: #333; margin: 5px 0;">✦持续时间：%s</p></div>`,
			userNickname, userID, reason, durationText,
		)

		SendToAllBindings(platform, groupID, "html", notifyHTML, "0", "Amer", notifyText, "")
		log.Printf("[Filter] 用户 %s 触发违规被封禁: %s", userID, reason)
	}
}
