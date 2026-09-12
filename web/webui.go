package web

import (
	"amer/adapter/message"
	"amer/adapter/qq"
	"amer/adapter/yunhu"
	"amer/config"
	"amer/db"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

// StatsAPIHandler returns real-time system stats and message counters.
func StatsAPIHandler(c *gin.Context) {
	qqCount := 0
	yhCount := 0

	if db.RDB != nil {
		qqKeys, _ := db.RDB.Keys(db.Ctx, "QQ:*:QQ:*").Result()
		yhKeys, _ := db.RDB.Keys(db.Ctx, "YH:*:YH:*").Result()

		for _, key := range qqKeys {
			n, _ := db.RDB.LLen(db.Ctx, key).Result()
			qqCount += int(n)
		}
		for _, key := range yhKeys {
			n, _ := db.RDB.LLen(db.Ctx, key).Result()
			yhCount += int(n)
		}
	}

	bindings := db.GetAllBindings()
	blacklist := db.GetBlacklist()
	groupBlacklist := db.GetGroupBlacklist()
	uptimeSeconds := int64(time.Since(startTime).Seconds())

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"qq_messages":           qqCount,
			"yh_messages":           yhCount,
			"total_messages":        qqCount + yhCount,
			"total_bindings":        len(bindings),
			"total_blacklist":       len(blacklist) + len(groupBlacklist),
			"total_user_blacklist":  len(blacklist),
			"total_group_blacklist": len(groupBlacklist),
			"uptime_seconds":        uptimeSeconds,
		},
	})
}

// BindingsAPIHandler returns all group bindings enriched with group names and metadata.
func BindingsAPIHandler(c *gin.Context) {
	rawBindings := db.GetAllBindings()

	type EnrichedBinding struct {
		QQGroupID      string `json:"qq_group_id"`
		QQGroupName    string `json:"qq_group_name"`
		QQAvatarURL    string `json:"qq_avatar_url,omitempty"`
		YHGroupID      string `json:"yh_group_id"`
		YHGroupName    string `json:"yh_group_name"`
		YHAvatarURL    string `json:"yh_avatar_url,omitempty"`
		YHHeadcount    int64  `json:"yh_headcount,omitempty"`
		YHIntroduction string `json:"yh_introduction,omitempty"`
		QQToYHSync     bool   `json:"qq_to_yh_sync"`
		YHToQQSync     bool   `json:"yh_to_qq_sync"`
		SyncMode       string `json:"sync_mode"`
	}

	var list []EnrichedBinding
	for _, b := range rawBindings {
		qqName := b.QQGroupID
		if message.GlobalQQSender != nil {
			var qid int64
			fmt.Sscanf(b.QQGroupID, "%d", &qid)
			if qid != 0 {
				if name := message.GlobalQQSender.GetGroupName(qid); name != "" {
					qqName = name
				}
			}
		}
		qqAvatar := fmt.Sprintf("https://p.qlogo.cn/gh/%s/%s/100", b.QQGroupID, b.QQGroupID)

		yhName := b.YHGroupID
		var yhAvatar string
		var yhHeadcount int64
		var yhIntro string

		grpInfo := yunhu.YHClient.GetGroupInfo(b.YHGroupID)
		if grpInfo != nil {
			if grpInfo.Name != "" {
				yhName = grpInfo.Name
			}
			yhAvatar = grpInfo.AvatarURL
			yhHeadcount = grpInfo.Headcount
			yhIntro = grpInfo.Introduction
		} else if message.GlobalYHSender != nil {
			if name := message.GlobalYHSender.GetGroupName(b.YHGroupID); name != "" {
				yhName = name
			}
		}

		list = append(list, EnrichedBinding{
			QQGroupID:      b.QQGroupID,
			QQGroupName:    qqName,
			QQAvatarURL:    qqAvatar,
			YHGroupID:      b.YHGroupID,
			YHGroupName:    yhName,
			YHAvatarURL:    yhAvatar,
			YHHeadcount:    yhHeadcount,
			YHIntroduction: yhIntro,
			QQToYHSync:     b.QQToYHSync,
			YHToQQSync:     b.YHToQQSync,
			SyncMode:       b.SyncMode,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   list,
	})
}

type BindRequest struct {
	QQGroupID string `json:"qq_group_id"`
	YHGroupID string `json:"yh_group_id"`
	SyncMode  string `json:"sync_mode"`
}

// BindActionHandler binds a QQ group with a Yunhu group.
func BindActionHandler(c *gin.Context) {
	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "QQ群号和云湖群号不能为空"})
		return
	}

	res := db.Bind("QQ", "YH", req.QQGroupID, req.YHGroupID)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	if req.SyncMode != "" && req.SyncMode != "全同步" {
		validModes := map[string]map[string]bool{
			"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
			"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
			"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
			"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
		}
		if syncData, ok := validModes[req.SyncMode]; ok {
			_ = db.SetSync("QQ", "YH", req.QQGroupID, req.YHGroupID, syncData)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "绑定成功"})
}

// UnbindActionHandler unbinds a specific QQ group and Yunhu group pair.
func UnbindActionHandler(c *gin.Context) {
	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "QQ群号和云湖群号不能为空"})
		return
	}

	res := db.Unbind("QQ", "YH", req.QQGroupID, req.YHGroupID)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "解绑成功"})
}

type SetModeRequest struct {
	QQGroupID string `json:"qq_group_id"`
	YHGroupID string `json:"yh_group_id"`
	SyncMode  string `json:"sync_mode"`
}

// SetModeActionHandler sets the forwarding sync mode for a bound pair.
func SetModeActionHandler(c *gin.Context) {
	var req SetModeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "参数不完整"})
		return
	}

	validModes := map[string]map[string]bool{
		"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
		"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
		"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
		"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
	}

	syncData, ok := validModes[req.SyncMode]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "无效的同步模式"})
		return
	}

	res := db.SetSync("QQ", "YH", req.QQGroupID, req.YHGroupID, syncData)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "同步模式已更新"})
}

type UserBlacklistDetailItem struct {
	UserID        string `json:"user_id"`
	Reason        string `json:"reason"`
	RemainingTime int64  `json:"remaining_time"`
	Platform      string `json:"platform"` // "QQ" or "YH"
	Nickname      string `json:"nickname,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

func ResolveUserDetail(item db.BlacklistItem) UserBlacklistDetailItem {
	uid := strings.TrimSpace(item.UserID)
	res := UserBlacklistDetailItem{
		UserID:        uid,
		Reason:        item.Reason,
		RemainingTime: item.RemainingTime,
	}

	// 规则：uid 超过 7 位数就是 QQ 用户
	if len(uid) > 7 {
		res.Platform = "QQ"
		res.Nickname = fmt.Sprintf("QQ用户(%s)", uid)
		res.AvatarURL = fmt.Sprintf("https://q1.qlogo.cn/g?b=qq&nk=%s&s=100", uid)
		return res
	}

	// 7 位及以下：调用云湖 API 获取用户详情，检测用户名称是否为空
	userInfo := yunhu.YHClient.GetUserInfo(uid)
	if userInfo != nil && strings.TrimSpace(userInfo.Nickname) != "" {
		res.Platform = "YH"
		res.Nickname = userInfo.Nickname
		res.AvatarURL = userInfo.AvatarURL
		return res
	}

	// 若未在云湖获取到有效名称，兜底归为 QQ 用户
	res.Platform = "QQ"
	res.Nickname = fmt.Sprintf("QQ用户(%s)", uid)
	res.AvatarURL = fmt.Sprintf("https://q1.qlogo.cn/g?b=qq&nk=%s&s=100", uid)
	return res
}

// BlacklistAPIHandler returns the list of banned users with platform and detail resolution.
func BlacklistAPIHandler(c *gin.Context) {
	list := db.GetBlacklist()
	platformFilter := strings.ToUpper(strings.TrimSpace(c.Query("platform")))

	var detailList []UserBlacklistDetailItem
	for _, item := range list {
		detail := ResolveUserDetail(item)
		if platformFilter == "" || platformFilter == "ALL" || detail.Platform == platformFilter {
			detailList = append(detailList, detail)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   detailList,
	})
}

type BlacklistRequest struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

// AddBlacklistHandler adds a user to the blacklist.
func AddBlacklistHandler(c *gin.Context) {
	var req BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "用户ID不能为空"})
		return
	}
	if req.Reason == "" {
		req.Reason = "管理员通过 WebUI 手动拉黑"
	}

	err := db.AddToBlacklist(req.UserID, req.Reason, req.Duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "添加黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已将用户加入黑名单"})
}

// RemoveBlacklistHandler removes a user from the blacklist.
func RemoveBlacklistHandler(c *gin.Context) {
	var req BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "用户ID不能为空"})
		return
	}

	err := db.RemoveFromBlacklist(req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "解除黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已成功将用户移出黑名单"})
}

type GroupBlacklistRequest struct {
	GroupID  string `json:"group_id"`
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

type GroupBlacklistDetailItem struct {
	GroupID       string `json:"group_id"`
	Reason        string `json:"reason"`
	RemainingTime int64  `json:"remaining_time"`
	GroupName     string `json:"group_name,omitempty"`
	Platform      string `json:"platform,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	Introduction  string `json:"introduction,omitempty"`
	Headcount     int64  `json:"headcount,omitempty"`
	CreateBy      string `json:"create_by,omitempty"`
}

func ResolveGroupDetail(item db.GroupBlacklistItem) GroupBlacklistDetailItem {
	gid := strings.TrimSpace(item.GroupID)
	detail := GroupBlacklistDetailItem{
		GroupID:       gid,
		Reason:        item.Reason,
		RemainingTime: item.RemainingTime,
	}

	// 优先调用云湖群聊接口查询群元数据（支持 9 位数等各类云湖群号）
	grpInfo := yunhu.YHClient.GetGroupInfo(gid)
	if grpInfo != nil && strings.TrimSpace(grpInfo.Name) != "" {
		detail.Platform = "YH"
		detail.GroupName = grpInfo.Name
		detail.AvatarURL = grpInfo.AvatarURL
		detail.Introduction = grpInfo.Introduction
		detail.Headcount = grpInfo.Headcount
		detail.CreateBy = grpInfo.CreateBy
		return detail
	}

	// 若云湖未查询到对应群聊，识别为 QQ 群并拉取 QQ 群名与头像
	detail.Platform = "QQ"
	detail.AvatarURL = fmt.Sprintf("https://p.qlogo.cn/gh/%s/%s/100", gid, gid)
	if qID, err := strconv.ParseInt(gid, 10, 64); err == nil && qq.GlobalOneBotServer != nil {
		detail.GroupName = qq.GlobalOneBotServer.GetGroupName(qID)
	}
	if detail.GroupName == "" {
		detail.GroupName = fmt.Sprintf("QQ群-%s", gid)
	}
	return detail
}

// GroupBlacklistAPIHandler returns the list of banned groups.
func GroupBlacklistAPIHandler(c *gin.Context) {
	list := db.GetGroupBlacklist()
	platformFilter := strings.ToUpper(strings.TrimSpace(c.Query("platform")))

	var detailList []GroupBlacklistDetailItem
	for _, item := range list {
		detail := ResolveGroupDetail(item)
		if platformFilter == "" || platformFilter == "ALL" || detail.Platform == platformFilter {
			detailList = append(detailList, detail)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   detailList,
	})
}

// AddGroupBlacklistHandler adds a group to the blacklist.
func AddGroupBlacklistHandler(c *gin.Context) {
	var req GroupBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.GroupID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "群号/群ID不能为空"})
		return
	}
	if req.Reason == "" {
		req.Reason = "管理员通过 WebUI 手动拉黑群聊"
	}

	err := db.AddGroupToBlacklist(req.GroupID, req.Reason, req.Duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "添加群黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已将群聊加入黑名单"})
}

// RemoveGroupBlacklistHandler removes a group from the blacklist.
func RemoveGroupBlacklistHandler(c *gin.Context) {
	var req GroupBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.GroupID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "群号/群ID不能为空"})
		return
	}

	err := db.RemoveGroupFromBlacklist(req.GroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "解除群黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已成功将群聊移出黑名单"})
}

type BlockedWordAddRequest struct {
	Category string   `json:"category"`
	Words    []string `json:"words"`
	RawWords string   `json:"raw_words"`
}

type BlockedWordDeleteRequest struct {
	Category string `json:"category"`
	Word     string `json:"word"`
}

type BlockedCategoryDeleteRequest struct {
	Category string `json:"category"`
}

// BlockedWordsAPIHandler returns all blocked words grouped by category.
func BlockedWordsAPIHandler(c *gin.Context) {
	words := config.GetBlockedWords()
	totalCount := 0
	for _, wList := range words {
		totalCount += len(wList)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"categories":  words,
			"total_count": totalCount,
			"config_path": config.GetConfigPath(),
		},
	})
}

// AddBlockedWordsHandler adds one or multiple blocked words to a category and hot-updates config.yaml.
func AddBlockedWordsHandler(c *gin.Context) {
	var req BlockedWordAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "请求参数解析失败"})
		return
	}

	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "默认分类"
	}

	var toAdd []string
	for _, w := range req.Words {
		w = strings.TrimSpace(w)
		if w != "" {
			toAdd = append(toAdd, w)
		}
	}
	if req.RawWords != "" {
		lines := strings.FieldsFunc(req.RawWords, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == '，' || r == ';' || r == '；'
		})
		for _, w := range lines {
			w = strings.TrimSpace(w)
			if w != "" {
				toAdd = append(toAdd, w)
			}
		}
	}

	if len(toAdd) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "请至少提供一个非空的屏蔽词"})
		return
	}

	currentWords := config.GetBlockedWords()
	existingList := currentWords[category]

	existingMap := make(map[string]bool)
	for _, w := range existingList {
		existingMap[w] = true
	}

	addedCount := 0
	for _, w := range toAdd {
		if !existingMap[w] {
			existingMap[w] = true
			existingList = append(existingList, w)
			addedCount++
		}
	}

	currentWords[category] = existingList

	if err := config.UpdateBlockedWords(currentWords); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": fmt.Sprintf("保存配置文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"msg":         fmt.Sprintf("成功向分类【%s】添加 %d 个新屏蔽词，并已即时热更新生效！", category, addedCount),
		"added_count": addedCount,
	})
}

// DeleteBlockedWordHandler deletes a single word from a category.
func DeleteBlockedWordHandler(c *gin.Context) {
	var req BlockedWordDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Category == "" || req.Word == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "分类名称与要删除的屏蔽词不能为空"})
		return
	}

	category := strings.TrimSpace(req.Category)
	targetWord := strings.TrimSpace(req.Word)

	currentWords := config.GetBlockedWords()
	existingList, exists := currentWords[category]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "未找到指定的屏蔽词分类"})
		return
	}

	var newList []string
	found := false
	for _, w := range existingList {
		if w == targetWord {
			found = true
		} else {
			newList = append(newList, w)
		}
	}

	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "分类中未找到该屏蔽词"})
		return
	}

	currentWords[category] = newList

	if err := config.UpdateBlockedWords(currentWords); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": fmt.Sprintf("更新配置文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"msg":    fmt.Sprintf("已成功从分类【%s】中删除屏蔽词【%s】，已热更新生效！", category, targetWord),
	})
}

// DeleteBlockedCategoryHandler deletes an entire category of blocked words.
func DeleteBlockedCategoryHandler(c *gin.Context) {
	var req BlockedCategoryDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "分类名称不能为空"})
		return
	}

	category := strings.TrimSpace(req.Category)
	currentWords := config.GetBlockedWords()
	if _, exists := currentWords[category]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "未找到指定的屏蔽词分类"})
		return
	}

	delete(currentWords, category)

	if err := config.UpdateBlockedWords(currentWords); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": fmt.Sprintf("更新配置文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"msg":    fmt.Sprintf("已成功删除屏蔽词分类【%s】，已热更新生效！", category),
	})
}

// ReloadConfigAPIHandler forces a manual reload of config.yaml into memory.
func ReloadConfigAPIHandler(c *gin.Context) {
	cfg, err := config.ReloadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": fmt.Sprintf("热重载配置失败: %v", err)})
		return
	}

	totalWords := 0
	for _, ws := range cfg.BlockedWords {
		totalWords += len(ws)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"msg":    fmt.Sprintf("配置文件 %s 已成功热重载！当前屏蔽词分类 %d 个，总词数 %d 个。", config.GetConfigPath(), len(cfg.BlockedWords), totalWords),
		"data": gin.H{
			"blocked_categories":  len(cfg.BlockedWords),
			"total_blocked_words": totalWords,
		},
	})
}

// SystemInfoAPIHandler returns overall system and configuration parameters.
func SystemInfoAPIHandler(c *gin.Context) {
	words := config.GetBlockedWords()
	blockedWordGroups := make(map[string]int)
	totalBlockedWords := 0
	for category, wList := range words {
		blockedWordGroups[category] = len(wList)
		totalBlockedWords += len(wList)
	}

	webuiPort := config.AppConfig.WebUI.Port
	if webuiPort == 0 {
		webuiPort = config.AppConfig.Server.Port
	}
	webuiHost := config.AppConfig.WebUI.Host
	if webuiHost == "" {
		webuiHost = config.AppConfig.Server.Host
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"port":         config.AppConfig.Server.Port,
			"host":         config.AppConfig.Server.Host,
			"webui_port":   webuiPort,
			"webui_host":   webuiHost,
			"lan_ips":      GetLocalIPs(),
			"yh_mode":      config.AppConfig.YH.Mode,
			"bot_qq":       config.AppConfig.QQ.BotQQ,
			"temp_folder":  config.AppConfig.TempFolder,
			"image_scale":  config.AppConfig.Image.Scale,
			"image_max_w":  config.AppConfig.Image.MaxWidth,
			"image_max_h":  config.AppConfig.Image.MaxHeight,
			"blocked_cats": blockedWordGroups,
			"blocked_sum":  totalBlockedWords,
			"uptime_sec":   int64(time.Since(startTime).Seconds()),
		},
	})
}

func WebUIHandler(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Amer 控制台 - 互通管理系统</title>
    <style>
        :root {
            --bg-color: #f8fafc;
            --surface-color: #ffffff;
            --border-color: #e2e8f0;
            --text-primary: #0f172a;
            --text-secondary: #475569;
            --text-muted: #94a3b8;
            --primary: #2563eb;
            --primary-hover: #1d4ed8;
            --success: #10b981;
            --danger: #ef4444;
            --danger-hover: #dc2626;
            --warning: #f59e0b;
            --radius: 6px;
            --font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: var(--font); background: var(--bg-color); color: var(--text-primary); line-height: 1.5; font-size: 14px; }
        
        .navbar { background: var(--surface-color); border-bottom: 1px solid var(--border-color); height: 56px; display: flex; align-items: center; justify-content: space-between; padding: 0 24px; position: sticky; top: 0; z-index: 100; }
        .nav-brand { display: flex; align-items: center; gap: 10px; font-weight: 600; font-size: 16px; color: var(--text-primary); }
        .nav-brand .logo { width: 28px; height: 28px; background: var(--primary); color: white; border-radius: var(--radius); display: flex; align-items: center; justify-content: center; font-weight: bold; font-size: 14px; }
        .status-badge { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: #047857; background: #ecfdf5; padding: 3px 8px; border-radius: 9999px; border: 1px solid #a7f3d0; font-weight: 500; }
        .status-dot { width: 6px; height: 6px; background: var(--success); border-radius: 50%; }

        .container { max-width: 1200px; margin: 24px auto; padding: 0 20px; }
        
        .tabs-header { display: flex; gap: 8px; border-bottom: 1px solid var(--border-color); margin-bottom: 24px; }
        .tab-btn { background: none; border: none; padding: 10px 18px; font-size: 14px; font-weight: 500; color: var(--text-secondary); cursor: pointer; border-bottom: 2px solid transparent; transition: all 0.15s ease; }
        .tab-btn:hover { color: var(--primary); }
        .tab-btn.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }

        .tab-pane { display: none; }
        .tab-pane.active { display: block; }

        .grid-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; margin-bottom: 24px; }
        .stat-card { background: var(--surface-color); border: 1px solid var(--border-color); border-radius: var(--radius); padding: 18px 20px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
        .stat-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; }
        .stat-val { font-size: 26px; font-weight: 700; color: var(--text-primary); }

        .panel { background: var(--surface-color); border: 1px solid var(--border-color); border-radius: var(--radius); margin-bottom: 24px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
        .panel-header { padding: 16px 20px; border-bottom: 1px solid var(--border-color); display: flex; align-items: center; justify-content: space-between; }
        .panel-title { font-size: 15px; font-weight: 600; color: var(--text-primary); display: flex; align-items: center; gap: 8px; }
        .panel-body { padding: 20px; }

        .btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 7px 14px; font-size: 13px; font-weight: 500; border-radius: var(--radius); cursor: pointer; border: 1px solid transparent; transition: background 0.15s; text-decoration: none; }
        .btn-primary { background: var(--primary); color: #fff; }
        .btn-primary:hover { background: var(--primary-hover); }
        .btn-outline { background: #fff; border-color: var(--border-color); color: var(--text-primary); }
        .btn-outline:hover { background: #f1f5f9; }
        .btn-danger { background: #fee2e2; border-color: #fecaca; color: #b91c1c; }
        .btn-danger:hover { background: #fca5a5; }
        .btn-sm { padding: 4px 10px; font-size: 12px; }

        .form-row { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 16px; align-items: flex-end; }
        .form-group { display: flex; flex-direction: column; gap: 6px; flex: 1; min-width: 180px; }
        .form-group label { font-size: 12px; font-weight: 500; color: var(--text-secondary); }
        .form-control { height: 34px; padding: 0 10px; border: 1px solid var(--border-color); border-radius: var(--radius); font-size: 13px; color: var(--text-primary); outline: none; background: #fff; }
        .form-control:focus { border-color: var(--primary); }

        .table-responsive { width: 100%; overflow-x: auto; }
        table { width: 100%; border-collapse: collapse; text-align: left; font-size: 13px; }
        th { background: #f8fafc; color: var(--text-secondary); font-weight: 600; padding: 10px 14px; border-bottom: 1px solid var(--border-color); white-space: nowrap; }
        td { padding: 12px 14px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); vertical-align: middle; }
        tr:last-child td { border-bottom: none; }
        tr:hover td { background: #f8fafc; }

        .tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }
        .tag-all { background: #ecfdf5; color: #047857; border: 1px solid #a7f3d0; }
        .tag-qq { background: #eff6ff; color: #1d4ed8; border: 1px solid #bfdbfe; }
        .tag-yh { background: #fffbeb; color: #b45309; border: 1px solid #fde68a; }
        .tag-stopped { background: #f1f5f9; color: #475569; border: 1px solid #cbd5e1; }

        .select-sm { height: 28px; padding: 0 6px; font-size: 12px; border-radius: var(--radius); border: 1px solid var(--border-color); background: #fff; }

        #toast-container { position: fixed; top: 16px; right: 20px; z-index: 9999; display: flex; flex-direction: column; gap: 8px; }
        .toast { padding: 10px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; color: #fff; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1); opacity: 0; transform: translateY(-10px); transition: all 0.2s ease; }
        .toast.show { opacity: 1; transform: translateY(0); }
        .toast-success { background: #059669; }
        .toast-error { background: #dc2626; }

        .empty-state { text-align: center; padding: 40px 20px; color: var(--text-muted); font-size: 13px; }

        .chip { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; background: #f1f5f9; border: 1px solid #cbd5e1; border-radius: 9999px; font-size: 12px; color: #334155; margin: 3px 4px 3px 0; }
        .chip .chip-del { cursor: pointer; color: #94a3b8; font-weight: bold; border-radius: 50%; width: 14px; height: 14px; display: inline-flex; align-items: center; justify-content: center; font-size: 11px; }
        .chip .chip-del:hover { color: #ef4444; background: #fee2e2; }
        .category-box { background: #fff; border: 1px solid var(--border-color); border-radius: var(--radius); padding: 16px; margin-bottom: 16px; }
        .category-header { display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #f1f5f9; padding-bottom: 10px; margin-bottom: 12px; }
        .category-title { font-weight: 600; font-size: 14px; color: var(--text-primary); display: flex; align-items: center; gap: 8px; }

        .info-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
        .info-item { background: #f8fafc; border: 1px solid var(--border-color); border-radius: var(--radius); padding: 10px 14px; }
        .info-item .k { font-size: 12px; color: var(--text-secondary); margin-bottom: 2px; }
        .info-item .v { font-weight: 600; color: var(--text-primary); font-size: 13px; }

        .user-avatar { width: 34px; height: 34px; border-radius: 50%; object-fit: cover; background: #e2e8f0; vertical-align: middle; flex-shrink: 0; border: 1px solid var(--border-color); }
        .user-info-cell { display: flex; align-items: center; gap: 10px; }
        .filter-tabs { display: flex; gap: 6px; }
        .filter-btn { padding: 4px 10px; font-size: 12px; border-radius: var(--radius); border: 1px solid var(--border-color); background: #fff; color: var(--text-secondary); cursor: pointer; transition: all 0.15s; }
        .filter-btn:hover { background: #f1f5f9; color: var(--primary); }
        .filter-btn.active { background: var(--primary); color: #fff; border-color: var(--primary); font-weight: 600; }
        .subtab-header { display: flex; gap: 8px; margin-bottom: 16px; border-bottom: 1px solid var(--border-color); padding-bottom: 8px; }
        .subtab-btn { background: none; border: none; padding: 6px 14px; font-size: 13px; font-weight: 500; color: var(--text-secondary); cursor: pointer; border-radius: var(--radius); transition: all 0.15s; }
        .subtab-btn:hover { background: #f1f5f9; color: var(--primary); }
        .subtab-btn.active { background: #eff6ff; color: var(--primary); font-weight: 600; border: 1px solid #bfdbfe; }
    </style>
</head>
<body>

    <header class="navbar">
        <div class="nav-brand">
            <div class="logo">A</div>
            <span>Amer 控制中心</span>
            <span class="status-badge"><span class="status-dot"></span> 服务正常</span>
            <span id="nav-lan-badge" style="display:none;font-size:12px;color:#2563eb;background:#eff6ff;padding:2px 8px;border-radius:9999px;border:1px solid #bfdbfe;font-weight:500;"></span>
        </div>
        <div>
            <button class="btn btn-outline btn-sm" onclick="refreshCurrentTab()">🔄 刷新数据</button>
        </div>
    </header>

    <div id="toast-container"></div>

    <main class="container">
        <nav class="tabs-header">
            <button class="tab-btn active" onclick="switchTab('dashboard')">📊 运行概览</button>
            <button class="tab-btn" onclick="switchTab('bindings')">🔗 群聊绑定与转发模式</button>
            <button class="tab-btn" onclick="switchTab('blacklist')">🚫 黑名单管理</button>
            <button class="tab-btn" onclick="switchTab('blocked')">🛡️ 屏蔽词设置</button>
            <button class="tab-btn" onclick="switchTab('logs')">📜 控制台日志</button>
            <button class="tab-btn" onclick="switchTab('system')">⚙️ 系统状态与配置</button>
        </nav>

        <!-- 1. 运行概览 Tab -->
        <section id="pane-dashboard" class="tab-pane active">
            <!-- 局域网内网访问指引条 -->
            <div id="lan-access-banner" style="display:none;background:#eff6ff;border:1px solid #bfdbfe;border-left:4px solid #2563eb;border-radius:var(--radius);padding:12px 16px;margin-bottom:20px;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;">
                <div>
                    <div style="font-weight:600;font-size:13px;color:#1e40af;margin-bottom:3px;">🌐 局域网内网访问支持</div>
                    <div id="lan-access-urls" style="font-size:12px;color:#1d4ed8;line-height:1.6;"></div>
                </div>
                <div style="font-size:11px;color:#3b82f6;background:#dbeafe;padding:4px 8px;border-radius:4px;">局域网内任意手机或电脑浏览器输入上方链接均可直接访问控制台</div>
            </div>

            <div class="grid-stats">
                <div class="stat-card">
                    <div class="stat-label">总同步消息数</div>
                    <div class="stat-val" id="stat-total">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">QQ 消息量</div>
                    <div class="stat-val" id="stat-qq">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">云湖消息量</div>
                    <div class="stat-val" id="stat-yh">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">当前群绑定对数</div>
                    <div class="stat-val" id="stat-bindings">-</div>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📌 快速快捷入口</div>
                </div>
                <div class="panel-body" style="display: flex; gap: 12px; flex-wrap: wrap;">
                    <button class="btn btn-primary" onclick="switchTab('bindings'); document.getElementById('qq-input').focus();">➕ 绑定新群聊</button>
                    <button class="btn btn-outline" onclick="switchTab('blocked')">🛡️ 屏蔽词设置与热更新</button>
                    <button class="btn btn-outline" onclick="switchTab('logs')">📜 查看实时控制台日志</button>
                    <button class="btn btn-outline" onclick="switchTab('blacklist')">🚫 查看与添加黑名单</button>
                    <button class="btn btn-outline" onclick="switchTab('system')">⚙️ 查看详细系统配置</button>
                </div>
            </div>
        </section>

        <!-- 2. 群聊绑定与模式管理 Tab -->
        <section id="pane-bindings" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">➕ 新增群聊绑定</div>
                </div>
                <div class="panel-body">
                    <form id="bind-form" onsubmit="handleCreateBinding(event)">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="qq-input">QQ 群号 *</label>
                                <input id="qq-input" class="form-control" type="text" placeholder="例如: 1013637348" required>
                            </div>
                            <div class="form-group">
                                <label for="yh-input">云湖群号 *</label>
                                <input id="yh-input" class="form-control" type="text" placeholder="例如: 644353075" required>
                            </div>
                            <div class="form-group" style="max-width: 180px;">
                                <label for="mode-select">默认转发模式</label>
                                <select id="mode-select" class="form-control">
                                    <option value="全同步">全同步 (双向)</option>
                                    <option value="QQ到云湖">仅 QQ 到云湖</option>
                                    <option value="云湖到QQ">仅 云湖 到 QQ</option>
                                    <option value="停止">暂停同步</option>
                                </select>
                            </div>
                            <button type="submit" class="btn btn-primary" style="height: 34px;">确认绑定</button>
                        </div>
                    </form>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📋 当前已绑定群聊列表 (<span id="binding-count">0</span>)</div>
                    <div style="display: flex; gap: 8px;">
                        <input id="binding-search" class="form-control" style="height: 28px; width: 180px; font-size: 12px;" placeholder="搜索群号/群名..." oninput="filterBindingsTable()">
                    </div>
                </div>
                <div class="table-responsive">
                    <table>
                        <thead>
                            <tr>
                                <th>QQ 群信息</th>
                                <th>云湖群信息</th>
                                <th>当前转发模式</th>
                                <th>修改转发模式</th>
                                <th style="text-align: right;">操作</th>
                            </tr>
                        </thead>
                        <tbody id="bindings-tbody">
                            <tr><td colspan="5" class="empty-state">正在加载绑定数据...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </section>

        <!-- 3. 黑名单管理 Tab -->
        <section id="pane-blacklist" class="tab-pane">
            <div class="subtab-header">
                <button id="subtab-btn-user" class="subtab-btn active" onclick="switchBlacklistSubtab('user')">👤 用户黑名单 (<span id="tab-user-blacklist-count">0</span>)</button>
                <button id="subtab-btn-group" class="subtab-btn" onclick="switchBlacklistSubtab('group')">👥 群聊黑名单 (<span id="tab-group-blacklist-count">0</span>)</button>
            </div>

            <!-- 用户黑名单区域 -->
            <div id="blacklist-user-section">
                <div class="panel">
                    <div class="panel-header">
                        <div class="panel-title">🚫 添加用户到黑名单</div>
                    </div>
                    <div class="panel-body">
                        <form id="blacklist-form" onsubmit="handleAddBlacklist(event)">
                            <div class="form-row">
                                <div class="form-group">
                                    <label for="ban-uid">违规用户 ID *</label>
                                    <input id="ban-uid" class="form-control" type="text" placeholder="QQ号或云湖用户ID" required>
                                </div>
                                <div class="form-group">
                                    <label for="ban-reason">封禁原因</label>
                                    <input id="ban-reason" class="form-control" type="text" placeholder="例如: 频繁刷屏 / 违规广告">
                                </div>
                                <div class="form-group" style="max-width: 180px;">
                                    <label for="ban-duration">封禁时长</label>
                                    <select id="ban-duration" class="form-control">
                                        <option value="1800">30 分钟</option>
                                        <option value="3600" selected>1 小时</option>
                                        <option value="86400">1 天</option>
                                        <option value="604800">7 天</option>
                                        <option value="0">永久封禁</option>
                                    </select>
                                </div>
                                <button type="submit" class="btn btn-danger" style="height: 34px;">添加用户封禁</button>
                            </div>
                        </form>
                    </div>
                </div>

                <div class="panel">
                    <div class="panel-header">
                        <div class="panel-title">📋 处于黑名单中的用户 (<span id="blacklist-count">0</span>)</div>
                        <div class="filter-tabs">
                            <button id="user-filter-all" class="filter-btn active" onclick="setUserPlatformFilter('ALL')">全部 (<span id="count-user-all">0</span>)</button>
                            <button id="user-filter-qq" class="filter-btn" onclick="setUserPlatformFilter('QQ')">🐧 QQ 用户 (<span id="count-user-qq">0</span>)</button>
                            <button id="user-filter-yh" class="filter-btn" onclick="setUserPlatformFilter('YH')">☁️ 云湖用户 (<span id="count-user-yh">0</span>)</button>
                        </div>
                    </div>
                    <div class="table-responsive">
                        <table>
                            <thead>
                                <tr>
                                    <th style="width: 80px;">平台</th>
                                    <th>用户信息</th>
                                    <th>封禁原因</th>
                                    <th>剩余时间</th>
                                    <th style="text-align: right;">操作</th>
                                </tr>
                            </thead>
                            <tbody id="blacklist-tbody">
                                <tr><td colspan="5" class="empty-state">暂无被封禁的用户</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <!-- 群聊黑名单区域 -->
            <div id="blacklist-group-section" style="display: none;">
                <div class="panel">
                    <div class="panel-header">
                        <div class="panel-title">🚫 添加群聊到黑名单</div>
                    </div>
                    <div class="panel-body">
                        <form id="group-blacklist-form" onsubmit="handleAddGroupBlacklist(event)">
                            <div class="form-row">
                                <div class="form-group">
                                    <label for="group-ban-id">违规群号 / 群 ID *</label>
                                    <input id="group-ban-id" class="form-control" type="text" placeholder="QQ群号或云湖群ID" required>
                                </div>
                                <div class="form-group">
                                    <label for="group-ban-reason">封禁原因</label>
                                    <input id="group-ban-reason" class="form-control" type="text" placeholder="例如: 违规违禁群聊 / 刷屏干扰">
                                </div>
                                <div class="form-group" style="max-width: 180px;">
                                    <label for="group-ban-duration">封禁时长</label>
                                    <select id="group-ban-duration" class="form-control">
                                        <option value="1800">30 分钟</option>
                                        <option value="3600" selected>1 小时</option>
                                        <option value="86400">1 天</option>
                                        <option value="604800">7 天</option>
                                        <option value="0">永久封禁</option>
                                    </select>
                                </div>
                                <button type="submit" class="btn btn-danger" style="height: 34px;">添加群聊封禁</button>
                            </div>
                        </form>
                    </div>
                </div>

                <div class="panel">
                    <div class="panel-header">
                        <div class="panel-title">📋 处于黑名单中的群聊 (<span id="group-blacklist-count">0</span>)</div>
                        <div class="filter-tabs">
                            <button id="group-filter-all" class="filter-btn active" onclick="setGroupPlatformFilter('ALL')">全部 (<span id="count-group-all">0</span>)</button>
                            <button id="group-filter-qq" class="filter-btn" onclick="setGroupPlatformFilter('QQ')">🐧 QQ 群 (<span id="count-group-qq">0</span>)</button>
                            <button id="group-filter-yh" class="filter-btn" onclick="setGroupPlatformFilter('YH')">☁️ 云湖群 (<span id="count-group-yh">0</span>)</button>
                        </div>
                    </div>
                    <div class="table-responsive">
                        <table>
                            <thead>
                                <tr>
                                    <th style="width: 80px;">平台</th>
                                    <th>群聊信息</th>
                                    <th>封禁原因</th>
                                    <th>剩余时间</th>
                                    <th style="text-align: right;">操作</th>
                                </tr>
                            </thead>
                            <tbody id="group-blacklist-tbody">
                                <tr><td colspan="5" class="empty-state">暂无被封禁的群聊</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </section>

        <!-- 4. 屏蔽词设置 Tab -->
        <section id="pane-blocked" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">🛡️ 添加屏蔽词</div>
                    <div>
                        <button class="btn btn-outline btn-sm" onclick="handleReloadConfig()">🔄 重新加载 config.yaml (热更新)</button>
                    </div>
                </div>
                <div class="panel-body">
                    <form id="blocked-word-form" onsubmit="handleAddBlockedWords(event)">
                        <div class="form-row">
                            <div class="form-group" style="max-width: 240px;">
                                <label for="bw-category-input">屏蔽词分类 *</label>
                                <input id="bw-category-input" class="form-control" type="text" list="category-datalist" placeholder="输入或选择分类(如: 广告/违规)" required>
                                <datalist id="category-datalist"></datalist>
                            </div>
                            <div class="form-group" style="flex: 2;">
                                <label for="bw-words-input">屏蔽词内容 * (支持逗号、分号或换行批量输入多个词)</label>
                                <input id="bw-words-input" class="form-control" type="text" placeholder="例如: 兼职, 买片, 挂科, 刷单" required>
                            </div>
                            <button type="submit" class="btn btn-primary" style="height: 34px;">添加并即时生效</button>
                        </div>
                    </form>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📋 屏蔽词分类列表 (<span id="bw-cat-count">0</span> 个分类，共 <span id="bw-total-count">0</span> 个词)</div>
                    <div style="display: flex; gap: 8px;">
                        <input id="bw-search-input" class="form-control" style="height: 28px; width: 180px; font-size: 12px;" placeholder="搜索分类或屏蔽词..." oninput="filterBlockedWords()">
                    </div>
                </div>
                <div class="panel-body" id="blocked-words-container">
                    <div class="empty-state">正在加载屏蔽词数据...</div>
                </div>
            </div>
        </section>

        <!-- 5. 控制台日志 Tab -->
        <section id="pane-logs" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📜 命令行控制台日志</div>
                    <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
                        <span id="log-status-badge" class="status-badge" style="background:#ecfdf5;color:#047857;border-color:#a7f3d0;">
                            <span class="status-dot"></span> 实时接收中 (SSE)
                        </span>
                        <button id="btn-toggle-live" class="btn btn-outline btn-sm" onclick="toggleLiveLogs()">⏸️ 暂停实时</button>
                        <button id="btn-refresh-logs" class="btn btn-outline btn-sm" style="display:none;" onclick="loadStaticLogs()">🔄 刷新快照</button>
                        <button class="btn btn-outline btn-sm" onclick="clearLogsScreen()">🗑️ 清屏</button>
                        <button class="btn btn-outline btn-sm" onclick="exportLogs()">📥 导出</button>
                    </div>
                </div>
                <div class="panel-body" style="padding:14px 20px;">
                    <!-- 过滤栏与开关 -->
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px;gap:12px;flex-wrap:wrap;">
                        <div style="display:flex;align-items:center;gap:8px;flex:1;max-width:380px;">
                            <input id="log-search-input" class="form-control" type="text" placeholder="🔍 过滤关键字 (如 QQ, Yunhu, Error...)" oninput="filterLogs()" style="height:32px;font-size:12px;">
                        </div>
                        <div style="display:flex;align-items:center;gap:16px;font-size:12px;color:var(--text-secondary);">
                            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
                                <input id="chk-auto-scroll" type="checkbox" checked onchange="toggleAutoScroll(this.checked)">
                                <span>自动滚动到底部</span>
                            </label>
                            <span id="log-count-text">显示: 0 行</span>
                        </div>
                    </div>

                    <!-- 终端日志容器 -->
                    <div id="terminal-container" style="background:#0f172a;border:1px solid #334155;border-radius:6px;padding:12px 14px;height:560px;overflow-y:auto;font-family:'JetBrains Mono', Consolas, Menlo, Monaco, monospace;font-size:12px;line-height:1.55;color:#cbd5e1;white-space:pre-wrap;word-break:break-all;">
                    </div>
                </div>
            </div>
        </section>

        <!-- 6. 系统设置与状态 Tab -->
        <section id="pane-system" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">⚙️ 运行参数与系统配置</div>
                </div>
                <div class="panel-body">
                    <div class="info-grid" id="system-info-grid">
                        <div class="info-item"><div class="k">正在加载系统信息...</div></div>
                    </div>
                </div>
            </div>
        </section>
    </main>

    <script>
        let cachedBindings = [];
        let cachedBlockedWords = {};

        function switchTab(tabId) {
            document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelectorAll('.tab-pane').forEach(pane => pane.classList.remove('active'));
            
            event && event.target && event.target.classList.add('active');
            const targetPane = document.getElementById('pane-' + tabId);
            if (targetPane) targetPane.classList.add('active');

            if (tabId === 'dashboard') loadStats();
            if (tabId === 'bindings') loadBindings();
            if (tabId === 'blacklist') { loadBlacklist(); loadGroupBlacklist(); }
            if (tabId === 'blocked') loadBlockedWords();
            if (tabId === 'logs') {
                if (isLive && !logEventSource) {
                    startLiveLogs();
                } else if (!isLive && allLogLines.length === 0) {
                    loadStaticLogs();
                }
            }
            if (tabId === 'system') loadSystemInfo();
        }

        function refreshCurrentTab() {
            loadStats();
            loadBindings();
            loadBlacklist();
            loadGroupBlacklist();
            loadBlockedWords();
            loadSystemInfo();
            if (!isLive) {
                loadStaticLogs();
            }
            showToast('数据已刷新', 'success');
        }

        function showToast(msg, type = 'success') {
            const container = document.getElementById('toast-container');
            const toast = document.createElement('div');
            toast.className = 'toast toast-' + type;
            toast.innerText = msg;
            container.appendChild(toast);

            requestAnimationFrame(() => toast.classList.add('show'));
            setTimeout(() => {
                toast.classList.remove('show');
                setTimeout(() => toast.remove(), 200);
            }, 2600);
        }

        function loadStats() {
            fetch('/api/stats')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        const d = res.data;
                        document.getElementById('stat-total').innerText = d.total_messages;
                        document.getElementById('stat-qq').innerText = d.qq_messages;
                        document.getElementById('stat-yh').innerText = d.yh_messages;
                        document.getElementById('stat-bindings').innerText = d.total_bindings;
                    }
                })
                .catch(() => {});
        }

        function loadBindings() {
            fetch('/api/bindings')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        cachedBindings = res.data || [];
                        renderBindings(cachedBindings);
                    }
                })
                .catch(() => {
                    document.getElementById('bindings-tbody').innerHTML = '<tr><td colspan="5" class="empty-state">获取绑定列表失败</td></tr>';
                });
        }

        function renderBindings(list) {
            const tbody = document.getElementById('bindings-tbody');
            document.getElementById('binding-count').innerText = list.length;
            if (!list || list.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" class="empty-state">暂无绑定的群聊，请在上方添加新绑定</td></tr>';
                return;
            }

            let html = '';
            list.forEach(function(item) {
                let tagClass = 'tag-all';
                if (item.sync_mode === 'QQ到云湖') tagClass = 'tag-qq';
                else if (item.sync_mode === '云湖到QQ') tagClass = 'tag-yh';
                else if (item.sync_mode === '停止') tagClass = 'tag-stopped';

                let qqAvatar = item.qq_avatar_url ? '<img class="user-avatar" src="' + escapeHTML(item.qq_avatar_url) + '" onerror="this.style.display=\'none\'">' : '';
                let yhAvatar = item.yh_avatar_url ? '<img class="user-avatar" src="' + escapeHTML(item.yh_avatar_url) + '" onerror="this.style.display=\'none\'">' : '';
                let yhCountTag = item.yh_headcount ? ' <span style="font-size:11px;color:#0284c7;background:#e0f2fe;padding:1px 5px;border-radius:4px;">👥 ' + item.yh_headcount + '人</span>' : '';

                html += '<tr>' +
                    '<td><div class="user-info-cell">' + qqAvatar + '<div><strong>' + escapeHTML(item.qq_group_name) + '</strong><div style="font-size:11px;color:#64748b;">群号: ' + escapeHTML(item.qq_group_id) + '</div></div></div></td>' +
                    '<td><div class="user-info-cell">' + yhAvatar + '<div><strong>' + escapeHTML(item.yh_group_name) + '</strong>' + yhCountTag + '<div style="font-size:11px;color:#64748b;">群号: ' + escapeHTML(item.yh_group_id) + '</div></div></div></td>' +
                    '<td><span class="tag ' + tagClass + '">' + escapeHTML(item.sync_mode) + '</span></td>' +
                    '<td><select class="select-sm" onchange="changeSyncMode(\'' + item.qq_group_id + '\',\'' + item.yh_group_id + '\',this.value)">' +
                    '<option value="全同步"' + (item.sync_mode === '全同步' ? ' selected' : '') + '>全同步 (双向)</option>' +
                    '<option value="QQ到云湖"' + (item.sync_mode === 'QQ到云湖' ? ' selected' : '') + '>仅 QQ 到云湖</option>' +
                    '<option value="云湖到QQ"' + (item.sync_mode === '云湖到QQ' ? ' selected' : '') + '>仅 云湖 到 QQ</option>' +
                    '<option value="停止"' + (item.sync_mode === '停止' ? ' selected' : '') + '>暂停同步</option>' +
                    '</select></td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-danger btn-sm" onclick="handleUnbind(\'' + item.qq_group_id + '\',\'' + item.yh_group_id + '\')">解除绑定</button>' +
                    '</td></tr>';
            });
            tbody.innerHTML = html;
        }

        function filterBindingsTable() {
            const query = document.getElementById('binding-search').value.trim().toLowerCase();
            if (!query) {
                renderBindings(cachedBindings);
                return;
            }
            const filtered = cachedBindings.filter(item => 
                item.qq_group_id.toLowerCase().includes(query) ||
                item.yh_group_id.toLowerCase().includes(query) ||
                (item.qq_group_name && item.qq_group_name.toLowerCase().includes(query)) ||
                (item.yh_group_name && item.yh_group_name.toLowerCase().includes(query))
            );
            renderBindings(filtered);
        }

        function handleCreateBinding(e) {
            e.preventDefault();
            const qq = document.getElementById('qq-input').value.trim();
            const yh = document.getElementById('yh-input').value.trim();
            const mode = document.getElementById('mode-select').value;

            if (!qq || !yh) {
                showToast('QQ群号与云湖群号均不能为空', 'error');
                return;
            }

            fetch('/api/bindings/bind', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh, sync_mode: mode })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('群聊绑定成功！');
                    document.getElementById('qq-input').value = '';
                    document.getElementById('yh-input').value = '';
                    loadBindings();
                    loadStats();
                } else {
                    showToast(res.msg || '绑定失败', 'error');
                }
            })
            .catch(() => showToast('网络请求失败', 'error'));
        }

        function changeSyncMode(qq, yh, newMode) {
            fetch('/api/bindings/mode', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh, sync_mode: newMode })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已将转发模式更新为: ' + newMode);
                    loadBindings();
                } else {
                    showToast(res.msg || '修改模式失败', 'error');
                }
            })
            .catch(() => showToast('修改模式请求异常', 'error'));
        }

        function handleUnbind(qq, yh) {
            if (!confirm('确定要解除 QQ群 (' + qq + ') 与 云湖群 (' + yh + ') 的互通绑定吗？')) {
                return;
            }

            fetch('/api/bindings/unbind', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('解绑成功');
                    loadBindings();
                    loadStats();
                } else {
                    showToast(res.msg || '解绑失败', 'error');
                }
            })
            .catch(() => showToast('解绑请求异常', 'error'));
        }

        let rawUserBlacklist = [];
        let currentUserPlatformFilter = 'ALL';

        let rawGroupBlacklist = [];
        let currentGroupPlatformFilter = 'ALL';

        function switchBlacklistSubtab(subtab) {
            const userBtn = document.getElementById('subtab-btn-user');
            const groupBtn = document.getElementById('subtab-btn-group');
            const userSec = document.getElementById('blacklist-user-section');
            const groupSec = document.getElementById('blacklist-group-section');

            if (subtab === 'user') {
                userBtn.classList.add('active');
                groupBtn.classList.remove('active');
                userSec.style.display = 'block';
                groupSec.style.display = 'none';
            } else {
                groupBtn.classList.add('active');
                userBtn.classList.remove('active');
                groupSec.style.display = 'block';
                userSec.style.display = 'none';
            }
        }

        function setUserPlatformFilter(plat) {
            currentUserPlatformFilter = plat;
            document.querySelectorAll('#blacklist-user-section .filter-tabs .filter-btn').forEach(btn => btn.classList.remove('active'));
            if (plat === 'ALL') document.getElementById('user-filter-all').classList.add('active');
            if (plat === 'QQ') document.getElementById('user-filter-qq').classList.add('active');
            if (plat === 'YH') document.getElementById('user-filter-yh').classList.add('active');
            applyUserFilterAndRender();
        }

        function setGroupPlatformFilter(plat) {
            currentGroupPlatformFilter = plat;
            document.querySelectorAll('#blacklist-group-section .filter-tabs .filter-btn').forEach(btn => btn.classList.remove('active'));
            if (plat === 'ALL') document.getElementById('group-filter-all').classList.add('active');
            if (plat === 'QQ') document.getElementById('group-filter-qq').classList.add('active');
            if (plat === 'YH') document.getElementById('group-filter-yh').classList.add('active');
            applyGroupFilterAndRender();
        }

        function loadBlacklist() {
            fetch('/api/blacklist')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        rawUserBlacklist = res.data || [];
                        updateUserCounts();
                        applyUserFilterAndRender();
                    }
                })
                .catch(() => {
                    document.getElementById('blacklist-tbody').innerHTML = '<tr><td colspan="5" class="empty-state">加载用户黑名单失败</td></tr>';
                });
        }

        function updateUserCounts() {
            const allCount = rawUserBlacklist.length;
            const qqCount = rawUserBlacklist.filter(item => item.platform === 'QQ').length;
            const yhCount = rawUserBlacklist.filter(item => item.platform === 'YH').length;

            document.getElementById('count-user-all').innerText = allCount;
            document.getElementById('count-user-qq').innerText = qqCount;
            document.getElementById('count-user-yh').innerText = yhCount;
            document.getElementById('tab-user-blacklist-count').innerText = allCount;
        }

        function applyUserFilterAndRender() {
            let filtered = rawUserBlacklist;
            if (currentUserPlatformFilter !== 'ALL') {
                filtered = rawUserBlacklist.filter(item => item.platform === currentUserPlatformFilter);
            }
            renderBlacklist(filtered);
        }

        function renderBlacklist(list) {
            const tbody = document.getElementById('blacklist-tbody');
            document.getElementById('blacklist-count').innerText = list.length;
            if (!list || list.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" class="empty-state">暂无符合条件的黑名单用户</td></tr>';
                return;
            }

            let html = '';
            list.forEach(function(item) {
                let timeStr = '永久封禁';
                if (item.remaining_time > 0) {
                    const mins = Math.ceil(item.remaining_time / 60);
                    timeStr = '约 ' + mins + ' 分钟后自动解封';
                }

                let platTag = '<span class="tag tag-stopped">未知</span>';
                if (item.platform === 'QQ') {
                    platTag = '<span class="tag tag-qq">🐧 QQ</span>';
                } else if (item.platform === 'YH') {
                    platTag = '<span class="tag tag-yh">☁️ 云湖</span>';
                }

                let avatarImg = item.avatar_url ? '<img class="user-avatar" src="' + escapeHTML(item.avatar_url) + '" onerror="this.style.display=\'none\'">' : '';
                let nicknameDisplay = escapeHTML(item.nickname || item.user_id);

                html += '<tr>' +
                    '<td>' + platTag + '</td>' +
                    '<td><div class="user-info-cell">' + avatarImg + '<div><strong>' + nicknameDisplay + '</strong><br><code style="color:var(--text-muted);font-size:11px;">ID: ' + escapeHTML(item.user_id) + '</code></div></div></td>' +
                    '<td>' + escapeHTML(item.reason || '无') + '</td>' +
                    '<td><span class="tag tag-stopped">' + timeStr + '</span></td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-outline btn-sm" onclick="handleRemoveBlacklist(\'' + item.user_id + '\')">解除封禁</button>' +
                    '</td></tr>';
            });
            tbody.innerHTML = html;
        }

        function handleAddBlacklist(e) {
            e.preventDefault();
            const uid = document.getElementById('ban-uid').value.trim();
            const reason = document.getElementById('ban-reason').value.trim();
            const duration = parseInt(document.getElementById('ban-duration').value, 10);

            if (!uid) {
                showToast('用户 ID 不能为空', 'error');
                return;
            }

            fetch('/api/blacklist/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_id: uid, reason: reason, duration: duration })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已添加用户封禁');
                    document.getElementById('ban-uid').value = '';
                    document.getElementById('ban-reason').value = '';
                    loadBlacklist();
                    loadStats();
                } else {
                    showToast(res.msg || '添加黑名单失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        function handleRemoveBlacklist(uid) {
            if (!confirm('确认解封用户 ' + uid + ' 吗？')) return;

            fetch('/api/blacklist/remove', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_id: uid })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已解除用户封禁');
                    loadBlacklist();
                    loadStats();
                } else {
                    showToast(res.msg || '操作失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        // Group Blacklist Functions
        function loadGroupBlacklist() {
            fetch('/api/group_blacklist')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        rawGroupBlacklist = res.data || [];
                        updateGroupCounts();
                        applyGroupFilterAndRender();
                    }
                })
                .catch(() => {
                    document.getElementById('group-blacklist-tbody').innerHTML = '<tr><td colspan="5" class="empty-state">加载群聊黑名单失败</td></tr>';
                });
        }

        function updateGroupCounts() {
            const allCount = rawGroupBlacklist.length;
            const qqCount = rawGroupBlacklist.filter(item => item.platform === 'QQ').length;
            const yhCount = rawGroupBlacklist.filter(item => item.platform === 'YH').length;

            document.getElementById('count-group-all').innerText = allCount;
            document.getElementById('count-group-qq').innerText = qqCount;
            document.getElementById('count-group-yh').innerText = yhCount;
            document.getElementById('tab-group-blacklist-count').innerText = allCount;
        }

        function applyGroupFilterAndRender() {
            let filtered = rawGroupBlacklist;
            if (currentGroupPlatformFilter !== 'ALL') {
                filtered = rawGroupBlacklist.filter(item => item.platform === currentGroupPlatformFilter);
            }
            renderGroupBlacklist(filtered);
        }

        function renderGroupBlacklist(list) {
            const tbody = document.getElementById('group-blacklist-tbody');
            document.getElementById('group-blacklist-count').innerText = list.length;
            if (!list || list.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" class="empty-state">暂无处于黑名单中的群聊</td></tr>';
                return;
            }

            let html = '';
            list.forEach(function(item) {
                let timeStr = '永久封禁';
                if (item.remaining_time > 0) {
                    const mins = Math.ceil(item.remaining_time / 60);
                    timeStr = '约 ' + mins + ' 分钟后自动解封';
                }

                let platTag = '<span class="tag tag-stopped">未知</span>';
                if (item.platform === 'QQ') {
                    platTag = '<span class="tag tag-qq">🐧 QQ</span>';
                } else if (item.platform === 'YH') {
                    platTag = '<span class="tag tag-yh">☁️ 云湖</span>';
                }

                let grpAvatar = item.avatar_url ? '<img class="user-avatar" src="' + escapeHTML(item.avatar_url) + '" onerror="this.style.display=\'none\'">' : '';
                let groupNameDisplay = escapeHTML(item.group_name || ('群聊 ' + item.group_id));
                let countTag = item.headcount ? ' <span style="font-size:11px;color:#0284c7;background:#e0f2fe;padding:1px 5px;border-radius:4px;">👥 ' + item.headcount + '人</span>' : '';
                let introHtml = item.introduction ? '<div style="font-size:11px;color:#64748b;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="' + escapeHTML(item.introduction) + '">' + escapeHTML(item.introduction) + '</div>' : '';

                html += '<tr>' +
                    '<td>' + platTag + '</td>' +
                    '<td><div class="user-info-cell">' + grpAvatar + '<div><strong>' + groupNameDisplay + '</strong>' + countTag + '<br><code style="color:var(--text-muted);font-size:11px;">群号: ' + escapeHTML(item.group_id) + '</code>' + introHtml + '</div></div></td>' +
                    '<td>' + escapeHTML(item.reason || '无') + '</td>' +
                    '<td><span class="tag tag-stopped">' + timeStr + '</span></td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-outline btn-sm" onclick="handleRemoveGroupBlacklist(\'' + item.group_id + '\')">解除封禁</button>' +
                    '</td></tr>';
            });
            tbody.innerHTML = html;
        }

        function handleAddGroupBlacklist(e) {
            e.preventDefault();
            const gid = document.getElementById('group-ban-id').value.trim();
            const reason = document.getElementById('group-ban-reason').value.trim();
            const duration = parseInt(document.getElementById('group-ban-duration').value, 10);

            if (!gid) {
                showToast('群号/群 ID 不能为空', 'error');
                return;
            }

            fetch('/api/group_blacklist/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ group_id: gid, reason: reason, duration: duration })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已添加群聊封禁');
                    document.getElementById('group-ban-id').value = '';
                    document.getElementById('group-ban-reason').value = '';
                    loadGroupBlacklist();
                    loadStats();
                } else {
                    showToast(res.msg || '添加群黑名单失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        function handleRemoveGroupBlacklist(gid) {
            if (!confirm('确认解封群聊 ' + gid + ' 吗？')) return;

            fetch('/api/group_blacklist/remove', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ group_id: gid })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已解除群聊封禁');
                    loadGroupBlacklist();
                    loadStats();
                } else {
                    showToast(res.msg || '操作失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        let isLive = true;
        let logEventSource = null;
        let allLogLines = [];
        let autoScroll = true;

        function startLiveLogs() {
            if (logEventSource) {
                logEventSource.close();
            }
            isLive = true;
            const badge = document.getElementById('log-status-badge');
            if (badge) {
                badge.innerHTML = '<span class="status-dot"></span> 实时接收中 (SSE)';
                badge.style.background = '#ecfdf5';
                badge.style.color = '#047857';
                badge.style.borderColor = '#a7f3d0';
            }
            const btnToggle = document.getElementById('btn-toggle-live');
            if (btnToggle) btnToggle.innerHTML = '⏸️ 暂停实时';
            const btnRef = document.getElementById('btn-refresh-logs');
            if (btnRef) btnRef.style.display = 'none';

            logEventSource = new EventSource('/api/logs/stream');
            logEventSource.onmessage = function(e) {
                if (e.data) {
                    appendLogLine(e.data);
                }
            };
            logEventSource.onerror = function() {
                console.warn('SSE 重连中...');
            };
        }

        function stopLiveLogs() {
            isLive = false;
            if (logEventSource) {
                logEventSource.close();
                logEventSource = null;
            }
            const badge = document.getElementById('log-status-badge');
            if (badge) {
                badge.innerHTML = '⚪ 静态快照 (已暂停实时)';
                badge.style.background = '#f1f5f9';
                badge.style.color = '#475569';
                badge.style.borderColor = '#cbd5e1';
            }
            const btnToggle = document.getElementById('btn-toggle-live');
            if (btnToggle) btnToggle.innerHTML = '▶️ 恢复实时接收';
            const btnRef = document.getElementById('btn-refresh-logs');
            if (btnRef) btnRef.style.display = 'inline-flex';
        }

        function toggleLiveLogs() {
            if (isLive) {
                stopLiveLogs();
            } else {
                startLiveLogs();
            }
        }

        function loadStaticLogs() {
            fetch('/api/logs?lines=500')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success' && res.data && res.data.logs) {
                        allLogLines = res.data.logs;
                        renderTerminalLines();
                        showToast('静态日志快照已刷新');
                    }
                })
                .catch(() => showToast('获取日志失败', 'error'));
        }

        function appendLogLine(rawText) {
            allLogLines.push(rawText);
            if (allLogLines.length > 2500) {
                allLogLines.shift();
            }

            const term = document.getElementById('terminal-container');
            if (!term) return;
            const kw = document.getElementById('log-search-input').value.trim().toLowerCase();
            if (!kw || rawText.toLowerCase().indexOf(kw) !== -1) {
                const lineDiv = document.createElement('div');
                lineDiv.innerHTML = colorizeLogLine(rawText);
                term.appendChild(lineDiv);
                if (autoScroll) {
                    term.scrollTop = term.scrollHeight;
                }
            }
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: ' + term.children.length + ' 行';
        }

        function colorizeLogLine(text) {
            const s = escapeHTML(text);
            if (s.indexOf('Error') !== -1 || s.indexOf('失败') !== -1 || s.indexOf('panic') !== -1 || s.indexOf('FAIL') !== -1) {
                return '<span style="color:#f87171;">' + s + '</span>';
            }
            if (s.indexOf('Success') !== -1 || s.indexOf('成功') !== -1) {
                return '<span style="color:#34d399;">' + s + '</span>';
            }
            if (s.indexOf('Warning') !== -1 || s.indexOf('警告') !== -1) {
                return '<span style="color:#fbbf24;">' + s + '</span>';
            }
            if (s.indexOf('[QQ') !== -1 || s.indexOf('[OneBot') !== -1) {
                return '<span style="color:#38bdf8;">' + s + '</span>';
            }
            if (s.indexOf('[Yunhu') !== -1 || s.indexOf('[YH') !== -1) {
                return '<span style="color:#a78bfa;">' + s + '</span>';
            }
            if (s.indexOf('[WebUI') !== -1 || s.indexOf('[Web Server') !== -1) {
                return '<span style="color:#60a5fa;">' + s + '</span>';
            }
            return '<span style="color:#cbd5e1;">' + s + '</span>';
        }

        function renderTerminalLines() {
            const term = document.getElementById('terminal-container');
            if (!term) return;
            const kw = document.getElementById('log-search-input').value.trim().toLowerCase();
            term.innerHTML = '';
            let count = 0;
            for (let i = 0; i < allLogLines.length; i++) {
                const line = allLogLines[i];
                if (!kw || line.toLowerCase().indexOf(kw) !== -1) {
                    const div = document.createElement('div');
                    div.innerHTML = colorizeLogLine(line);
                    term.appendChild(div);
                    count++;
                }
            }
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: ' + count + ' 行';
            if (autoScroll) {
                term.scrollTop = term.scrollHeight;
            }
        }

        function filterLogs() {
            renderTerminalLines();
        }

        function clearLogsScreen() {
            allLogLines = [];
            const term = document.getElementById('terminal-container');
            if (term) term.innerHTML = '<div style="color:#64748b;padding:8px 0;">已清空屏幕，等待新日志...</div>';
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: 0 行';
        }

        function toggleAutoScroll(enabled) {
            autoScroll = enabled;
            if (autoScroll) {
                const term = document.getElementById('terminal-container');
                if (term) term.scrollTop = term.scrollHeight;
            }
        }

        function exportLogs() {
            if (allLogLines.length === 0) {
                showToast('暂无日志可导出', 'error');
                return;
            }
            const blob = new Blob([allLogLines.join('\n')], { type: 'text/plain;charset=utf-8' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'amer-console-' + new Date().toISOString().replace(/[:.]/g, '-') + '.log';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            showToast('日志已成功导出');
        }

        function loadSystemInfo() {
            fetch('/api/system')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        const d = res.data;
                        const container = document.getElementById('system-info-grid');
                        const port = d.webui_port || d.port;

                        // 更新局域网内网访问提示
                        if (d.lan_ips && d.lan_ips.length > 0) {
                            const mainLanIP = d.lan_ips[0];
                            const navBadge = document.getElementById('nav-lan-badge');
                            if (navBadge) {
                                navBadge.style.display = 'inline-block';
                                navBadge.innerText = '内网: http://' + mainLanIP + ':' + port + '/webui';
                            }
                            const banner = document.getElementById('lan-access-banner');
                            const bannerUrls = document.getElementById('lan-access-urls');
                            if (banner && bannerUrls) {
                                banner.style.display = 'flex';
                                let urlsHtml = '';
                                for (let i = 0; i < d.lan_ips.length; i++) {
                                    const u = 'http://' + d.lan_ips[i] + ':' + port + '/webui';
                                    urlsHtml += '<div>• 内网访问地址 ' + (i + 1) + ': <a href="' + u + '" target="_blank" style="color:#1d4ed8;font-weight:600;text-decoration:underline;">' + u + '</a></div>';
                                }
                                bannerUrls.innerHTML = urlsHtml;
                            }
                        }

                        let lanIPsStr = '未检测到局域网 IP';
                        if (d.lan_ips && d.lan_ips.length > 0) {
                            lanIPsStr = d.lan_ips.map(ip => 'http://' + ip + ':' + port + '/webui').join('<br>');
                        }

                        container.innerHTML =
                            '<div class="info-item"><div class="k">WebUI 独立运行端口</div><div class="v">' + port + '</div></div>' +
                            '<div class="info-item"><div class="k">绑定监听主机 (0.0.0.0 允许局域网访问)</div><div class="v">' + d.webui_host + '</div></div>' +
                            '<div class="info-item" style="grid-column: 1 / -1;"><div class="k">局域网内网访问地址 (其他手机/电脑访问)</div><div class="v" style="color:var(--primary);word-break:break-all;">' + lanIPsStr + '</div></div>' +
                            '<div class="info-item"><div class="k">云湖接收模式</div><div class="v">' + d.yh_mode + '</div></div>' +
                            '<div class="info-item"><div class="k">QQ 机器人账号</div><div class="v">' + (d.bot_qq || '未指定') + '</div></div>' +
                            '<div class="info-item"><div class="k">图片缩放比例</div><div class="v">' + (d.image_scale || '默认') + '</div></div>' +
                            '<div class="info-item"><div class="k">图片限制尺寸 (宽x高)</div><div class="v">' + (d.image_max_w || 300) + 'px × ' + (d.image_max_h || 320) + 'px</div></div>' +
                            '<div class="info-item"><div class="k">屏蔽词总数</div><div class="v">' + d.blocked_sum + ' 个</div></div>' +
                            '<div class="info-item"><div class="k">临时缓存目录</div><div class="v">' + (d.temp_folder || 'temp') + '</div></div>';
                    }
                })
                .catch(() => {});
        }

        function loadBlockedWords() {
            fetch('/api/blocked_words')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success' && res.data) {
                        cachedBlockedWords = res.data.categories || {};
                        renderBlockedWords(cachedBlockedWords);
                        updateCategoryDatalist(cachedBlockedWords);
                    }
                })
                .catch(() => {
                    const c = document.getElementById('blocked-words-container');
                    if (c) c.innerHTML = '<div class="empty-state">获取屏蔽词数据失败</div>';
                });
        }

        function updateCategoryDatalist(cats) {
            const dl = document.getElementById('category-datalist');
            if (!dl) return;
            let options = '';
            for (let cat in cats) {
                options += '<option value="' + escapeHTML(cat) + '"></option>';
            }
            dl.innerHTML = options;
        }

        function renderBlockedWords(cats) {
            const container = document.getElementById('blocked-words-container');
            if (!container) return;

            const kw = document.getElementById('bw-search-input') ? document.getElementById('bw-search-input').value.trim().toLowerCase() : '';
            const catKeys = Object.keys(cats);
            let totalWords = 0;
            catKeys.forEach(k => totalWords += (cats[k] || []).length);

            const catCountEl = document.getElementById('bw-cat-count');
            const totalCountEl = document.getElementById('bw-total-count');
            if (catCountEl) catCountEl.innerText = catKeys.length;
            if (totalCountEl) totalCountEl.innerText = totalWords;

            if (catKeys.length === 0) {
                container.innerHTML = '<div class="empty-state">暂无屏蔽词配置，请在上方添加分类与词语</div>';
                return;
            }

            let html = '';
            let matchCatCount = 0;

            catKeys.forEach(function(catName) {
                const words = cats[catName] || [];
                const filteredWords = kw ? words.filter(w => w.toLowerCase().includes(kw) || catName.toLowerCase().includes(kw)) : words;

                if (kw && filteredWords.length === 0 && !catName.toLowerCase().includes(kw)) {
                    return;
                }
                matchCatCount++;

                html += '<div class="category-box">' +
                    '<div class="category-header">' +
                    '<div class="category-title">📂 ' + escapeHTML(catName) + ' <span style="font-size:12px;font-weight:normal;color:#64748b;">(' + words.length + ' 个词)</span></div>' +
                    '<button class="btn btn-danger btn-sm" onclick="handleDeleteCategory(\'' + escapeHTML(catName) + '\')">🗑️ 删除整个分类</button>' +
                    '</div><div style="display:flex;flex-wrap:wrap;gap:4px;">';

                if (filteredWords.length === 0) {
                    html += '<span style="font-size:12px;color:#94a3b8;">未匹配到屏蔽词</span>';
                } else {
                    filteredWords.forEach(function(w) {
                        html += '<span class="chip"><span>' + escapeHTML(w) + '</span>' +
                            '<span class="chip-del" title="删除此屏蔽词" onclick="handleDeleteBlockedWord(\'' + escapeHTML(catName) + '\',\'' + escapeHTML(w) + '\')">✕</span></span>';
                    });
                }

                html += '</div></div>';
            });

            if (matchCatCount === 0) {
                container.innerHTML = '<div class="empty-state">未找到包含关键词 "' + escapeHTML(kw) + '" 的屏蔽词或分类</div>';
            } else {
                container.innerHTML = html;
            }
        }

        function filterBlockedWords() {
            renderBlockedWords(cachedBlockedWords);
        }

        function handleAddBlockedWords(e) {
            e.preventDefault();
            const cat = document.getElementById('bw-category-input').value.trim();
            const wordsRaw = document.getElementById('bw-words-input').value.trim();

            if (!cat || !wordsRaw) {
                showToast('分类名称和屏蔽词均不能为空', 'error');
                return;
            }

            fetch('/api/blocked_words/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ category: cat, raw_words: wordsRaw })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast(res.msg || '屏蔽词添加成功，已热更新生效！');
                    document.getElementById('bw-words-input').value = '';
                    loadBlockedWords();
                    loadSystemInfo();
                } else {
                    showToast(res.msg || '添加失败', 'error');
                }
            })
            .catch(() => showToast('添加屏蔽词请求失败', 'error'));
        }

        function handleDeleteBlockedWord(cat, word) {
            fetch('/api/blocked_words/delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ category: cat, word: word })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast(res.msg || '已删除屏蔽词并即时热更新！');
                    loadBlockedWords();
                    loadSystemInfo();
                } else {
                    showToast(res.msg || '删除失败', 'error');
                }
            })
            .catch(() => showToast('删除屏蔽词请求失败', 'error'));
        }

        function handleDeleteCategory(cat) {
            if (!confirm('确定要删除屏蔽词分类【' + cat + '】及其下的所有词语吗？')) {
                return;
            }

            fetch('/api/blocked_words/delete_category', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ category: cat })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast(res.msg || '已删除分类并即时热更新！');
                    loadBlockedWords();
                    loadSystemInfo();
                } else {
                    showToast(res.msg || '删除分类失败', 'error');
                }
            })
            .catch(() => showToast('删除分类请求失败', 'error'));
        }

        function handleReloadConfig() {
            fetch('/api/config/reload', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast(res.msg || 'config.yaml 配置文件已成功重新热加载！');
                    loadBlockedWords();
                    loadSystemInfo();
                    loadStats();
                } else {
                    showToast(res.msg || '热重载配置失败', 'error');
                }
            })
            .catch(() => showToast('热重载配置请求失败', 'error'));
        }

        function escapeHTML(str) {
            if (!str) return '';
            return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        }

        loadStats();
        loadSystemInfo();
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
