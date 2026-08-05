package message

import (
	"amer/config"
	"amer/db"
	"fmt"
	"regexp"
	"strings"
)

// IsAdmin checks if the given userID is an authorized administrator
func IsAdmin(platform, userID string, roles ...string) bool {
	if userID == "" {
		return false
	}
	uID := strings.TrimSpace(userID)
	if config.AppConfig.AdminUserID != "" && uID == strings.TrimSpace(config.AppConfig.AdminUserID) {
		return true
	}
	if config.AppConfig.YH.AdminID != "" && uID == strings.TrimSpace(config.AppConfig.YH.AdminID) {
		return true
	}
	if config.AppConfig.QQ.AdminQQ != "" && uID == strings.TrimSpace(config.AppConfig.QQ.AdminQQ) {
		return true
	}
	for _, r := range roles {
		rLower := strings.ToLower(strings.TrimSpace(r))
		if rLower == "owner" || rLower == "admin" {
			return true
		}
	}
	return false
}

// HandleAdminCommand processes admin commands like /amer-block-qq-1234567, /amer-unblock-yh-8516939, or /amer-unbl-yh-8516939
func HandleAdminCommand(platform, senderID, text string, roles ...string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/amer-block") && !strings.HasPrefix(trimmed, "/amer-unbl") &&
		!strings.HasPrefix(trimmed, "/amer-unblock") && !strings.HasPrefix(trimmed, "/amer-ban") &&
		!strings.HasPrefix(trimmed, "/amer-unban") {
		return false, ""
	}

	if !IsAdmin(platform, senderID, roles...) {
		return true, "❌ 权限不足：该指令仅限管理员使用！"
	}

	re := regexp.MustCompile(`(?i)^/amer-(block|unblock|ban|unbl|unban)[-_ ]?(qq|yh)?[-_ ]?(.*)$`)
	matches := re.FindStringSubmatch(trimmed)

	if len(matches) < 4 {
		return true, "❌ 指令格式错误！正确用法示例：\n- 拉黑: `/amer-block-qq-1234567` 或 `/amer-block-yh-8516939`\n- 解封: `/amer-unbl-qq-1234567` 或 `/amer-unbl-yh-8516939`"
	}

	action := strings.ToLower(matches[1])     // "block", "unblock", "ban", "unbl", "unban"
	targetPlat := strings.ToUpper(matches[2]) // "QQ", "YH"
	targetID := strings.TrimSpace(matches[3]) // "1234567"

	// If platform was embedded in targetID (e.g. "qq-1234567" or "yh-1234567")
	if targetPlat == "" {
		if strings.HasPrefix(strings.ToLower(targetID), "qq-") {
			targetPlat = "QQ"
			targetID = strings.TrimPrefix(strings.ToLower(targetID), "qq-")
		} else if strings.HasPrefix(strings.ToLower(targetID), "yh-") {
			targetPlat = "YH"
			targetID = strings.TrimPrefix(strings.ToLower(targetID), "yh-")
		} else if strings.HasPrefix(strings.ToLower(targetID), "qq ") {
			targetPlat = "QQ"
			targetID = strings.TrimPrefix(strings.ToLower(targetID), "qq ")
		} else if strings.HasPrefix(strings.ToLower(targetID), "yh ") {
			targetPlat = "YH"
			targetID = strings.TrimPrefix(strings.ToLower(targetID), "yh ")
		}
	}

	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return true, "❌ 缺少目标用户 ID！用法示例：`/amer-block-qq-1234567` 或 `/amer-unbl-yh-8516939`"
	}

	platName := "QQ"
	if targetPlat == "YH" || targetPlat == "YUNHU" {
		platName = "云湖"
	}

	if action == "block" || action == "ban" {
		reason := fmt.Sprintf("管理员 (%s:%s) 手动指令拉黑", platform, senderID)
		err := db.AddToBlacklist(targetID, reason, 3600*24*365) // 1 year block
		if err != nil {
			return true, fmt.Sprintf("❌ 拉黑失败: %v", err)
		}
		return true, fmt.Sprintf("✅ 已成功将 %s 用户 [%s] 加入黑名单 (1 年)", platName, targetID)
	}

	if action == "unbl" || action == "unban" || action == "unblock" {
		err := db.RemoveFromBlacklist(targetID)
		if err != nil {
			return true, fmt.Sprintf("❌ 解封失败: %v", err)
		}
		return true, fmt.Sprintf("✅ 已成功将 %s 用户 [%s] 从黑名单移除", platName, targetID)
	}

	return false, ""
}
