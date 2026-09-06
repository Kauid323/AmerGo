package message

import (
	"amer/db"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ReplyMsgInfo struct {
	SenderName string
	SenderUID  int64
	RawText    string
	Summary    string
}

type QQMessageSender interface {
	SendGroupMsg(groupID int64, message string) (int64, error)
	SendGroupForwardMsg(groupID int64, nodes []interface{}) error
	GetGroupName(groupID int64) string
	GetSelfInfo() (int64, string)
	GetGroupMemberName(groupID int64, userID int64) string
	GetReplyMsg(messageID int64, groupID int64) (*ReplyMsgInfo, error)
}

type YunhuMessageSender interface {
	Send(recvID, recvType, contentType, content string) (string, error)
	SetBoard(recvID, recvType, content string) error
	GetGroupName(groupID string) string
	UploadImage(imgData []byte, filename string) (string, error)
}

var (
	GlobalQQSender QQMessageSender
	GlobalYHSender YunhuMessageSender
)

func RegisterQQSender(sender QQMessageSender) {
	GlobalQQSender = sender
}

func RegisterYHSender(sender YunhuMessageSender) {
	GlobalYHSender = sender
}

func SendForwardVideoToQQ(qqGroupID int64, senderNickname, senderID, yhGroupID, videoPath string) error {
	if GlobalQQSender == nil {
		return fmt.Errorf("QQ 发送器未注册 (OneBot WS 未连接)")
	}

	yhGroupName := yhGroupID
	if GlobalYHSender != nil {
		if name := GlobalYHSender.GetGroupName(yhGroupID); name != "" {
			yhGroupName = name
		}
	}

	botUin, botName := GlobalQQSender.GetSelfInfo()
	botUinStr := fmt.Sprintf("%d", botUin)

	textMsg := fmt.Sprintf("来自云湖群 [%s] 的视频消息\n发送人: %s (用户ID: %s)", yhGroupName, senderNickname, senderID)

	videoFileURI := videoPath
	if !strings.HasPrefix(videoPath, "file://") && !strings.HasPrefix(videoPath, "http://") && !strings.HasPrefix(videoPath, "https://") {
		videoFileURI = "file:///" + filepath.ToSlash(videoPath)
	}

	nodes := []interface{}{
		// 第 1 条节点消息：独立文本消息节点（使用机器人真实账号与头像）
		map[string]interface{}{
			"type": "node",
			"data": map[string]interface{}{
				"name": botName,
				"uin":  botUinStr,
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"data": map[string]interface{}{
							"text": textMsg,
						},
					},
				},
			},
		},
		// 第 2 条节点消息：独立视频消息节点（使用机器人真实账号与头像）
		map[string]interface{}{
			"type": "node",
			"data": map[string]interface{}{
				"name": botName,
				"uin":  botUinStr,
				"content": []interface{}{
					map[string]interface{}{
						"type": "video",
						"data": map[string]interface{}{
							"file": videoFileURI,
						},
					},
				},
			},
		},
	}

	return GlobalQQSender.SendGroupForwardMsg(qqGroupID, nodes)
}

func SendToAllBindings(platform, id, msgType, content, senderID, senderNickname, noBaseContent, msgID string) string {
	banStatus, err := db.IsInBlacklist(senderID)
	if err == nil && banStatus.IsBanned {
		log.Printf("[Sync] 用户 %s 处于黑名单中，取消消息同步", senderID)
		return "用户处于黑名单中"
	}

	hasBlocked, cleaned := ContainsBlockedWords(content)
	if hasBlocked {
		content = cleaned
	}

	keyLocal := fmt.Sprintf("%s:%s:%s:%s", platform, id, platform, id)
	msgToSave := map[string]interface{}{
		"sender_id":       senderID,
		"sender_nickname": senderNickname,
		"message_type":    msgType,
		"message_content": content,
		"timestamp":       time.Now().Format("2006-01-02 15:04:05"),
		"msg_id":          msgID,
		"platform_from":   platform,
		"id_from":         id,
	}
	db.SaveMessageLog(keyLocal, msgToSave, msgID)

	forwardContent := content
	if noBaseContent != "" && msgType != "video" {
		if msgType == "a2ui" {
			forwardContent = noBaseContent
		} else {
			forwardContent = ReplaceBlockedWords(noBaseContent)
		}
	}

	bindInfo := db.GetInfo(platform, id)
	if bindInfo.Status != 0 {
		log.Printf("[Sync Warning] 获取 %s:%s 绑定信息失败: %s", platform, id, bindInfo.Msg)
		return bindInfo.Msg
	}

	bindData := bindInfo.Data
	if platform == "QQ" {
		var yhGroupIDs []db.BindingItem
		if v, ok := bindData["YH_group_ids"].([]db.BindingItem); ok {
			yhGroupIDs = v
		}

		for _, g := range yhGroupIDs {
			if g.Sync {
				keyAB := fmt.Sprintf("QQ:%s:YH:%s", id, g.ID)
				keyBA := fmt.Sprintf("YH:%s:QQ:%s", g.ID, id)
				db.SaveMessageLog(keyAB, msgToSave, "")
				db.SaveMessageLog(keyBA, msgToSave, "")

				if GlobalYHSender != nil {
					sendContentType := msgType
					if sendContentType != "a2ui" {
						sendContentType = "html"
					}
					yhMsgID, err := GlobalYHSender.Send(g.ID, "group", sendContentType, forwardContent)
					if err != nil {
						log.Printf("[Sync Error] 同步到云湖群 %s 失败: %v", g.ID, err)
					} else {
						if yhMsgID != "" {
							db.SaveYunhuMsgCache(msgID, g.ID, "group", senderID, yhMsgID)
							if qqMsgIDNum, err := strconv.ParseInt(msgID, 10, 64); err == nil && qqMsgIDNum != 0 {
								qqGroupIDNum, _ := strconv.ParseInt(id, 10, 64)
								db.SaveQQMsgMapping(qqMsgIDNum, qqGroupIDNum, senderID, senderNickname, content, yhMsgID)
							}
						}
						log.Printf("[Sync Success] 成功同步消息到云湖群 %s (yhMsgId: %s)", g.ID, yhMsgID)
					}
				} else {
					log.Printf("[Sync Warning] 云湖发送器未初始化")
				}
			}
		}
	} else if platform == "YH" {
		var qqGroupIDs []db.BindingItem
		if v, ok := bindData["QQ_group_ids"].([]db.BindingItem); ok {
			qqGroupIDs = v
		}

		for _, g := range qqGroupIDs {
			if g.Sync {
				keyAB := fmt.Sprintf("YH:%s:QQ:%s", id, g.ID)
				keyBA := fmt.Sprintf("QQ:%s:YH:%s", g.ID, id)
				db.SaveMessageLog(keyAB, msgToSave, "")
				db.SaveMessageLog(keyBA, msgToSave, "")

				if GlobalQQSender != nil {
					qqGroupID, err := strconv.ParseInt(g.ID, 10, 64)
					if err == nil {
						var sendErr error
						if msgType == "video" {
							videoFilePath := content
							sendErr = SendForwardVideoToQQ(qqGroupID, senderNickname, senderID, id, videoFilePath)
							if sendErr != nil {
								log.Printf("[Sync Warning] 使用 OneBot 11 合并转发视频失败 (%v)，尝试使用常规消息同步", sendErr)
								headerText := ReplaceBlockedWords(noBaseContent)
								_, sendErr = GlobalQQSender.SendGroupMsg(qqGroupID, fmt.Sprintf("%s\n[CQ:video,file=%s]", headerText, videoFilePath))
							} else {
								log.Printf("[Sync Success] 成功通过 OneBot 11 合并转发节点将视频消息同步至 QQ 群 %d", qqGroupID)
							}
						} else {
							var sentQQMsgID int64
							sentQQMsgID, sendErr = GlobalQQSender.SendGroupMsg(qqGroupID, forwardContent)
							if sendErr == nil {
								log.Printf("[Sync Success] 成功同步消息到 QQ 群 %d (qqMsgId: %d)", qqGroupID, sentQQMsgID)
								if sentQQMsgID != 0 && msgID != "" {
									db.BindYunhuMsgToQQMsg(msgID, sentQQMsgID)
									db.SaveQQMsgMapping(sentQQMsgID, qqGroupID, senderID, senderNickname, content, msgID)
								}
							}
						}

						if sendErr != nil {
							log.Printf("[Sync Error] 同步到 QQ 群 %d 失败: %v", qqGroupID, sendErr)
						}
					}
				} else {
					log.Printf("[Sync Warning] QQ 发送器未注册 (OneBot WS 未连接)")
				}
			}
		}
	}

	return "消息已同步至所有绑定群聊"
}

func SendA2UIToAllBindings(platform, id, content, senderID, senderNickname, a2uiJSON, msgID string) string {
	bindInfo := db.GetInfo(platform, id)
	if bindInfo.Status != 0 {
		return bindInfo.Msg
	}

	msgToSave := map[string]interface{}{
		"sender_id":       senderID,
		"sender_nickname": senderNickname,
		"message_type":    "video",
		"message_content": content,
		"timestamp":       time.Now().Format("2006-01-02 15:04:05"),
		"msg_id":          msgID,
		"platform_from":   platform,
		"id_from":         id,
	}

	bindData := bindInfo.Data
	if platform == "QQ" {
		var yhGroupIDs []db.BindingItem
		if v, ok := bindData["YH_group_ids"].([]db.BindingItem); ok {
			yhGroupIDs = v
		}

		for _, g := range yhGroupIDs {
			if g.Sync {
				keyAB := fmt.Sprintf("QQ:%s:YH:%s", id, g.ID)
				keyBA := fmt.Sprintf("YH:%s:QQ:%s", g.ID, id)
				db.SaveMessageLog(keyAB, msgToSave, "")
				db.SaveMessageLog(keyBA, msgToSave, "")

				if GlobalYHSender != nil {
					yhMsgID, err := GlobalYHSender.Send(g.ID, "group", "a2ui", a2uiJSON)
					if err != nil {
						log.Printf("[Sync Error] 同步 A2UI 消息到云湖群 %s 失败: %v", g.ID, err)
					} else {
						if yhMsgID != "" {
							db.SaveYunhuMsgCache(msgID, g.ID, "group", senderID, yhMsgID)
						}
						log.Printf("[Sync Success] 成功同步 A2UI 消息到云湖群 %s (yhMsgId: %s)", g.ID, yhMsgID)
					}
				} else {
					log.Printf("[Sync Warning] 云湖发送器未初始化")
				}
			}
		}
	}

	return "消息已同步至所有绑定群聊"
}

func SetBoardForAllGroups(platform, id, messageContent, groupName string) {
	bindInfo := db.GetInfo(platform, id)
	if bindInfo.Status != 0 {
		return
	}

	boardContent := fmt.Sprintf("【提醒】\n%s群：%s | %s\n  %s", platform, groupName, id, messageContent)
	bindData := bindInfo.Data

	if platform == "QQ" {
		var yhGroupIDs []db.BindingItem
		if v, ok := bindData["YH_group_ids"].([]db.BindingItem); ok {
			yhGroupIDs = v
		}

		for _, g := range yhGroupIDs {
			if g.Sync && GlobalYHSender != nil {
				_ = GlobalYHSender.SetBoard(g.ID, "group", boardContent)
			}
		}
	}
}
