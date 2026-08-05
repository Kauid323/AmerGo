package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RDB *redis.Client
	Ctx = context.Background()

	memoryReportStore   sync.Map
	memoryYunhuMsgStore sync.Map
)

func InitRedis(host string, port int, db int, password string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(Ctx).Err(); err != nil {
		return fmt.Errorf("无法连接到 Redis 服务器 (%s): %w", addr, err)
	}

	RDB = client
	log.Printf("[Redis] 成功连接到 %s (DB %d)", addr, db)
	return nil
}

type BlacklistStatus struct {
	IsBanned      bool   `json:"is_banned"`
	Reason        string `json:"reason,omitempty"`
	Notified      bool   `json:"notified"`
	RemainingTime int64  `json:"remaining_time,omitempty"`
}

type ReportRecord struct {
	ReportID     string `json:"report_id"`
	MsgID        string `json:"msg_id"`
	ChatID       string `json:"chat_id"`
	ChatType     string `json:"chat_type"`
	GroupID      string `json:"group_id"`
	SenderID     string `json:"sender_id"`
	SenderName   string `json:"sender_name"`
	ReporterID   string `json:"reporter_id"`
	ReporterName string `json:"reporter_name"`
	Reason       string `json:"reason"`
	Status       string `json:"status"` // "PENDING", "APPROVED", "REJECTED", "RECALLED"
	CreatedAt    string `json:"created_at"`
}

func SaveReportRecord(report *ReportRecord) error {
	bytes, err := json.Marshal(report)
	if err != nil {
		return err
	}
	memoryReportStore.Store(report.ReportID, string(bytes))
	if RDB != nil {
		key := fmt.Sprintf("report_record:%s", report.ReportID)
		_ = RDB.Set(Ctx, key, string(bytes), 7*24*time.Hour).Err()
	}
	return nil
}

func GetReportRecord(reportID string) (*ReportRecord, error) {
	var valStr string
	if RDB != nil {
		key := fmt.Sprintf("report_record:%s", reportID)
		if v, err := RDB.Get(Ctx, key).Result(); err == nil && v != "" {
			valStr = v
		}
	}
	if valStr == "" {
		if v, ok := memoryReportStore.Load(reportID); ok {
			valStr = v.(string)
		}
	}
	if valStr == "" {
		return nil, fmt.Errorf("未找到 ID 为 %s 的举报记录", reportID)
	}
	var rec ReportRecord
	if err := json.Unmarshal([]byte(valStr), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

type YunhuMsgCache struct {
	RealMsgID string `json:"real_msg_id"`
	ChatID    string `json:"chat_id"`
	ChatType  string `json:"chat_type"`
	SenderID  string `json:"sender_id"`
}

func SaveYunhuMsgCache(keyMsgID, chatID, chatType, senderID, realYHMsgID string) {
	if keyMsgID == "" {
		return
	}
	if realYHMsgID == "" {
		realYHMsgID = keyMsgID
	}
	data := YunhuMsgCache{
		RealMsgID: realYHMsgID,
		ChatID:    chatID,
		ChatType:  chatType,
		SenderID:  senderID,
	}
	bytes, _ := json.Marshal(data)
	memoryYunhuMsgStore.Store(keyMsgID, string(bytes))
	if RDB != nil {
		key := fmt.Sprintf("yh_msg_cache:%s", keyMsgID)
		_ = RDB.Set(Ctx, key, string(bytes), 7*24*time.Hour).Err()
	}
}

func GetYunhuMsgCache(keyMsgID string) (realYHMsgID, chatID, chatType, senderID string, found bool) {
	if keyMsgID == "" {
		return "", "", "", "", false
	}
	var valStr string
	if RDB != nil {
		key := fmt.Sprintf("yh_msg_cache:%s", keyMsgID)
		if v, err := RDB.Get(Ctx, key).Result(); err == nil && v != "" {
			valStr = v
		}
	}
	if valStr == "" {
		if v, ok := memoryYunhuMsgStore.Load(keyMsgID); ok {
			valStr = v.(string)
		}
	}
	if valStr == "" {
		return "", "", "", "", false
	}
	var data YunhuMsgCache
	_ = json.Unmarshal([]byte(valStr), &data)
	if data.RealMsgID == "" {
		data.RealMsgID = keyMsgID
	}
	return data.RealMsgID, data.ChatID, data.ChatType, data.SenderID, true
}

func AddToBlacklist(userID string, reason string, duration int) error {
	key := fmt.Sprintf("blacklist:%s", userID)
	notifiedKey := fmt.Sprintf("blacklist_notified:%s", userID)
	expireKey := fmt.Sprintf("blacklist_expire:%s", userID)

	if RDB != nil {
		_ = RDB.Set(Ctx, key, reason, 0).Err()
		_ = RDB.Set(Ctx, notifiedKey, "false", 0).Err()

		if duration > 0 {
			dur := time.Duration(duration) * time.Second
			expireTime := time.Now().Unix() + int64(duration)
			_ = RDB.Set(Ctx, expireKey, expireTime, dur).Err()
			_ = RDB.Expire(Ctx, key, dur).Err()
			_ = RDB.Expire(Ctx, notifiedKey, dur).Err()
		} else {
			_ = RDB.Del(Ctx, expireKey).Err()
		}
	}

	return nil
}

func RemoveFromBlacklist(userID string) error {
	if RDB == nil {
		return nil
	}
	key := fmt.Sprintf("blacklist:%s", userID)
	notifiedKey := fmt.Sprintf("blacklist_notified:%s", userID)
	expireKey := fmt.Sprintf("blacklist_expire:%s", userID)
	return RDB.Del(Ctx, key, notifiedKey, expireKey).Err()
}

func IsInBlacklist(userID string) (BlacklistStatus, error) {
	if RDB == nil {
		return BlacklistStatus{IsBanned: false}, nil
	}
	key := fmt.Sprintf("blacklist:%s", userID)
	exists, err := RDB.Exists(Ctx, key).Result()
	if err != nil || exists == 0 {
		return BlacklistStatus{IsBanned: false}, nil
	}

	reason, _ := RDB.Get(Ctx, key).Result()
	expireStr, _ := RDB.Get(Ctx, fmt.Sprintf("blacklist_expire:%s", userID)).Result()

	var remainingTime int64
	if expireStr != "" {
		var expireTime int64
		fmt.Sscanf(expireStr, "%d", &expireTime)
		now := time.Now().Unix()
		if expireTime > 0 && now > expireTime {
			_ = RemoveFromBlacklist(userID)
			return BlacklistStatus{IsBanned: false}, nil
		}
		remainingTime = expireTime - now
	}

	return BlacklistStatus{
		IsBanned:      true,
		Reason:        reason,
		RemainingTime: remainingTime,
	}, nil
}

func DetectMessageFrequency(platform, userID string, threshold int, timeWindow int) bool {
	if RDB == nil {
		return false
	}
	key := fmt.Sprintf("message_frequency:%s:%s", platform, userID)
	count, err := RDB.Incr(Ctx, key).Result()
	if err != nil {
		return false
	}

	if count == 1 {
		_ = RDB.Expire(Ctx, key, time.Duration(timeWindow)*time.Second)
	}

	return count > int64(threshold)
}

func StoreSensitiveMessage(platform, groupID, senderID, senderNickname, content string) {
	key := fmt.Sprintf("sensitive_messages:%s:%s", platform, groupID)
	msgData := map[string]string{
		"sender_id":       senderID,
		"sender_nickname": senderNickname,
		"message_content": content,
		"timestamp":       time.Now().Format("2006-01-02 15:04:05"),
		"platform_from":   platform,
		"id_from":         groupID,
	}

	bytes, _ := json.Marshal(msgData)
	if RDB != nil {
		_ = RDB.RPush(Ctx, key, string(bytes)).Err()
	}
}

func SaveMessageLog(key string, msgData map[string]interface{}, msgID string) {
	bytes, _ := json.Marshal(msgData)
	if RDB != nil {
		_ = RDB.RPush(Ctx, key, string(bytes)).Err()

		if msgID != "" {
			_ = RDB.Set(Ctx, fmt.Sprintf("msg_id:%s", msgID), string(bytes), 0).Err()
		}
	}
}
