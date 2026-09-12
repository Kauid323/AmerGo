package yunhu

import (
	"amer/adapter/message"
	"amer/config"
	"amer/db"
	"amer/model"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// reportCooldownMap records user report cooldowns and attempt counts
	// key: reporterID + "_" + msgID (or just reporterID)
	reportCooldownMap sync.Map
	// adminActionClickMap records admin action clicks for reportID
	adminActionClickMap sync.Map
	// recentYunhuEventIDs tracks recently received event IDs to prevent duplicate execution
	recentYunhuEventIDs sync.Map
)

func isDuplicateYunhuEvent(eventID string) bool {
	if eventID == "" {
		return false
	}
	now := time.Now()
	if val, loaded := recentYunhuEventIDs.LoadOrStore(eventID, now); loaded {
		if t, ok := val.(time.Time); ok && now.Sub(t) < 5*time.Minute {
			return true
		}
		recentYunhuEventIDs.Store(eventID, now)
	}
	return false
}

type reportCooldownItem struct {
	FirstReportTime time.Time
	AttemptCount    int
}

type adminClickItem struct {
	ClickCount int
	LastClick  time.Time
}

func parseGroupIDs(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	input = strings.ReplaceAll(input, "，", ",")
	parts := strings.Split(input, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res
}

type ReportData struct {
	Platform     string `json:"platform"`
	GroupID      string `json:"group_id"`
	MsgID        string `json:"msg_id"`
	YunhuMsgID   string `json:"yunhu_msg_id"`
	ChatID       string `json:"chat_id"`
	ChatType     string `json:"chat_type"`
	SenderID     string `json:"sender_id"`
	SenderName   string `json:"sender_name"`
	ReporterID   string `json:"reporter_id"`
	ReporterName string `json:"reporter_name"`
	Reason       string `json:"reason"`
}

func NotifyAdminReport(report *db.ReportRecord) {
	adminID := config.AppConfig.YH.AdminID
	if adminID == "" {
		log.Printf("[Report Admin] 云湖管理员 ID (yh.admin_id) 未配置，跳过推送通知")
		return
	}

	reporterInfo := report.ReporterName
	if report.ReporterID != "" {
		reporterInfo = fmt.Sprintf("%s (ID: %s)", report.ReporterName, report.ReporterID)
	}

	// 1. 确定真实的云湖群聊与云湖消息 ID
	yhChatID := report.ChatID
	yhChatType := report.ChatType
	if yhChatType == "" {
		yhChatType = "group"
	}
	if (yhChatID == "" || yhChatID == report.GroupID) && report.GroupID != "" {
		if bindInfo := db.GetInfo("QQ", report.GroupID); bindInfo.Status == 0 {
			if yhGroupIDs, ok := bindInfo.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
				yhChatID = yhGroupIDs[0].ID
			}
		}
	}

	var candidateYHMsgIDs []string
	if report.YunhuMsgID != "" {
		candidateYHMsgIDs = append(candidateYHMsgIDs, report.YunhuMsgID)
	}

	var qqMsgID int64
	if id, err := strconv.ParseInt(report.MsgID, 10, 64); err == nil && id != 0 {
		qqMsgID = id
	}
	if qqMsgID != 0 {
		if realID, ok := db.GetYunhuMsgIDByQQMsgID(qqMsgID); ok && realID != "" {
			candidateYHMsgIDs = append(candidateYHMsgIDs, realID)
		}
	}
	if cachedYHID, _, _, _, found := db.GetYunhuMsgCache(report.MsgID); found && cachedYHID != "" {
		candidateYHMsgIDs = append(candidateYHMsgIDs, cachedYHID)
	}
	if len(report.MsgID) >= 20 {
		candidateYHMsgIDs = append(candidateYHMsgIDs, report.MsgID)
	}

	var originalYHItem *YunhuMessageItem
	if YHClient != nil && yhChatID != "" {
		for _, targetYHMsgID := range candidateYHMsgIDs {
			if targetYHMsgID == "" || strings.HasPrefix(targetYHMsgID, "-") {
				continue
			}
			yhItem, err := YHClient.GetMessage(yhChatID, yhChatType, targetYHMsgID)
			if err == nil && yhItem != nil {
				originalYHItem = yhItem
				log.Printf("[Report Admin] 成功通过云湖官方 API 获取被举报原消息 (MsgID: %s, 群: %s)", targetYHMsgID, yhChatID)
				break
			} else {
				log.Printf("[Report Admin Debug] 通过云湖官方 API 获取消息 (MsgID: %s, 群: %s) 失败: %v", targetYHMsgID, yhChatID, err)
			}
		}
	}

	// 2. 原汁原味直接调用发送消息 API 重新将原消息推送给管理员（不做任何二次 HTML 或 Markdown 代码块包装）
	sentOriginal := false
	if originalYHItem != nil {
		if _, err := YHClient.SendMessageItem(adminID, "user", originalYHItem); err == nil {
			sentOriginal = true
			log.Printf("[Report Admin] 成功将被举报云湖原消息通过发送 API 原生转发给管理员 %s", adminID)
		} else {
			log.Printf("[Report Admin Error] 转发被举报云湖原消息失败: %v", err)
		}
	}

	// 兜底：如果云湖接口没取到，但有 QQ 端原消息，也直接以原 HTML 发送
	if !sentOriginal {
		var rawQQMsg string
		var qGroupID int64
		if report.GroupID != "" {
			_, _ = fmt.Sscanf(report.GroupID, "%d", &qGroupID)
		}
		if qqMsgID != 0 && message.GlobalQQSender != nil {
			if msgInfo, err := message.GlobalQQSender.GetReplyMsg(qqMsgID, qGroupID); err == nil && msgInfo != nil {
				if msgInfo.RawText != "" {
					rawQQMsg = msgInfo.RawText
				} else if msgInfo.Summary != "" {
					rawQQMsg = msgInfo.Summary
				}
			}
		}
		if rawQQMsg == "" && qqMsgID != 0 {
			if mapping, ok := db.GetQQMsgInfo(qqMsgID); ok && mapping != nil && mapping.RawText != "" {
				rawQQMsg = mapping.RawText
			}
		}
		if rawQQMsg != "" {
			if strings.Contains(rawQQMsg, "[CQ:video") {
				videoURL := message.ExtractVideoURL(rawQQMsg)
				if videoURL != "" {
					surfaceID := fmt.Sprintf("video_%s_%d", report.MsgID, time.Now().UnixNano())
					a2uiJSON := message.BuildVideoA2UI(surfaceID, fmt.Sprintf("QQ群-%s", report.GroupID), report.GroupID, report.SenderName, report.SenderID, videoURL, report.MsgID)
					if _, err := YHClient.Send(adminID, "user", "a2ui", a2uiJSON); err == nil {
						sentOriginal = true
						log.Printf("[Report Admin] 成功将被举报 QQ 视频消息转为 A2UI 原生推送给管理员 %s", adminID)
					}
				}
			} else if strings.Contains(rawQQMsg, "[CQ:record") {
				audioURL, fileVal := message.ExtractAudioURL(rawQQMsg)
				if audioURL != "" || fileVal != "" {
					audioPlayURI := message.FetchAudioDataURI(audioURL, fileVal)
					surfaceID := fmt.Sprintf("audio_%s_%d", report.MsgID, time.Now().UnixNano())
					a2uiJSON := message.BuildAudioA2UI(surfaceID, fmt.Sprintf("QQ群-%s", report.GroupID), report.GroupID, report.SenderName, report.SenderID, audioPlayURI, report.MsgID)
					if _, err := YHClient.Send(adminID, "user", "a2ui", a2uiJSON); err == nil {
						sentOriginal = true
						log.Printf("[Report Admin] 成功将被举报 QQ 语音消息转为 A2UI 原生推送给管理员 %s", adminID)
					}
				}
			}
			if !sentOriginal {
				rawHTML := message.CQToHTMLWithGroup(rawQQMsg, qGroupID)
				if _, err := YHClient.Send(adminID, "user", "html", rawHTML); err == nil {
					sentOriginal = true
					log.Printf("[Report Admin] 成功将被举报 QQ 原消息转为富文本原生推送给管理员 %s", adminID)
				}
			}
		}
	}

	originTip := "ℹ️ **提示**: 被举报原消息内容已在上方重新发送供您审阅。"
	if !sentOriginal {
		originTip = "ℹ️ **提示**: 暂未能通过接口拉取到原多媒体消息原文，请核对消息ID。"
	}

	text := fmt.Sprintf("🚨 **收到新的违规消息举报通知**\n\n"+
		"- **举报 ID**: `%s`\n"+
		"- **消息 ID**: `%s`\n"+
		"- **来源群聊**: `QQ群-%s`\n"+
		"- **被举报用户 ID**: `%s`\n"+
		"- **举报原因**: %s\n"+
		"- **举报人**: `%s`\n"+
		"- **提交时间**: %s\n\n"+
		"%s\n\n"+
		"请审核并选择处理操作：",
		report.ReportID, report.MsgID, report.GroupID, report.SenderID, report.Reason, reporterInfo, report.CreatedAt, originTip)

	buttons := []interface{}{
		[]map[string]interface{}{
			{
				"text":       "✅ 同意 (封禁该用户 1 小时)",
				"actionType": 3,
				"value":      fmt.Sprintf("report_action:approve:%s", report.ReportID),
			},
			{
				"text":       "❌ 拒绝 (忽略该举报)",
				"actionType": 3,
				"value":      fmt.Sprintf("report_action:reject:%s", report.ReportID),
			},
			{
				"text":       "🗑️ 撤回原消息",
				"actionType": 3,
				"value":      fmt.Sprintf("report_action:recall:%s", report.ReportID),
			},
		},
	}

	if err := YHClient.SendWithButtons(adminID, "user", "markdown", text, buttons); err != nil {
		log.Printf("[Report Admin Error] 向管理员 %s 发送举报处理通知失败: %v", adminID, err)
	} else {
		log.Printf("[Report Admin] 已向管理员 %s 推送违规举报通知 (ID: %s)", adminID, report.ReportID)
	}
}

func ProcessReportData(req ReportData) (bool, string) {
	if req.MsgID == "" {
		return false, "缺少必要参数: msgId"
	}
	if req.Reason == "" {
		req.Reason = "违规内容举报"
	}
	if req.Platform == "" {
		req.Platform = "QQ"
	}
	if req.SenderID == "" {
		req.SenderID = "0"
	}
	if req.SenderName == "" {
		req.SenderName = "匿名用户"
	}

	// 优先使用请求传入的云湖群与消息 ID，如未传则尝试反查
	chatID := req.ChatID
	chatType := req.ChatType
	if chatType == "" {
		chatType = "group"
	}
	realYHMsgID := req.YunhuMsgID
	if realYHMsgID == "" {
		if rID, cID, cType, cachedSenderID, found := db.GetYunhuMsgCache(req.MsgID); found {
			if rID != "" && !strings.HasPrefix(rID, "-") {
				realYHMsgID = rID
			}
			if chatID == "" {
				chatID = cID
				chatType = cType
			}
			if cachedSenderID != "" && (req.SenderID == "0" || req.SenderID == "") {
				req.SenderID = cachedSenderID
			}
		}
	}
	if realYHMsgID == "" {
		if qqID, err := strconv.ParseInt(req.MsgID, 10, 64); err == nil && qqID != 0 {
			if rID, ok := db.GetYunhuMsgIDByQQMsgID(qqID); ok && rID != "" {
				realYHMsgID = rID
			}
		}
	}
	if realYHMsgID == "" && len(req.MsgID) >= 20 {
		realYHMsgID = req.MsgID
	}

	if chatID == "" {
		// Check if req.GroupID is a QQ group ID bound to a YH group ID
		bindInfo := db.GetInfo("QQ", req.GroupID)
		if bindInfo.Status == 0 {
			if yhGroupIDs, ok := bindInfo.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
				chatID = yhGroupIDs[0].ID
				chatType = "group"
			}
		}
		if chatID == "" {
			chatID = req.GroupID
			chatType = "group"
		}
	}

	reportID := fmt.Sprintf("rep_%d", time.Now().UnixNano())
	createdAt := time.Now().Format("2006-01-02 15:04:05")

	// 优先保留被举报的目标消息原始 ID (例如 QQ 消息 ID)，如果是云湖消息则为云湖消息 ID
	targetMsgID := req.MsgID
	if targetMsgID == "" {
		targetMsgID = realYHMsgID
	}

	reportRecord := &db.ReportRecord{
		ReportID:     reportID,
		MsgID:        targetMsgID,
		YunhuMsgID:   realYHMsgID,
		ChatID:       chatID,
		ChatType:     chatType,
		GroupID:      req.GroupID,
		SenderID:     req.SenderID,
		SenderName:   req.SenderName,
		ReporterID:   req.ReporterID,
		ReporterName: req.ReporterName,
		Reason:       req.Reason,
		Status:       "PENDING",
		CreatedAt:    createdAt,
	}

	_ = db.SaveReportRecord(reportRecord)

	reportContent := fmt.Sprintf("【消息举报 ID: %s】\n消息ID: %s\n被举报人: %s(ID: %s)\n举报原因: %s\n举报人: %s(ID: %s)\n提交时间: %s",
		reportID, req.MsgID, req.SenderName, req.SenderID, req.Reason, req.ReporterName, req.ReporterID, createdAt)

	// 1. Store in Redis sensitive messages list
	db.StoreSensitiveMessage("REPORT", req.GroupID, req.SenderID, req.SenderName, reportContent)

	// 2. Count reports for this sender
	if req.SenderID != "0" && req.SenderID != "" {
		reportCountKey := fmt.Sprintf("report_count:%s:%s", req.Platform, req.SenderID)
		count, _ := db.RDB.Incr(db.Ctx, reportCountKey).Result()
		if count == 1 {
			_ = db.RDB.Expire(db.Ctx, reportCountKey, 24*time.Hour)
		}

		log.Printf("[Report] 收到对用户 %s (ID: %s) 的举报 (ReportID: %s)，24小时内累计举报数: %d，原因: %s", req.SenderName, req.SenderID, reportID, count, req.Reason)

		// 3. If reports exceed threshold (e.g. 3 reports), automatically block the user
		if count >= 3 {
			_ = db.AddToBlacklist(req.SenderID, fmt.Sprintf("累计收到 %d 次违规举报 (最后原因: %s)", count, req.Reason), 3600)
			log.Printf("[Report Ban] 用户 %s 被累计举报 %d 次，已自动加入黑名单封禁 1 小时", req.SenderID, count)
		}
	}

	// 4. Send notification to admin
	NotifyAdminReport(reportRecord)

	return true, fmt.Sprintf("举报 (ID: %s) 已成功提交，管理员将尽快核实处理", reportID)
}

func HandleYunhuEvent(event model.YunhuEvent) {
	eventID := strings.TrimSpace(event.Header.EventID.String())
	if eventID != "" && isDuplicateYunhuEvent(eventID) {
		log.Printf("[Yunhu Event] 忽略重复事件 ID: %s", eventID)
		return
	}

	eventType := event.Header.EventType.String()
	log.Printf("[Yunhu Event] 收到事件: %s, 事件ID: %s", eventType, eventID)

	switch {
	case eventType == "message.receive.normal":
		handleNormalMessage(event)
	case eventType == "message.receive.instruction":
		actionName := event.Event.ActionName.String()
		if actionName == "" {
			actionName = event.Event.Value.String()
		}
		if strings.HasPrefix(actionName, "report_action:") {
			handleA2UIButtonEvent(event)
		} else {
			handleInstructionMessage(event)
		}
	case strings.HasPrefix(eventType, "a2ui.button.") || strings.HasPrefix(eventType, "button.") || eventType == "bot.button.click":
		handleA2UIButtonEvent(event)
	case eventType == "bot.followed":
		handleBotFollowed(event)
	case eventType == "bot.unfollowed":
		log.Printf("[Yunhu] 用户 %s 取消关注机器人", event.Event.Sender.SenderNickname)
	case eventType == "group.join":
		log.Printf("[Yunhu] 用户 %s 加入群聊 %s", event.Event.Sender.SenderNickname, event.Event.GetChatID())
	case eventType == "group.leave":
		log.Printf("[Yunhu] 用户 %s 离开群聊 %s", event.Event.Sender.SenderNickname, event.Event.GetChatID())
	default:
		if event.Event.ActionName.String() != "" || event.Event.Value.String() != "" {
			handleA2UIButtonEvent(event)
		} else {
			log.Printf("[Yunhu] 未知或未处理事件类型: %s", eventType)
		}
	}
}

func handleNormalMessage(event model.YunhuEvent) {
	msg := event.Event.Message
	sender := event.Event.Sender
	chatID := event.Event.GetChatID()
	chatType := event.Event.GetChatType()
	senderID := sender.SenderID.String()

	if senderID == config.AppConfig.QQ.BotQQ || sender.SenderType == "bot" {
		return
	}

	if chatID != "" {
		if banStatus, err := db.IsGroupInBlacklist(chatID); err == nil && banStatus.IsBanned {
			log.Printf("[Yunhu Filter] 群/频道 %s 处于群黑名单中，忽略消息", chatID)
			return
		}
	}

	// Cache Yunhu message ID, chatID, chatType for potential recall
	if msg.MsgID.String() != "" {
		db.SaveYunhuMsgCache(msg.MsgID.String(), chatID, chatType, senderID, msg.MsgID.String())
	}

	msgType := "text"
	content := msg.Content.Text
	if content != "" {
		content = message.ConvertYunhuEmoji(content)
	}
	videoLocalPath := ""
	var tempFilesToClean []string

	// 如果文本中含有云湖图床链接，先下载到本地并替换为本地 file:/// 路径，防止 OneBot 报 403
	if content != "" && strings.Contains(content, "chat-img.jwznb.com") {
		var imgFiles []string
		content, imgFiles = replaceYunhuImagesInText(content)
		tempFilesToClean = append(tempFilesToClean, imgFiles...)
	}

	if content == "" {
		if msg.Content.ImageURL != "" {
			imgURL := msg.Content.ImageURL
			if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
				imgURL = "https://chat-img.jwznb.com/" + strings.TrimPrefix(imgURL, "/")
			}
			localImgPath, err := downloadYunhuImageToLocal(imgURL)
			if err == nil && localImgPath != "" {
				tempFilesToClean = append(tempFilesToClean, localImgPath)
				localURI := "file:///" + filepath.ToSlash(localImgPath)
				content = fmt.Sprintf("[CQ:image,file=%s]", localURI)
				log.Printf("[Yunhu Image] 图片已下载到本地: %s (原URL: %s)", localURI, imgURL)
			} else {
				log.Printf("[Yunhu Image Warning] 下载图片到本地失败 (%v)，回退至网络 URL", err)
				content = fmt.Sprintf("[CQ:image,file=%s]", imgURL)
			}
			msgType = "image"
		} else if msg.Content.VideoURL != "" {
			videoURL := msg.Content.VideoURL
			if !strings.HasPrefix(videoURL, "http://") && !strings.HasPrefix(videoURL, "https://") {
				videoURL = "https://chat-video1.jwznb.com/" + strings.TrimPrefix(videoURL, "/")
			}
			tempFolder := config.AppConfig.TempFolder
			if tempFolder == "" {
				tempFolder = "utils/temp"
			}
			_ = os.MkdirAll(tempFolder, 0755)
			fileName := fmt.Sprintf("video_%d.mp4", time.Now().UnixNano())
			targetPath := filepath.Join(tempFolder, fileName)
			absPath, err := filepath.Abs(targetPath)
			if err == nil {
				log.Printf("[Yunhu Video Download] 正在下载云湖视频至本地: %s (URL: %s)", absPath, videoURL)
				errDownload := downloadFile(videoURL, absPath)
				if errDownload == nil {
					videoLocalPath = absPath
					content = absPath
					msgType = "video"
					tempFilesToClean = append(tempFilesToClean, absPath)
					log.Printf("[Yunhu Video Download] 视频成功下载至本地: %s", absPath)
				} else {
					log.Printf("[Yunhu Video Download Error] 视频下载失败: %v", errDownload)
					content = fmt.Sprintf("[视频消息: %s]", videoURL)
				}
			} else {
				content = fmt.Sprintf("[视频消息: %s]", videoURL)
			}
		}
	}

	if content == "" {
		return
	}

	if handled, replyMsg := message.HandleAdminCommand("YH", senderID, content); handled {
		if chatID != "" {
			_, _ = YHClient.Send(chatID, chatType, "text", replyMsg)
		}
		return
	}

	yhGroupName := YHClient.GetGroupName(chatID)
	parentIDStr := strings.TrimSpace(msg.ParentID.String())
	parentSummary := strings.TrimSpace(msg.Content.Parent)

	replyPrefix := ""
	if parentIDStr != "" {
		replyPrefix = message.BuildYunhuReplyCQPrefix(parentIDStr, parentSummary)
		if replyPrefix == "" && parentSummary != "" {
			// 如果未能映射到具体 QQ 消息 ID，退回展示引用文字摘要
			cleanParent := message.ReplaceBlockedWords(parentSummary)
			replyPrefix = fmt.Sprintf("「引用: %s」\n", cleanParent)
		}
	}

	formattedQQMsg := content
	if msgType == "text" {
		formattedQQMsg = fmt.Sprintf("%s[%s] %s(%s):\n%s", replyPrefix, yhGroupName, sender.SenderNickname.String(), senderID, content)
	} else if msgType == "image" {
		formattedQQMsg = fmt.Sprintf("%s[%s] %s(%s):\n%s", replyPrefix, yhGroupName, sender.SenderNickname.String(), senderID, content)
	} else if msgType == "video" {
		formattedQQMsg = fmt.Sprintf("%s[%s] %s(%s):\n[CQ:video,file=file:///%s]", replyPrefix, yhGroupName, sender.SenderNickname.String(), senderID, filepath.ToSlash(videoLocalPath))
	}

	message.SendToAllBindings("YH", chatID, msgType, content, senderID, sender.SenderNickname.String(), formattedQQMsg, msg.MsgID.String())

	// 延迟异步清理下载的本地临时图片与视频文件
	if len(tempFilesToClean) > 0 {
		go func(files []string) {
			time.Sleep(30 * time.Second)
			for _, f := range files {
				if err := os.Remove(f); err == nil {
					log.Printf("[Yunhu Temp Clean] 已自动清理临时文件: %s", f)
				}
			}
		}(tempFilesToClean)
	}
}

// downloadYunhuImageToLocal downloads image from chat-img.jwznb.com to local temp folder with proper headers and returns absolute path.
func downloadYunhuImageToLocal(imgURL string) (string, error) {
	if imgURL == "" {
		return "", fmt.Errorf("empty image url")
	}
	if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
		imgURL = "https://chat-img.jwznb.com/" + strings.TrimPrefix(imgURL, "/")
	}

	tempFolder := config.AppConfig.TempFolder
	if tempFolder == "" {
		tempFolder = "utils/temp"
	}
	_ = os.MkdirAll(tempFolder, 0755)

	tmpFileName := fmt.Sprintf("tmp_%d.bin", time.Now().UnixNano())
	tmpFilePath := filepath.Join(tempFolder, tmpFileName)

	if err := downloadFile(imgURL, tmpFilePath); err != nil {
		return "", err
	}

	// 嗅探文件魔数确定真实图片扩展名
	ext := ".jpg"
	if f, err := os.Open(tmpFilePath); err == nil {
		header := make([]byte, 16)
		n, _ := f.Read(header)
		f.Close()
		if n >= 2 && header[0] == 0xFF && header[1] == 0xD8 {
			ext = ".jpg"
		} else if n >= 8 && string(header[1:4]) == "PNG" {
			ext = ".png"
		} else if n >= 4 && string(header[0:3]) == "GIF" {
			ext = ".gif"
		} else if n >= 12 && string(header[0:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
			ext = ".webp"
		} else {
			ext = getExt(imgURL, ".jpg")
		}
	}

	finalName := fmt.Sprintf("yh_img_%d%s", time.Now().UnixNano(), ext)
	finalPath := filepath.Join(tempFolder, finalName)
	if err := os.Rename(tmpFilePath, finalPath); err != nil {
		finalPath = tmpFilePath
	}

	absPath, err := filepath.Abs(finalPath)
	if err != nil {
		return finalPath, nil
	}
	return absPath, nil
}

// replaceYunhuImagesInText searches for chat-img.jwznb.com URLs in text, downloads them, and replaces with file:/// paths.
func replaceYunhuImagesInText(text string) (string, []string) {
	if !strings.Contains(text, "chat-img.jwznb.com") {
		return text, nil
	}

	var tempFiles []string
	reImg := regexp.MustCompile(`https?://chat-img\.jwznb\.com/[^\s\)\"\'\,]+`)
	res := reImg.ReplaceAllStringFunc(text, func(imgURL string) string {
		localPath, err := downloadYunhuImageToLocal(imgURL)
		if err == nil && localPath != "" {
			tempFiles = append(tempFiles, localPath)
			return "file:///" + filepath.ToSlash(localPath)
		}
		return imgURL
	})
	return res, tempFiles
}

func getExt(url string, defaultExt string) string {
	lower := strings.ToLower(url)
	if strings.Contains(lower, ".png") {
		return ".png"
	} else if strings.Contains(lower, ".gif") {
		return ".gif"
	} else if strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") {
		return ".jpg"
	} else if strings.Contains(lower, ".mp4") {
		return ".mp4"
	}
	return defaultExt
}

func downloadFile(url string, filePath string) error {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: tr,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://myapp.jwznb.com/")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func handleInstructionMessage(event model.YunhuEvent) {
	msg := event.Event.Message
	sender := event.Event.Sender
	chatID := event.Event.GetChatID()
	chatType := event.Event.GetChatType()
	cmdName := msg.CommandName.String()
	if cmdName == "" {
		cmdName = msg.InstructionName.String()
	}
	senderID := sender.SenderID.String()

	log.Printf("[Yunhu Instruction] 收到指令: %s, 来自: %s, chatID: %s, chatType: %s", cmdName, sender.SenderNickname, chatID, chatType)

	if chatID != "" {
		if banStatus, err := db.IsGroupInBlacklist(chatID); err == nil && banStatus.IsBanned {
			log.Printf("[Yunhu Filter] 群/频道 %s 处于群黑名单中，忽略指令", chatID)
			return
		}
	}

	if chatType == "group" {
		switch cmdName {
		case "帮助":
			_, _ = YHClient.Send(chatID, chatType, "markdown", config.AppConfig.Messages.MessageYH)

		case "群列表":
			bindInfos := db.GetInfo("YH", chatID)
			if bindInfos.Status == 0 {
				menu := fmt.Sprintf("## 🌟 云湖群: %s 绑定信息\n\n", YHClient.GetGroupName(chatID))
				if qqGroupIDs, ok := bindInfos.Data["QQ_group_ids"].([]db.BindingItem); ok && len(qqGroupIDs) > 0 {
					menu += "### 📋 QQ群列表\n\n"
					for idx, q := range qqGroupIDs {
						syncMode := "未设置"
						if q.Sync && q.BindingSync {
							syncMode = "互通"
						} else if q.Sync && !q.BindingSync {
							syncMode = "单向-云湖到QQ"
						} else if !q.Sync && q.BindingSync {
							syncMode = "单向-QQ到云湖"
						}
						qqName := fmt.Sprintf("QQ群-%s", q.ID)
						if message.GlobalQQSender != nil {
							var qID int64
							fmt.Sscanf(q.ID, "%d", &qID)
							if name := message.GlobalQQSender.GetGroupName(qID); name != "" {
								qqName = name
							}
						}
						menu += fmt.Sprintf("%d. **%s** (%s) - 同步模式: %s\n", idx+1, qqName, q.ID, syncMode)
					}
				} else {
					menu += "未绑定任何 QQ 群。\n"
				}
				_, _ = YHClient.Send(chatID, chatType, "markdown", menu)
			}

		case "解绑":
			targetGroup := msg.Content.GetFormFieldString("group_id")
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldString("QQ群号")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldString("群号")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldByFuzzyLabel("群号", "群", "qq")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFirstInputFieldValue()
			}
			if targetGroup == "" {
				targetGroup = strings.TrimSpace(strings.TrimPrefix(msg.Content.Text, "/解绑"))
			}
			if targetGroup == "" {
				_, _ = YHClient.Send(chatID, chatType, "text", "缺少参数，用法: /解绑 <QQ群号/全部>")
				return
			}
			var res db.BindInfoResult
			if targetGroup == "全部" {
				res = db.UnbindAll("YH", chatID)
			} else {
				res = db.Unbind("YH", "QQ", chatID, targetGroup)
				if res.Status == 0 && message.GlobalQQSender != nil {
					var qID int64
					fmt.Sscanf(targetGroup, "%d", &qID)
					yhGroupName := YHClient.GetGroupName(chatID)
					_, _ = message.GlobalQQSender.SendGroupMsg(qID, fmt.Sprintf("【Amer 解绑通知】本 QQ 群与云湖群「%s」(ID: %s) 的绑定已解除！", yhGroupName, chatID))
				}
			}
			_, _ = YHClient.Send(chatID, chatType, "text", res.Msg)

		case "绑定":
			targetGroup := msg.Content.GetFormFieldString("group_id")
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldString("QQ群号")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldString("群号")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFormFieldByFuzzyLabel("群号", "群", "qq")
			}
			if targetGroup == "" {
				targetGroup = msg.Content.GetFirstInputFieldValue()
			}
			if targetGroup == "" {
				targetGroup = strings.TrimSpace(strings.TrimPrefix(msg.Content.Text, "/绑定"))
			}
			if targetGroup == "" {
				_, _ = YHClient.Send(chatID, chatType, "text", "缺少参数，用法: /绑定 <QQ群号>")
				return
			}
			res := db.Bind("YH", "QQ", chatID, targetGroup)
			_, _ = YHClient.Send(chatID, chatType, "text", res.Msg)
			if res.Status == 0 && message.GlobalQQSender != nil {
				var qID int64
				fmt.Sscanf(targetGroup, "%d", &qID)
				yhGroupName := YHClient.GetGroupName(chatID)
				_, _ = message.GlobalQQSender.SendGroupMsg(qID, fmt.Sprintf("【Amer 绑定通知】本 QQ 群与云湖群「%s」(ID: %s) 绑定成功！", yhGroupName, chatID))
			}

		case "同步模式":
			syncType := msg.Content.GetFormFieldString("sync_type")
			if syncType == "" {
				syncType = msg.Content.GetFormFieldString("同步模式")
			}
			if syncType == "" {
				syncType = msg.Content.GetFormFieldByFuzzyLabel("同步模式", "模式", "sync")
			}
			targetQQGroup := msg.Content.GetFormFieldString("group_id")
			if targetQQGroup == "" {
				targetQQGroup = msg.Content.GetFormFieldString("QQ群号")
			}
			if targetQQGroup == "" {
				targetQQGroup = msg.Content.GetFormFieldString("群号")
			}
			if targetQQGroup == "" {
				targetQQGroup = msg.Content.GetFormFieldByFuzzyLabel("群号", "群", "qq")
			}
			if targetQQGroup == "" {
				targetQQGroup = msg.Content.GetFirstInputFieldValue()
			}

			if syncType == "" {
				args := strings.Fields(strings.TrimPrefix(msg.Content.Text, "/同步模式"))
				if len(args) > 0 {
					syncType = args[0]
				}
				if len(args) > 1 {
					targetQQGroup = args[1]
				}
			}

			validModes := map[string]map[string]bool{
				"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
				"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
				"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
				"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
			}

			syncData, ok := validModes[syncType]
			if !ok {
				_, _ = YHClient.Send(chatID, chatType, "text", "无效的同步模式！可用模式: 全同步, QQ到云湖, 云湖到QQ, 停止")
				return
			}

			var results []string
			if targetQQGroup != "" {
				res := db.SetSync("YH", "QQ", chatID, targetQQGroup, syncData)
				if res.Status == 0 {
					results = append(results, fmt.Sprintf("已成功将 QQ群 %s 的同步模式设置为: %s", targetQQGroup, syncType))
					if message.GlobalQQSender != nil {
						var qID int64
						fmt.Sscanf(targetQQGroup, "%d", &qID)
						yhGroupName := YHClient.GetGroupName(chatID)
						_, _ = message.GlobalQQSender.SendGroupMsg(qID, fmt.Sprintf("【Amer 同步模式通知】来自云湖群「%s」(ID: %s) 的同步模式已更改为: %s", yhGroupName, chatID, syncType))
					}
				} else {
					results = append(results, fmt.Sprintf("QQ群 %s 设置同步模式失败: %s", targetQQGroup, res.Msg))
				}
			} else {
				bindInfos := db.GetInfo("YH", chatID)
				if bindInfos.Status == 0 {
					if qqGroupIDs, ok := bindInfos.Data["QQ_group_ids"].([]db.BindingItem); ok && len(qqGroupIDs) > 0 {
						for _, q := range qqGroupIDs {
							res := db.SetSync("YH", "QQ", chatID, q.ID, syncData)
							if res.Status == 0 {
								results = append(results, fmt.Sprintf("已成功将 QQ群 %s 的同步模式设置为: %s", q.ID, syncType))
							} else {
								results = append(results, fmt.Sprintf("QQ群 %s 设置同步模式失败: %s", q.ID, res.Msg))
							}
						}
					} else {
						results = append(results, "当前群未绑定任何 QQ 群！")
					}
				} else {
					syncStatus := db.SetAllSync("YH", chatID, syncData)
					if syncStatus.Status == 0 {
						results = append(results, fmt.Sprintf("已更改所有绑定的同步模式为: %s", syncType))
					} else {
						results = append(results, fmt.Sprintf("设置同步模式失败: %s", syncStatus.Msg))
					}
				}
			}

			_, _ = YHClient.Send(chatID, chatType, "text", strings.Join(results, "\n"))
		}
	} else {
		if cmdName == "帮助" {
			_, _ = YHClient.Send(senderID, "user", "markdown", config.AppConfig.Messages.MessageYHFollowed)
		} else {
			_, _ = YHClient.Send(senderID, "user", "text", "请在群内使用指令,您目前可且仅可以使用/帮助命令")
		}
	}
}

func handleBotFollowed(event model.YunhuEvent) {
	userID := event.Event.UserID.String()
	if userID == "" {
		userID = event.Event.Sender.SenderID.String()
	}
	_, _ = YHClient.Send(userID, "user", "markdown", config.AppConfig.Messages.MessageYHFollowed)
	log.Printf("[Yunhu] 用户 %s 关注了机器人", event.Event.Sender.SenderNickname)
}

func handleA2UIButtonEvent(event model.YunhuEvent) {
	actionName := event.Event.ActionName.String()
	if actionName == "" {
		actionName = event.Event.Value.String()
	}

	userID := event.Event.UserID.String()
	if userID == "" {
		userID = event.Event.Sender.SenderID.String()
	}
	userName := event.Event.UserName.String()
	if userName == "" {
		userName = event.Event.Sender.SenderNickname.String()
	}

	log.Printf("[Yunhu Button Event] actionName: %s, userID: %s, userName: %s", actionName, userID, userName)

	// 1. Handle submit_report (from A2UI report modal submission)
	if actionName == "submit_report" {
		fc := event.Event.FormContext
		msgID := ""
		reason := ""
		senderID := ""
		groupID := ""

		if fc != nil {
			if v, ok := fc["msg_id"].(string); ok {
				msgID = v
			}
			if v, ok := fc["reason"].(string); ok {
				reason = v
			}
			if v, ok := fc["sender_id"].(string); ok {
				senderID = v
			}
			if v, ok := fc["group_id"].(string); ok {
				groupID = v
			}
		}

		cbMsgID := event.Event.GetMsgID()
		cbChatID := event.Event.GetChatID()
		cbChatType := event.Event.GetChatType()

		targetMsgID := msgID
		if targetMsgID == "" {
			targetMsgID = cbMsgID
		}

		realYHMsgID := cbMsgID
		if targetMsgID != "" && cbMsgID != "" {
			db.SaveYunhuMsgCache(targetMsgID, cbChatID, cbChatType, senderID, cbMsgID)
			if qqMsgIDNum, err := strconv.ParseInt(targetMsgID, 10, 64); err == nil && qqMsgIDNum != 0 {
				db.BindYunhuMsgToQQMsg(cbMsgID, qqMsgIDNum)
			}
		}

		// --- 举报冷却控制 (1分钟只能举报1次) ---
		// 规则：第二次收到点击举报消息发送"有冷却时间"提示，如果再收到多次点击则不发送任何消息提示
		cooldownKey := fmt.Sprintf("cooldown:%s", userID)
		now := time.Now()
		val, exists := reportCooldownMap.Load(cooldownKey)
		if exists {
			item := val.(reportCooldownItem)
			if now.Sub(item.FirstReportTime) < time.Minute {
				item.AttemptCount++
				reportCooldownMap.Store(cooldownKey, item)
				if item.AttemptCount == 2 {
					// 第二次点击：提醒冷却时间
					if cbChatID != "" {
						_, _ = YHClient.Send(cbChatID, cbChatType, "text", "⚠️ 举报过于频繁，有冷却时间（1分钟内只能提交1次），请稍后再试！")
					}
				}
				// 第3次及以上多次点击：静默丢弃，不发送消息
				log.Printf("[Yunhu Report RateLimit] 用户 %s 处在举报冷却中 (第 %d 次尝试)，直接忽略", userID, item.AttemptCount)
				return
			}
		}

		// 记录新的举报时间点
		reportCooldownMap.Store(cooldownKey, reportCooldownItem{
			FirstReportTime: now,
			AttemptCount:    1,
		})

		req := ReportData{
			Platform:     "QQ",
			GroupID:      groupID,
			MsgID:        targetMsgID,
			YunhuMsgID:   realYHMsgID,
			ChatID:       cbChatID,
			ChatType:     cbChatType,
			SenderID:     senderID,
			ReporterID:   userID,
			ReporterName: userName,
			Reason:       reason,
		}

		success, msgStr := ProcessReportData(req)
		log.Printf("[Yunhu Report Submit] 处理状态: %v, 结果: %s", success, msgStr)

		// Reply feedback message to user who clicked submit report button
		if cbChatID != "" {
			_, _ = YHClient.Send(cbChatID, cbChatType, "text", "✅ 举报已成功提交！已自动推送至管理员进行审核处理。")
		}
		return
	}

	// 2. Handle report_action:approve:reportID / report_action:reject:reportID / report_action:recall:reportID
	if strings.HasPrefix(actionName, "report_action:") {
		parts := strings.Split(actionName, ":")
		if len(parts) >= 3 {
			actionType := parts[1] // "approve", "reject", "recall"
			reportID := parts[2]

			reportRec, err := db.GetReportRecord(reportID)
			if err != nil || reportRec == nil {
				_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("❌ 未找到 ID 为 %s 的举报记录或已被清理！", reportID))
				return
			}

			if reportRec.Status != "PENDING" {
				// --- 管理员重复点击防护 ---
				// 处理完的消息：第二次收到点击提示“已处理”，如果以后再收到对应的多次点击则不发送消息提示
				clickKey := fmt.Sprintf("admin_click:%s:%s", reportID, actionType)
				val, exists := adminActionClickMap.Load(clickKey)
				count := 1
				if exists {
					item := val.(adminClickItem)
					count = item.ClickCount + 1
				}
				adminActionClickMap.Store(clickKey, adminClickItem{
					ClickCount: count,
					LastClick:  time.Now(),
				})

				if count == 1 {
					statusText := "已处理"
					switch reportRec.Status {
					case "APPROVED":
						statusText = "已同意 (已被封禁该用户)"
					case "REJECTED":
						statusText = "已拒绝 (已忽略该举报)"
					case "RECALLED":
						statusText = "已撤回违规消息"
					}
					_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("⚠️ 该举报 (ID: %s) 此前已被处理为【%s】，请勿重复操作！", reportID, statusText))
				} else {
					// 多次重复点击：静默忽略，不再发送消息
					log.Printf("[Report Admin RateLimit] 管理员 %s 重复点击已处理的举报 %s (第 %d 次)，静默忽略", userID, reportID, count)
				}
				return
			}

			if actionType == "approve" {
				reportRec.Status = "APPROVED"
				_ = db.SaveReportRecord(reportRec)

				_ = db.AddToBlacklist(reportRec.SenderID, fmt.Sprintf("管理员违规处理封禁 (%s)", reportRec.Reason), 3600)
				log.Printf("[Report Admin] 管理员 %s (%s) 批准了举报 %s，用户 %s 已拉黑 1 小时", userName, userID, reportID, reportRec.SenderID)

				targetChatID := reportRec.ChatID
				targetChatType := reportRec.ChatType
				if targetChatType == "" {
					targetChatType = "group"
				}
				if bindInfo := db.GetInfo("QQ", targetChatID); bindInfo.Status == 0 {
					if yhGroupIDs, ok := bindInfo.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
						targetChatID = yhGroupIDs[0].ID
					}
				}

				recallYHMsgID := reportRec.YunhuMsgID
				if recallYHMsgID == "" {
					if realID, _, _, _, found := db.GetYunhuMsgCache(reportRec.MsgID); found && realID != "" {
						recallYHMsgID = realID
					}
				}
				if recallYHMsgID == "" {
					if qqID, err := strconv.ParseInt(reportRec.MsgID, 10, 64); err == nil && qqID != 0 {
						if realID, ok := db.GetYunhuMsgIDByQQMsgID(qqID); ok && realID != "" {
							recallYHMsgID = realID
						}
					}
				}
				if recallYHMsgID == "" && len(reportRec.MsgID) >= 20 {
					recallYHMsgID = reportRec.MsgID
				}

				// Automatically attempt recall as well when approving
				if recallYHMsgID != "" && targetChatID != "" {
					_ = YHClient.Recall(recallYHMsgID, targetChatID, targetChatType)
				}

				_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("✅ 操作成功：已同意举报 [%s]！被举报用户 (%s) 已加入黑名单封禁 1 小时，相关违规消息已尝试撤回。", reportID, reportRec.SenderID))
			} else if actionType == "reject" {
				reportRec.Status = "REJECTED"
				_ = db.SaveReportRecord(reportRec)

				log.Printf("[Report Admin] 管理员 %s (%s) 拒绝了举报 %s", userName, userID, reportID)
				_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("❌ 操作成功：已拒绝举报 [%s]，针对用户 (%s) 的举报已被您忽略。", reportID, reportRec.SenderID))
			} else if actionType == "recall" {
				reportRec.Status = "RECALLED"
				_ = db.SaveReportRecord(reportRec)

				targetChatID := reportRec.ChatID
				targetChatType := reportRec.ChatType
				if targetChatType == "" {
					targetChatType = "group"
				}

				// If targetChatID is a QQ group ID, resolve real Yunhu Group ID
				if bindInfo := db.GetInfo("QQ", targetChatID); bindInfo.Status == 0 {
					if yhGroupIDs, ok := bindInfo.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
						targetChatID = yhGroupIDs[0].ID
					}
				}

				recallYHMsgID := reportRec.YunhuMsgID
				if recallYHMsgID == "" {
					if realID, _, _, _, found := db.GetYunhuMsgCache(reportRec.MsgID); found && realID != "" {
						recallYHMsgID = realID
					}
				}
				if recallYHMsgID == "" {
					if qqID, err := strconv.ParseInt(reportRec.MsgID, 10, 64); err == nil && qqID != 0 {
						if realID, ok := db.GetYunhuMsgIDByQQMsgID(qqID); ok && realID != "" {
							recallYHMsgID = realID
						}
					}
				}
				if recallYHMsgID == "" && len(reportRec.MsgID) >= 20 {
					recallYHMsgID = reportRec.MsgID
				}

				if recallYHMsgID != "" && targetChatID != "" {
					err := YHClient.Recall(recallYHMsgID, targetChatID, targetChatType)
					if err == nil {
						log.Printf("[Report Admin] 管理员 %s (%s) 成功撤回举报 %s 的消息 %s (Yunhu Group: %s)", userName, userID, reportID, recallYHMsgID, targetChatID)
						_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("🗑️ 操作成功：已为举报 [%s] 成功撤回原消息 (MsgID: %s, 群ID: %s)！", reportID, recallYHMsgID, targetChatID))
					} else {
						log.Printf("[Report Admin Fail] 撤回消息失败: %v", err)
						_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("❌ 撤回消息失败: %v", err))
					}
				} else {
					_, _ = YHClient.Send(userID, "user", "text", fmt.Sprintf("⚠️ 缺失原消息对象信息 (ChatID: %s, MsgID: %s)，无法直接撤回", reportRec.ChatID, reportRec.MsgID))
				}
			}
		}
		return
	}
}
