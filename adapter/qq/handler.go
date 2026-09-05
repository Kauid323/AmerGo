package qq

import (
	"amer/adapter/message"
	"amer/db"
	"amer/model"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"
)

func HandleOneBotEvent(event model.OneBotEvent) {
	postType := strings.TrimSpace(event.PostType)
	switch postType {
	case "message", "message_sent":
		handleQQMessage(event)
	case "request":
		handleQQRequest(event)
	case "notice":
		handleQQNotice(event)
	case "meta_event":
		// Heartbeat / lifecycle
	default:
		if postType != "" {
			log.Printf("[QQ Event] 未知事件类型: %s", postType)
		}
	}
}

func handleQQMessage(event model.OneBotEvent) {
	senderUserID := event.GetSenderUserID()
	botSelfID, _ := GlobalOneBotServer.GetSelfInfo()

	// 过滤机器人自身发送的消息，防止回环同步
	if senderUserID != 0 && (senderUserID == botSelfID || senderUserID == event.SelfID.Int64()) {
		log.Printf("[QQ Filter] 过滤机器人账号 (%d) 自身发送的消息", senderUserID)
		return
	}
	if strings.TrimSpace(event.PostType) == "message_sent" {
		log.Printf("[QQ Filter] 过滤 message_sent 事件")
		return
	}

	senderUserIDStr := strconv.FormatInt(senderUserID, 10)
	rawMsg := strings.TrimSpace(event.GetRawMessage())

	// Check for admin commands first
	if handled, replyMsg := message.HandleAdminCommand("QQ", senderUserIDStr, rawMsg, event.Sender.Role); handled {
		if event.MessageType == "private" {
			_ = GlobalOneBotServer.SendPrivateMsg(senderUserID, replyMsg)
		} else {
			_ = GlobalOneBotServer.SendGroupMsg(event.GroupID.Int64(), replyMsg)
		}
		return
	}

	if event.MessageType == "private" {
		log.Printf("[QQ Private Msg] 来自 %d (%s): %s", senderUserID, event.Sender.Nickname, rawMsg)
		if strings.Contains(rawMsg, "qun.invite") || strings.Contains(rawMsg, "group/invite_join") || strings.Contains(rawMsg, "邀请你加入群聊") {
			log.Printf("[QQ Private Msg] 收到群邀请卡片私聊，自动解析凭据确认入群")
			tryAcceptInviteCard(rawMsg, senderUserID)
			return
		}
		_ = GlobalOneBotServer.SendPrivateMsg(senderUserID, "你好！Amer 已更新为纯互通版（AI 功能已关闭）。在群内将消息同步至云湖吧！")
		return
	}

	if event.MessageType == "group" {
		groupID := event.GroupID.Int64()
		groupIDStr := strconv.FormatInt(groupID, 10)

		log.Printf("[QQ Group Msg] 群 %d 来自 %s(%d): %s", groupID, event.Sender.Nickname, senderUserID, rawMsg)

		// Command handling
		if strings.HasPrefix(rawMsg, "/") {
			cmd := strings.TrimPrefix(rawMsg, "/")
			if handleQQCommand(event, groupIDStr, senderUserIDStr, cmd) {
				return
			}
		}

		groupName := GlobalOneBotServer.GetGroupName(groupID)
		cleanedName := message.ReplaceBlockedWords(event.Sender.Nickname)
		cleanedMsg := message.ReplaceBlockedWords(rawMsg)
		htmlMsg := message.CQToHTML(cleanedMsg)
		cleanedMsgHTML := strings.ReplaceAll(htmlMsg, "\n", "<br>")

		avatarURL := fmt.Sprintf("http://q1.qlogo.cn/g?b=qq&nk=%d&s=100", senderUserID)

		contentHTML := fmt.Sprintf(
			`<div style="display: flex; align-items: flex-start; margin-bottom: 10px;"><img src="%s" alt="用户头像" style="width: 36px; height: 36px; border-radius: 50%%; margin-right: 10px;"><div style="flex: 1;"><strong style="font-size: 14px; color: #333;">%s</strong><p style="font-size: 8px; color: #6c757d; margin-top: 2px;"><strong>用户ID: </strong>%s</p></div></div><div style="background-color: #f9f9f9; padding: 5px; border-radius: 5px;"><p style="color: #000000;">%s</p></div><div style="font-family: Arial, sans-serif; line-height: 1.4; font-size: 12px; color: #888;"><details style="margin-top: 5px;"><summary style="cursor: pointer; color: #007bff; font-size: 12px;">详情</summary><p style="margin: 3px 0;">群聊: %s</p><p style="margin: 3px 0;">ID: %s</p><p style="margin: 3px 0;">发送时间: %s</p></details></div>`,
			avatarURL, cleanedName, senderUserIDStr, cleanedMsgHTML, groupName, groupIDStr, time.Now().Format("2006-01-02 15:04:05"),
		)

		if strings.Contains(rawMsg, "@全体成员") {
			message.SetBoardForAllGroups("QQ", groupIDStr, strings.ReplaceAll(rawMsg, "@全体成员", ""), groupName)
		} else if strings.Contains(rawMsg, "[CQ:video") {
			videoURL := message.ExtractVideoURL(rawMsg)
			if videoURL != "" {
				surfaceID := fmt.Sprintf("video_%s_%d", event.MessageID.String(), time.Now().UnixNano())
				a2uiJSON := message.BuildVideoA2UI(surfaceID, groupName, groupIDStr, cleanedName, senderUserIDStr, videoURL, event.MessageID.String())
				message.SendA2UIToAllBindings("QQ", groupIDStr, cleanedMsg, senderUserIDStr, cleanedName, a2uiJSON, event.MessageID.String())
			} else {
				message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, contentHTML, event.MessageID.String())
			}
		} else if strings.Contains(rawMsg, "[CQ:record") {
			audioURL, fileVal := message.ExtractAudioURL(rawMsg)
			if audioURL != "" || fileVal != "" {
				audioPlayURI := message.FetchAudioDataURI(audioURL, fileVal)
				surfaceID := fmt.Sprintf("audio_%s_%d", event.MessageID.String(), time.Now().UnixNano())
				a2uiJSON := message.BuildAudioA2UI(surfaceID, groupName, groupIDStr, cleanedName, senderUserIDStr, audioPlayURI, event.MessageID.String())
				message.SendA2UIToAllBindings("QQ", groupIDStr, cleanedMsg, senderUserIDStr, cleanedName, a2uiJSON, event.MessageID.String())
			} else {
				message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, contentHTML, event.MessageID.String())
			}
		} else if strings.Contains(rawMsg, "[CQ:forward") {
			forwardID := message.ExtractForwardID(rawMsg)
			if forwardID != "" {
				forwardHTML := FetchForwardMsgHTML(forwardID, groupName, cleanedName, senderUserIDStr)
				if forwardHTML != "" {
					message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, forwardHTML, event.MessageID.String())
				} else {
					fallbackHTML := BuildForwardFallbackHTML(groupName, cleanedName, senderUserIDStr)
					message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, fallbackHTML, event.MessageID.String())
				}
			} else {
				fallbackHTML := BuildForwardFallbackHTML(groupName, cleanedName, senderUserIDStr)
				message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, fallbackHTML, event.MessageID.String())
			}
		} else {
			message.SendToAllBindings("QQ", groupIDStr, "html", cleanedMsg, senderUserIDStr, cleanedName, contentHTML, event.MessageID.String())
		}
	}
}

type ForwardMessageNode struct {
	Sender struct {
		Nickname string              `json:"nickname"`
		UserID   model.FlexibleInt64 `json:"user_id"`
	} `json:"sender"`
	Time    int64       `json:"time"`
	Message interface{} `json:"message"`
	Content interface{} `json:"content"`
}

type ForwardMsgData struct {
	Messages []ForwardMessageNode `json:"messages"`
}

func parseNodeContent(content interface{}) string {
	if content == nil {
		return ""
	}
	if contentStr, ok := content.(string); ok {
		return message.CQToHTML(contentStr)
	}
	if contentArr, ok := content.([]interface{}); ok {
		var sb strings.Builder
		for _, elem := range contentArr {
			if elemMap, ok := elem.(map[string]interface{}); ok {
				elemType, _ := elemMap["type"].(string)
				dataMap, _ := elemMap["data"].(map[string]interface{})
				switch elemType {
				case "text":
					if text, ok := dataMap["text"].(string); ok {
						sb.WriteString(html.EscapeString(text))
					}
				case "image":
					imgURL, _ := dataMap["url"].(string)
					if imgURL == "" {
						imgURL, _ = dataMap["file"].(string)
					}
					if imgURL != "" {
						sb.WriteString(fmt.Sprintf(`<br><img src="%s" style="max-width: 100%%; margin: 5px 0; border-radius: 4px;"><br>`, imgURL))
					} else {
						sb.WriteString("[图片]")
					}
				case "at":
					qqVal, _ := dataMap["qq"].(string)
					sb.WriteString(fmt.Sprintf("<b>@%s</b> ", qqVal))
				case "video":
					videoURL, _ := dataMap["url"].(string)
					if videoURL == "" {
						videoURL, _ = dataMap["file"].(string)
					}
					if videoURL != "" {
						sb.WriteString(fmt.Sprintf(`<br><video src="%s" controls style="max-width: 100%%; margin: 5px 0;"></video><br>`, videoURL))
					} else {
						sb.WriteString("[视频消息]")
					}
				default:
					sb.WriteString(fmt.Sprintf("[%s消息]", elemType))
				}
			}
		}
		return sb.String()
	}
	return ""
}

func FetchForwardMsgHTML(forwardID, groupName, senderName, senderIDStr string) string {
	respChan := make(chan model.OneBotResponse, 1)
	go func() {
		resp, err := GlobalOneBotServer.CallAPI("get_forward_msg", map[string]interface{}{
			"message_id": forwardID,
			"id":         forwardID,
		})
		if err == nil {
			respChan <- resp
		}
	}()

	select {
	case resp := <-respChan:
		if resp.Status == "ok" {
			var forwardData ForwardMsgData
			if err := json.Unmarshal(resp.Data, &forwardData); err == nil && len(forwardData.Messages) > 0 {
				avatarURL := fmt.Sprintf("http://q1.qlogo.cn/g?b=qq&nk=%s&s=100", senderIDStr)
				var nodesSB strings.Builder
				for _, node := range forwardData.Messages {
					nodeSender := node.Sender.Nickname
					if nodeSender == "" {
						nodeSender = "匿名"
					}
					timeStr := time.Unix(node.Time, 0).Format("01-02 15:04")
					if node.Time <= 0 {
						timeStr = "未知时间"
					}
					nodeContent := node.Message
					if nodeContent == nil {
						nodeContent = node.Content
					}
					nodeContentHTML := parseNodeContent(nodeContent)

					nodesSB.WriteString(fmt.Sprintf(
						`<div style="background-color: #ffffff; padding: 6px 8px; border-radius: 4px; margin-bottom: 5px; border: 1px solid #e9ecef;"><span style="color: #007bff; font-weight: bold;">%s (%s):</span> <span style="color: #212529;">%s</span></div>`,
						html.EscapeString(nodeSender), timeStr, nodeContentHTML,
					))
				}

				return fmt.Sprintf(
					`<div style="display: flex; align-items: flex-start; margin-bottom: 10px;"><img src="%s" alt="用户头像" style="width: 36px; height: 36px; border-radius: 50%%; margin-right: 10px;"><div style="flex: 1;"><strong style="font-size: 14px; color: #333;">%s</strong><p style="font-size: 8px; color: #6c757d; margin-top: 2px;"><strong>用户ID: </strong>%s</p></div></div><div style="background-color: #f9f9f9; padding: 10px; border-radius: 5px; border: 1px solid #ddd;"><div style="font-weight: bold; margin-bottom: 8px; color: #333;">📨 合并转发消息 (共 %d 条)</div>%s</div><div style="font-family: Arial, sans-serif; line-height: 1.4; font-size: 12px; color: #888;"><details style="margin-top: 5px;"><summary style="cursor: pointer; color: #007bff; font-size: 12px;">详情</summary><p style="margin: 3px 0;">群聊: %s</p><p style="margin: 3px 0;">ID: %s</p><p style="margin: 3px 0;">发送时间: %s</p></details></div>`,
					avatarURL, senderName, senderIDStr, len(forwardData.Messages), nodesSB.String(), groupName, groupName, time.Now().Format("2006-01-02 15:04:05"),
				)
			}
		}
	case <-time.After(1500 * time.Millisecond):
		log.Printf("[OneBot Forward] 客户端无 get_forward_msg 响应 (ID: %s)，快速切换卡片展示", forwardID)
	}

	return ""
}

func BuildForwardFallbackHTML(groupName, senderName, senderIDStr string) string {
	avatarURL := fmt.Sprintf("http://q1.qlogo.cn/g?b=qq&nk=%s&s=100", senderIDStr)
	return fmt.Sprintf(
		`<div style="display: flex; align-items: flex-start; margin-bottom: 10px;"><img src="%s" alt="用户头像" style="width: 36px; height: 36px; border-radius: 50%%; margin-right: 10px;"><div style="flex: 1;"><strong style="font-size: 14px; color: #333;">%s</strong><p style="font-size: 8px; color: #6c757d; margin-top: 2px;"><strong>用户ID: </strong>%s</p></div></div><div style="background-color: #f9f9f9; padding: 10px; border-radius: 5px; border: 1px solid #ddd;"><div style="font-weight: bold; margin-bottom: 5px; color: #333;">📦 [QQ 合并转发消息]</div><div style="color: #666; font-size: 12px;">转发聊天记录</div></div><div style="font-family: Arial, sans-serif; line-height: 1.4; font-size: 12px; color: #888;"><details style="margin-top: 5px;"><summary style="cursor: pointer; color: #007bff; font-size: 12px;">详情</summary><p style="margin: 3px 0;">群聊: %s</p><p style="margin: 3px 0;">ID: %s</p><p style="margin: 3px 0;">发送时间: %s</p></details></div>`,
		avatarURL, senderName, senderIDStr, groupName, groupName, time.Now().Format("2006-01-02 15:04:05"),
	)
}

func handleQQCommand(event model.OneBotEvent, groupIDStr, userIDStr, cmd string) bool {
	groupID := event.GroupID.Int64()
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	mainCmd := parts[0]

	switch mainCmd {
	case "帮助":
		helpMsg := "📌 Amer 指令指南 📌\n\n1. /帮助 - 查看当前帮助信息\n2. /绑定列表 - 查看当前群聊绑定的云湖群\n3. /绑定 yh <云湖群ID> - 将当前QQ群与指定云湖群绑定\n4. /解绑 yh <云湖群ID> - 解除指定云湖群绑定\n5. /解绑 全部 - 解除当前群的所有绑定\n6. /同步模式 <全同步/QQ到云湖/云湖到QQ/停止> [云湖群ID] - 设置同步模式"
		_ = GlobalOneBotServer.SendGroupMsg(groupID, helpMsg)
		return true

	case "绑定列表":
		bindInfos := db.GetInfo("QQ", groupIDStr)
		if bindInfos.Status == 0 {
			menu := fmt.Sprintf("QQ群: %s\n\n", GlobalOneBotServer.GetGroupName(groupID))
			if yhGroupIDs, ok := bindInfos.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
				menu += "云湖群:\n"
				for idx, y := range yhGroupIDs {
					syncMode := "未设置"
					if y.Sync && y.BindingSync {
						syncMode = "互通"
					} else if y.Sync && !y.BindingSync {
						syncMode = "单向-QQ到云湖"
					} else if !y.Sync && y.BindingSync {
						syncMode = "单向-云湖到QQ"
					}
					menu += fmt.Sprintf("%d. 群号:%s (%s)\n", idx+1, y.ID, syncMode)
				}
			} else {
				menu += "当前QQ群未绑定任何云湖群。\n"
			}
			_ = GlobalOneBotServer.SendGroupMsg(groupID, menu)
		} else {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, bindInfos.Msg)
		}
		return true

	case "绑定":
		if len(parts) < 3 {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, "指令格式错误，请使用：/绑定 yh <云湖群ID>")
			return true
		}
		platform := strings.ToLower(parts[1])
		targetID := parts[2]

		if platform == "yh" || platform == "云湖" {
			bindStatus := db.Bind("QQ", "YH", groupIDStr, targetID)
			if bindStatus.Status == 0 {
				qqGroupName := GlobalOneBotServer.GetGroupName(groupID)
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("云湖群 %s 已成功绑定！", targetID))
				if message.GlobalYHSender != nil {
					_, _ = message.GlobalYHSender.Send(targetID, "group", "text", fmt.Sprintf("【Amer 绑定通知】与 QQ群「%s」(ID: %s) 绑定成功！", qqGroupName, groupIDStr))
				}
			} else {
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("绑定失败: %s", bindStatus.Msg))
			}
		} else {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, "不支持的平台！仅支持：yh（云湖群）")
		}
		return true

	case "解绑":
		if len(parts) < 2 {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, "指令格式错误，请使用：/解绑 yh <云湖群ID> 或 /解绑 全部")
			return true
		}
		target := strings.ToLower(parts[1])

		if target == "全部" || target == "all" {
			qqGroupName := GlobalOneBotServer.GetGroupName(groupID)
			bindInfos := db.GetInfo("QQ", groupIDStr)
			if bindInfos.Status == 0 {
				if yhGroupIDs, ok := bindInfos.Data["YH_group_ids"].([]db.BindingItem); ok {
					for _, y := range yhGroupIDs {
						if message.GlobalYHSender != nil {
							_, _ = message.GlobalYHSender.Send(y.ID, "group", "text", fmt.Sprintf("【Amer 解绑通知】与 QQ群「%s」(ID: %s) 的绑定已解除！", qqGroupName, groupIDStr))
						}
					}
				}
			}
			unbindStatus := db.UnbindAll("QQ", groupIDStr)
			_ = GlobalOneBotServer.SendGroupMsg(groupID, unbindStatus.Msg)
			return true
		}

		if len(parts) >= 3 && (target == "yh" || target == "云湖") {
			yhID := parts[2]
			unbindStatus := db.Unbind("QQ", "YH", groupIDStr, yhID)
			if unbindStatus.Status == 0 {
				qqGroupName := GlobalOneBotServer.GetGroupName(groupID)
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("云湖群 %s 已成功解绑！", yhID))
				if message.GlobalYHSender != nil {
					_, _ = message.GlobalYHSender.Send(yhID, "group", "text", fmt.Sprintf("【Amer 解绑通知】与 QQ群「%s」(ID: %s) 的绑定已解除！", qqGroupName, groupIDStr))
				}
			} else {
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("解绑失败: %s", unbindStatus.Msg))
			}
			return true
		}

		_ = GlobalOneBotServer.SendGroupMsg(groupID, "指令格式错误，请使用：/解绑 yh <云湖群ID> 或 /解绑 全部")
		return true

	case "同步模式":
		if len(parts) < 2 {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, "指令格式错误，用法：/同步模式 <全同步/QQ到云湖/云湖到QQ/停止> [云湖群ID]")
			return true
		}
		syncType := parts[1]
		targetYHGroup := ""
		if len(parts) >= 3 {
			targetYHGroup = parts[2]
		}

		validModes := map[string]map[string]bool{
			"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
			"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
			"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
			"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
		}

		syncData, ok := validModes[syncType]
		if !ok {
			_ = GlobalOneBotServer.SendGroupMsg(groupID, "无效的同步模式！可用模式: 全同步, QQ到云湖, 云湖到QQ, 停止")
			return true
		}

		qqGroupName := GlobalOneBotServer.GetGroupName(groupID)
		if targetYHGroup != "" {
			res := db.SetSync("QQ", "YH", groupIDStr, targetYHGroup, syncData)
			if res.Status == 0 {
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("已成功将云湖群 %s 的同步模式设置为: %s", targetYHGroup, syncType))
				if message.GlobalYHSender != nil {
					_, _ = message.GlobalYHSender.Send(targetYHGroup, "group", "text", fmt.Sprintf("【Amer 同步模式通知】来自 QQ群「%s」(ID: %s) 的同步模式已更改为: %s", qqGroupName, groupIDStr, syncType))
				}
			} else {
				_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("设置同步模式失败: %s", res.Msg))
			}
		} else {
			bindInfos := db.GetInfo("QQ", groupIDStr)
			if bindInfos.Status == 0 {
				if yhGroupIDs, ok := bindInfos.Data["YH_group_ids"].([]db.BindingItem); ok && len(yhGroupIDs) > 0 {
					for _, y := range yhGroupIDs {
						res := db.SetSync("QQ", "YH", groupIDStr, y.ID, syncData)
						if res.Status == 0 {
							_ = GlobalOneBotServer.SendGroupMsg(groupID, fmt.Sprintf("已成功将云湖群 %s 的同步模式设置为: %s", y.ID, syncType))
							if message.GlobalYHSender != nil {
								_, _ = message.GlobalYHSender.Send(y.ID, "group", "text", fmt.Sprintf("【Amer 同步模式通知】来自 QQ群「%s」(ID: %s) 的同步模式已更改为: %s", qqGroupName, groupIDStr, syncType))
							}
						}
					}
				} else {
					_ = GlobalOneBotServer.SendGroupMsg(groupID, "当前 QQ 群未绑定任何云湖群！")
				}
			}
		}
		return true
	}

	return false
}

func handleQQRequest(event model.OneBotEvent) {
	reqType := event.GetRequestType()
	log.Printf("[QQ Request] 收到请求: 类型=%s, 子类型=%s, 群号=%d, 发起人/用户=%d, Flag=%s",
		reqType, event.SubType, event.GroupID.Int64(), event.UserID.Int64(), event.Flag)

	if reqType == "friend" {
		resp, err := GlobalOneBotServer.CallAPI("set_friend_add_request", map[string]interface{}{
			"flag":    event.Flag,
			"approve": true,
		})
		if err != nil || resp.Status == "failed" {
			log.Printf("[QQ Request Error] 自动同意好友申请失败: %v, resp: %+v", err, resp)
		} else {
			log.Printf("[QQ Request Success] 已自动同意来自用户 %d 的好友申请", event.UserID.Int64())
		}
	} else if reqType == "group" {
		subType := event.SubType
		if subType == "" {
			subType = "invite"
		}

		go func(ev model.OneBotEvent, sType string) {
			if sType == "invite" {
				time.Sleep(1000 * time.Millisecond)
			}

			// 适配多个候选 flag: 原生 flag、eventType: 2 (自己确认入群，避开 eventType: 7 管理员审批) 与 legacy 格式
			flags := []string{ev.Flag}
			if strings.Contains(ev.Flag, ":7:") {
				flags = append(flags, strings.Replace(ev.Flag, ":7:", ":2:", 1))
			}
			flags = append(flags, fmt.Sprintf("invite:%d:%d", ev.GroupID.Int64(), ev.UserID.Int64()))

			for attempt := 1; attempt <= 3; attempt++ {
				for _, flagCandidate := range flags {
					resp, err := GlobalOneBotServer.CallAPI("set_group_add_request", map[string]interface{}{
						"flag":     flagCandidate,
						"sub_type": sType,
						"type":     sType,
						"approve":  true,
					})
					if err == nil && resp.Status == "ok" {
						if sType == "invite" {
							log.Printf("[QQ Request Success] 已自动同意群邀请！成功加入群: %d (邀请人: %d, Flag: %s)", ev.GroupID.Int64(), ev.UserID.Int64(), flagCandidate)
						} else {
							log.Printf("[QQ Request Success] 已自动同意加群请求！群号: %d (申请人: %d)", ev.GroupID.Int64(), ev.UserID.Int64())
						}
						return
					}
				}

				log.Printf("[QQ Request Warning] 第 %d 次处理群请求未成功 (群: %d, sub_type: %s)，准备重试...", attempt, ev.GroupID.Int64(), sType)
				if attempt < 3 {
					time.Sleep(1500 * time.Millisecond)
				}
			}
			log.Printf("[QQ Request Error] 处理群邀请/加群最终失败 (群: %d, Flag: %s)", ev.GroupID.Int64(), ev.Flag)
		}(event, subType)
	}
}

func tryAcceptInviteCard(rawMsg string, senderUserID int64) {
	if !strings.Contains(rawMsg, "group/invite_join") {
		return
	}
	idx := strings.Index(rawMsg, "groupcode=")
	if idx == -1 {
		return
	}
	sub := rawMsg[idx:]
	var groupCode, msgSeq string
	for _, part := range strings.Split(sub, "&") {
		part = strings.ReplaceAll(part, "\\", "")
		part = strings.ReplaceAll(part, "\"", "")
		part = strings.ReplaceAll(part, "}", "")
		part = strings.ReplaceAll(part, "]", "")
		kv := strings.Split(part, "=")
		if len(kv) >= 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k == "groupcode" {
				groupCode = v
			} else if k == "msgseq" {
				msgSeq = v
			}
		}
	}

	if groupCode != "" {
		log.Printf("[QQ Invite Card] 解析到群邀请链接卡片: 群号=%s, msgseq=%s", groupCode, msgSeq)
		go func() {
			time.Sleep(500 * time.Millisecond)
			if msgSeq != "" {
				flagCanonical := fmt.Sprintf("slreq:1:%s:%s:2:0", msgSeq, groupCode)
				resp, err := GlobalOneBotServer.CallAPI("set_group_add_request", map[string]interface{}{
					"flag":     flagCanonical,
					"sub_type": "invite",
					"type":     "invite",
					"approve":  true,
				})
				if err == nil && resp.Status == "ok" {
					log.Printf("[QQ Request Success] 通过邀请卡片 msgseq 成功确认加入群聊 %s！", groupCode)
					return
				}
			}
			flagLegacy := fmt.Sprintf("invite:%s:%d", groupCode, senderUserID)
			resp, err := GlobalOneBotServer.CallAPI("set_group_add_request", map[string]interface{}{
				"flag":     flagLegacy,
				"sub_type": "invite",
				"type":     "invite",
				"approve":  true,
			})
			if err == nil && resp.Status == "ok" {
				log.Printf("[QQ Request Success] 通过 legacy invite flag 成功确认加入群聊 %s！", groupCode)
				return
			}
		}()
	}
}

func handleQQNotice(event model.OneBotEvent) {
	noticeType := event.GetNoticeType()
	switch noticeType {
	case "group_increase":
		log.Printf("[QQ Notice] 群成员增加 (群: %d, 用户: %d, 操作者: %d, 子类型: %s)",
			event.GroupID.Int64(), event.UserID.Int64(), event.OperatorID.Int64(), event.SubType)
	case "group_decrease":
		log.Printf("[QQ Notice] 群成员减少 (群: %d, 用户: %d, 操作者: %d, 子类型: %s)",
			event.GroupID.Int64(), event.UserID.Int64(), event.OperatorID.Int64(), event.SubType)
	default:
		log.Printf("[QQ Notice] 收到通知事件: %s (子类型: %s)", noticeType, event.SubType)
	}
}
