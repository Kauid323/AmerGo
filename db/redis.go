package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RDB *redis.Client
	Ctx = context.Background()

	memoryReportStore   sync.Map
	memoryYunhuMsgStore sync.Map
	memoryQQMsgStore    sync.Map
	memoryYhToQQStore   sync.Map
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
	YunhuMsgID   string `json:"yunhu_msg_id"`
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

type QQMsgMapping struct {
	QQMsgID    int64  `json:"qq_msg_id"`
	QQGroupID  int64  `json:"qq_group_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	RawText    string `json:"raw_text"`
	YHMsgID    string `json:"yh_msg_id,omitempty"`
}

var memoryQqToYhStore sync.Map // map[string]string (qqMsgID -> yhMsgID)

// SaveQQMsgMapping stores the mapping from QQ Message ID and Yunhu Message ID
func SaveQQMsgMapping(qqMsgID, qqGroupID int64, senderID, senderName, rawText, yhMsgID string) {
	if qqMsgID == 0 {
		return
	}
	qqKey := fmt.Sprintf("%d", qqMsgID)
	// If yhMsgID not provided, check if we already have it cached
	if yhMsgID == "" {
		if v, ok := memoryQqToYhStore.Load(qqKey); ok {
			yhMsgID = v.(string)
		}
	}
	mapping := QQMsgMapping{
		QQMsgID:    qqMsgID,
		QQGroupID:  qqGroupID,
		SenderID:   senderID,
		SenderName: senderName,
		RawText:    rawText,
		YHMsgID:    yhMsgID,
	}
	bytes, _ := json.Marshal(mapping)
	memoryQQMsgStore.Store(qqKey, string(bytes))

	if RDB != nil {
		_ = RDB.Set(Ctx, fmt.Sprintf("qq_msg_map:%s", qqKey), string(bytes), 7*24*time.Hour).Err()
	}

	if yhMsgID != "" {
		memoryYhToQQStore.Store(yhMsgID, qqKey)
		memoryQqToYhStore.Store(qqKey, yhMsgID)
		if RDB != nil {
			_ = RDB.Set(Ctx, fmt.Sprintf("yh_to_qq:%s", yhMsgID), qqKey, 7*24*time.Hour).Err()
			_ = RDB.Set(Ctx, fmt.Sprintf("qq_to_yh:%s", qqKey), yhMsgID, 7*24*time.Hour).Err()
		}
	}
}

// BindYunhuMsgToQQMsg links an already sent Yunhu Message ID to its original QQ Message ID
func BindYunhuMsgToQQMsg(yhMsgID string, qqMsgID int64) {
	if yhMsgID == "" || qqMsgID == 0 {
		return
	}
	qqKey := fmt.Sprintf("%d", qqMsgID)
	memoryYhToQQStore.Store(yhMsgID, qqKey)
	memoryQqToYhStore.Store(qqKey, yhMsgID)
	if RDB != nil {
		_ = RDB.Set(Ctx, fmt.Sprintf("yh_to_qq:%s", yhMsgID), qqKey, 7*24*time.Hour).Err()
		_ = RDB.Set(Ctx, fmt.Sprintf("qq_to_yh:%s", qqKey), yhMsgID, 7*24*time.Hour).Err()
	}
}

// GetYunhuMsgIDByQQMsgID resolves a QQ message ID to the corresponding Yunhu message ID
func GetYunhuMsgIDByQQMsgID(qqMsgID int64) (string, bool) {
	if qqMsgID == 0 {
		return "", false
	}
	qqKey := fmt.Sprintf("%d", qqMsgID)
	if v, ok := memoryQqToYhStore.Load(qqKey); ok {
		return v.(string), true
	}
	if RDB != nil {
		if v, err := RDB.Get(Ctx, fmt.Sprintf("qq_to_yh:%s", qqKey)).Result(); err == nil && v != "" {
			return v, true
		}
	}
	if mapping, ok := GetQQMsgInfo(qqMsgID); ok && mapping != nil && mapping.YHMsgID != "" {
		return mapping.YHMsgID, true
	}
	return "", false
}

// GetQQMsgIDByYunhuMsgID resolves a Yunhu message ID to the corresponding QQ message ID (if forwarded from QQ)
func GetQQMsgIDByYunhuMsgID(yhMsgID string) (int64, bool) {
	if yhMsgID == "" {
		return 0, false
	}
	var qqKey string
	if v, ok := memoryYhToQQStore.Load(yhMsgID); ok {
		qqKey = v.(string)
	} else if RDB != nil {
		if v, err := RDB.Get(Ctx, fmt.Sprintf("yh_to_qq:%s", yhMsgID)).Result(); err == nil && v != "" {
			qqKey = v
		}
	}
	if qqKey == "" {
		return 0, false
	}
	var id int64
	if _, err := fmt.Sscanf(qqKey, "%d", &id); err == nil && id != 0 {
		return id, true
	}
	return 0, false
}

// GetQQMsgInfo retrieves cached QQ message information by QQ Message ID
func GetQQMsgInfo(qqMsgID int64) (*QQMsgMapping, bool) {
	if qqMsgID == 0 {
		return nil, false
	}
	qqKey := fmt.Sprintf("%d", qqMsgID)
	var valStr string
	if v, ok := memoryQQMsgStore.Load(qqKey); ok {
		valStr = v.(string)
	} else if RDB != nil {
		if v, err := RDB.Get(Ctx, fmt.Sprintf("qq_msg_map:%s", qqKey)).Result(); err == nil && v != "" {
			valStr = v
		}
	}
	if valStr == "" {
		return nil, false
	}
	var mapping QQMsgMapping
	if err := json.Unmarshal([]byte(valStr), &mapping); err == nil {
		return &mapping, true
	}
	return nil, false
}

var (
	memoryBlacklistStore  sync.Map
	memoryBlacklistExpire sync.Map
)

func AddToBlacklist(userID string, reason string, duration int) error {
	key := fmt.Sprintf("blacklist:%s", userID)
	notifiedKey := fmt.Sprintf("blacklist_notified:%s", userID)
	expireKey := fmt.Sprintf("blacklist_expire:%s", userID)

	now := time.Now().Unix()
	var expireTime int64
	if duration > 0 {
		expireTime = now + int64(duration)
		memoryBlacklistExpire.Store(userID, expireTime)
	} else {
		memoryBlacklistExpire.Delete(userID)
	}
	memoryBlacklistStore.Store(userID, reason)

	if RDB != nil {
		_ = RDB.Set(Ctx, key, reason, 0).Err()
		_ = RDB.Set(Ctx, notifiedKey, "false", 0).Err()

		if duration > 0 {
			dur := time.Duration(duration) * time.Second
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
	memoryBlacklistStore.Delete(userID)
	memoryBlacklistExpire.Delete(userID)

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
		if val, ok := memoryBlacklistStore.Load(userID); ok {
			reason := val.(string)
			var remainingTime int64
			if expVal, ok := memoryBlacklistExpire.Load(userID); ok {
				expTime := expVal.(int64)
				now := time.Now().Unix()
				if expTime > 0 && now > expTime {
					_ = RemoveFromBlacklist(userID)
					return BlacklistStatus{IsBanned: false}, nil
				}
				remainingTime = expTime - now
			}
			return BlacklistStatus{
				IsBanned:      true,
				Reason:        reason,
				RemainingTime: remainingTime,
			}, nil
		}
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

type BlacklistItem struct {
	UserID        string `json:"user_id"`
	Reason        string `json:"reason"`
	RemainingTime int64  `json:"remaining_time"`
}

func GetBlacklist() []BlacklistItem {
	var list []BlacklistItem
	userMap := make(map[string]bool)

	if RDB != nil {
		keys, err := RDB.Keys(Ctx, "blacklist:*").Result()
		if err == nil {
			for _, k := range keys {
				if strings.HasPrefix(k, "blacklist_notified:") || strings.HasPrefix(k, "blacklist_expire:") {
					continue
				}
				uid := strings.TrimPrefix(k, "blacklist:")
				if uid != "" && !userMap[uid] {
					userMap[uid] = true
					status, _ := IsInBlacklist(uid)
					if status.IsBanned {
						list = append(list, BlacklistItem{
							UserID:        uid,
							Reason:        status.Reason,
							RemainingTime: status.RemainingTime,
						})
					}
				}
			}
		}
	}

	// 补充内存中的黑名单项
	memoryBlacklistStore.Range(func(key, value interface{}) bool {
		uid := key.(string)
		if !userMap[uid] {
			userMap[uid] = true
			status, _ := IsInBlacklist(uid)
			if status.IsBanned {
				list = append(list, BlacklistItem{
					UserID:        uid,
					Reason:        status.Reason,
					RemainingTime: status.RemainingTime,
				})
			}
		}
		return true
	})

	return list
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
