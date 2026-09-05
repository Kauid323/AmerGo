package qq

import (
	"amer/adapter/message"
	"amer/model"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type memberCacheItem struct {
	name     string
	expireAt time.Time
}

type groupCacheItem struct {
	name     string
	expireAt time.Time
}

type OneBotServer struct {
	conn         *websocket.Conn
	connMu       sync.Mutex
	echoSeq      int64
	pendingReq   map[string]chan model.OneBotResponse
	reqMu        sync.Mutex
	SelfID       int64
	SelfNickname string
	memberCache  map[string]memberCacheItem
	memberMu     sync.RWMutex
	groupCache   map[int64]groupCacheItem
	groupMu      sync.RWMutex
}

var GlobalOneBotServer = &OneBotServer{
	pendingReq:  make(map[string]chan model.OneBotResponse),
	memberCache: make(map[string]memberCacheItem),
	groupCache:  make(map[int64]groupCacheItem),
}

func (s *OneBotServer) WsHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[OneBot WS Error] WebSocket 升级失败: %v", err)
		return
	}

	s.connMu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = conn
	s.connMu.Unlock()

	log.Printf("[OneBot WS Success] NapCat / OneBot V11 反向 WebSocket 客户端 (%s) 成功建立连接！", c.Request.RemoteAddr)

	go s.FetchLoginInfo()

	defer func() {
		s.connMu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.connMu.Unlock()
		conn.Close()
		log.Printf("[OneBot WS] 连接断开 (%s)", c.Request.RemoteAddr)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[OneBot WS Read Error] 消息读取错误: %v", err)
			break
		}

		log.Printf("[OneBot WS Recv] 收到原始消息: %s", string(message))

		var resp model.OneBotResponse
		if err := json.Unmarshal(message, &resp); err == nil && resp.Echo != "" {
			s.reqMu.Lock()
			ch, ok := s.pendingReq[resp.Echo]
			if ok {
				delete(s.pendingReq, resp.Echo)
			}
			s.reqMu.Unlock()
			if ok {
				ch <- resp
				continue
			}
		}

		var event model.OneBotEvent
		if err := json.Unmarshal(message, &event); err == nil && event.PostType != "" {
			if event.SelfID.Int64() != 0 {
				s.connMu.Lock()
				s.SelfID = event.SelfID.Int64()
				s.connMu.Unlock()
			}
			go HandleOneBotEvent(event)
		} else {
			log.Printf("[OneBot WS Parse Error] 解析 OneBot 事件失败: %v", err)
		}
	}
}

func (s *OneBotServer) FetchLoginInfo() {
	time.Sleep(500 * time.Millisecond)
	resp, err := s.CallAPI("get_login_info", map[string]interface{}{})
	if err == nil && resp.Status == "ok" {
		var info struct {
			UserID   model.FlexibleInt64 `json:"user_id"`
			Nickname string              `json:"nickname"`
		}
		if err := json.Unmarshal(resp.Data, &info); err == nil {
			s.connMu.Lock()
			s.SelfID = info.UserID.Int64()
			s.SelfNickname = info.Nickname
			s.connMu.Unlock()
			log.Printf("[OneBot Self Info] 成功获取机器人当前登录账号信息: QQ=%d, 昵称=%s", s.SelfID, s.SelfNickname)
		}
	}
}

func (s *OneBotServer) GetSelfInfo() (int64, string) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.SelfID != 0 {
		name := s.SelfNickname
		if name == "" {
			name = "Amer"
		}
		return s.SelfID, name
	}
	return 10000, "Amer"
}

func (s *OneBotServer) CallAPI(action string, params map[string]interface{}) (model.OneBotResponse, error) {
	return s.CallAPIWithTimeout(action, params, 30*time.Second)
}

func (s *OneBotServer) CallAPIWithTimeout(action string, params map[string]interface{}, timeout time.Duration) (model.OneBotResponse, error) {
	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()

	if conn == nil {
		return model.OneBotResponse{}, fmt.Errorf("OneBot WebSocket 未连接")
	}

	seq := atomic.AddInt64(&s.echoSeq, 1)
	echoID := fmt.Sprintf("amer_req_%d", seq)

	reqPayload := model.OneBotAction{
		Action: action,
		Params: params,
		Echo:   echoID,
	}

	respChan := make(chan model.OneBotResponse, 1)
	s.reqMu.Lock()
	s.pendingReq[echoID] = respChan
	s.reqMu.Unlock()

	bytes, err := json.Marshal(reqPayload)
	if err != nil {
		s.reqMu.Lock()
		delete(s.pendingReq, echoID)
		s.reqMu.Unlock()
		return model.OneBotResponse{}, err
	}

	log.Printf("[OneBot API Send] 发送 API 请求: %s", string(bytes))

	s.connMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, bytes)
	s.connMu.Unlock()

	if err != nil {
		s.reqMu.Lock()
		delete(s.pendingReq, echoID)
		s.reqMu.Unlock()
		return model.OneBotResponse{}, err
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(timeout):
		s.reqMu.Lock()
		delete(s.pendingReq, echoID)
		s.reqMu.Unlock()
		return model.OneBotResponse{}, fmt.Errorf("OneBot API 请求超时 (%s)", action)
	}
}

func (s *OneBotServer) SendGroupMsg(groupID int64, message string) error {
	_, err := s.CallAPI("send_group_msg", map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	})
	return err
}

func (s *OneBotServer) SendGroupForwardMsg(groupID int64, nodes []interface{}) error {
	resp, err := s.CallAPI("send_group_forward_msg", map[string]interface{}{
		"group_id": groupID,
		"messages": nodes,
	})
	if err != nil {
		return err
	}
	if resp.Status == "failed" {
		return fmt.Errorf("OneBot API 错误 (%d): %s", resp.RetCode, resp.Wording)
	}
	return nil
}

func (s *OneBotServer) SendPrivateMsg(userID int64, message string) error {
	_, err := s.CallAPI("send_private_msg", map[string]interface{}{
		"user_id": userID,
		"message": message,
	})
	return err
}

func (s *OneBotServer) GetGroupName(groupID int64) string {
	s.groupMu.RLock()
	if item, ok := s.groupCache[groupID]; ok && time.Now().Before(item.expireAt) {
		s.groupMu.RUnlock()
		return item.name
	}
	s.groupMu.RUnlock()

	resp, err := s.CallAPIWithTimeout("get_group_info", map[string]interface{}{
		"group_id": groupID,
	}, 3*time.Second)
	if err != nil {
		return fmt.Sprintf("QQ群-%d", groupID)
	}

	var info model.OneBotGroupInfo
	if err := json.Unmarshal(resp.Data, &info); err == nil && info.GroupName != "" {
		s.groupMu.Lock()
		s.groupCache[groupID] = groupCacheItem{
			name:     info.GroupName,
			expireAt: time.Now().Add(1 * time.Hour),
		}
		s.groupMu.Unlock()
		return info.GroupName
	}
	return fmt.Sprintf("QQ群-%d", groupID)
}

func (s *OneBotServer) UpdateMemberNameCache(groupID int64, userID int64, name string) {
	name = strings.TrimSpace(name)
	if name == "" || userID == 0 {
		return
	}
	s.memberMu.Lock()
	defer s.memberMu.Unlock()
	key := fmt.Sprintf("%d:%d", groupID, userID)
	s.memberCache[key] = memberCacheItem{
		name:     name,
		expireAt: time.Now().Add(30 * time.Minute),
	}
}

func (s *OneBotServer) GetGroupMemberName(groupID int64, userID int64) string {
	if userID == 0 {
		return ""
	}

	key := fmt.Sprintf("%d:%d", groupID, userID)
	s.memberMu.RLock()
	if item, ok := s.memberCache[key]; ok && time.Now().Before(item.expireAt) {
		s.memberMu.RUnlock()
		return item.name
	}
	s.memberMu.RUnlock()

	defaultName := strconv.FormatInt(userID, 10)
	if s.SelfID != 0 && userID == s.SelfID && s.SelfNickname != "" {
		defaultName = s.SelfNickname
	}

	// 1. 若在群内 (groupID != 0)，优先调用 get_group_member_info 获取群名片/昵称
	if groupID != 0 {
		resp, err := s.CallAPIWithTimeout("get_group_member_info", map[string]interface{}{
			"group_id": groupID,
			"user_id":  userID,
			"no_cache": false,
		}, 3*time.Second)
		if err == nil && resp.Status == "ok" {
			var info model.OneBotMemberInfo
			if err := json.Unmarshal(resp.Data, &info); err == nil {
				targetName := strings.TrimSpace(info.Card)
				if targetName == "" {
					targetName = strings.TrimSpace(info.Nickname)
				}
				if targetName != "" {
					s.memberMu.Lock()
					s.memberCache[key] = memberCacheItem{
						name:     targetName,
						expireAt: time.Now().Add(30 * time.Minute),
					}
					s.memberMu.Unlock()
					return targetName
				}
			}
		}
	}

	// 2. 尝试获取陌生人/用户个人资料
	resp, err := s.CallAPIWithTimeout("get_stranger_info", map[string]interface{}{
		"user_id":  userID,
		"no_cache": false,
	}, 2*time.Second)
	if err == nil && resp.Status == "ok" {
		var info model.OneBotStrangerInfo
		if err := json.Unmarshal(resp.Data, &info); err == nil {
			targetName := strings.TrimSpace(info.Nickname)
			if targetName != "" {
				s.memberMu.Lock()
				s.memberCache[key] = memberCacheItem{
					name:     targetName,
					expireAt: time.Now().Add(30 * time.Minute),
				}
				s.memberMu.Unlock()
				return targetName
			}
		}
	}

	// 3. 查询失败时短期缓存 1 分钟，防止短时间内高频超时重发
	s.memberMu.Lock()
	s.memberCache[key] = memberCacheItem{
		name:     defaultName,
		expireAt: time.Now().Add(1 * time.Minute),
	}
	s.memberMu.Unlock()

	return defaultName
}

var replyMsgCache sync.Map // key: int64, value: *message.ReplyMsgInfo

func extractTextFromOneBotMsg(msg interface{}) string {
	if msg == nil {
		return ""
	}
	if s, ok := msg.(string); ok {
		return s
	}
	if arr, ok := msg.([]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				segType, _ := m["type"].(string)
				data, _ := m["data"].(map[string]interface{})
				switch segType {
				case "text":
					if text, ok := data["text"].(string); ok {
						sb.WriteString(text)
					}
				case "image":
					sb.WriteString("[图片]")
				case "video":
					sb.WriteString("[视频]")
				case "record":
					sb.WriteString("[语音]")
				case "face":
					sb.WriteString("[表情]")
				case "at":
					if qq, ok := data["qq"].(string); ok {
						sb.WriteString("@" + qq + " ")
					}
				default:
					sb.WriteString(fmt.Sprintf("[%s]", segType))
				}
			}
		}
		return sb.String()
	}
	return ""
}

func formatReplySummary(raw string) string {
	// 去除嵌套的 reply CQ 码
	reReply := regexp.MustCompile(`\[CQ:reply,[^\]]+\]`)
	raw = reReply.ReplaceAllString(raw, "")

	// 还原 CQ 码文本转义 (&#91; -> [, &#93; -> ], &#44; -> ,, &#38; -> &)
	raw = message.UnescapeCQ(raw)

	// 转换云湖文本表情为 Emoji
	raw = message.ConvertYunhuEmoji(raw)

	// 转换 CQ 码为自然语言描述
	reImg := regexp.MustCompile(`\[CQ:image,[^\]]+\]`)
	raw = reImg.ReplaceAllString(raw, "[图片]")
	reVideo := regexp.MustCompile(`\[CQ:video,[^\]]+\]`)
	raw = reVideo.ReplaceAllString(raw, "[视频]")
	reAudio := regexp.MustCompile(`\[CQ:record,[^\]]+\]`)
	raw = reAudio.ReplaceAllString(raw, "[语音]")
	reFace := regexp.MustCompile(`\[CQ:face,[^\]]+\]`)
	raw = reFace.ReplaceAllString(raw, "[表情]")

	reAt := regexp.MustCompile(`\[CQ:at,qq=([^,\]]+)[^\]]*\] ?`)
	raw = reAt.ReplaceAllStringFunc(raw, func(m string) string {
		sub := reAt.FindStringSubmatch(m)
		if len(sub) >= 2 {
			qq := sub[1]
			if qq == "all" {
				return "@全体成员 "
			}
			return "@" + qq + " "
		}
		return "@某人 "
	})

	raw = strings.ReplaceAll(raw, "\r\n", " ")
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.TrimSpace(raw)

	runes := []rune(raw)
	if len(runes) > 80 {
		raw = string(runes[:80]) + "..."
	}
	return raw
}

// GetReplyMsg fetches quoted message content and sender by messageID from OneBot API with caching.
func (s *OneBotServer) GetReplyMsg(messageID int64, groupID int64) (*message.ReplyMsgInfo, error) {
	if messageID == 0 {
		return nil, fmt.Errorf("invalid message_id: 0")
	}

	if val, ok := replyMsgCache.Load(messageID); ok {
		if info, ok := val.(*message.ReplyMsgInfo); ok {
			return info, nil
		}
	}

	resp, err := s.CallAPIWithTimeout("get_msg", map[string]interface{}{
		"message_id": messageID,
	}, 3*time.Second)
	if err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("get_msg failed with retcode %d: %s", resp.RetCode, resp.Wording)
	}

	var msgData struct {
		Time        int64               `json:"time"`
		MessageType string              `json:"message_type"`
		MessageID   model.FlexibleInt64 `json:"message_id"`
		Sender      struct {
			UserID   model.FlexibleInt64 `json:"user_id"`
			Nickname string              `json:"nickname"`
			Card     string              `json:"card"`
			Role     string              `json:"role"`
		} `json:"sender"`
		Message    interface{} `json:"message"`
		RawMessage string      `json:"raw_message"`
	}

	if err := json.Unmarshal(resp.Data, &msgData); err != nil {
		return nil, err
	}

	senderName := strings.TrimSpace(msgData.Sender.Card)
	if senderName == "" {
		senderName = strings.TrimSpace(msgData.Sender.Nickname)
	}
	senderUID := msgData.Sender.UserID.Int64()
	if senderName == "" && senderUID != 0 {
		senderName = s.GetGroupMemberName(groupID, senderUID)
	}
	if senderName == "" && senderUID != 0 {
		senderName = strconv.FormatInt(senderUID, 10)
	}

	rawText := msgData.RawMessage
	if rawText == "" {
		rawText = extractTextFromOneBotMsg(msgData.Message)
	}

	summary := formatReplySummary(rawText)

	info := &message.ReplyMsgInfo{
		SenderName: senderName,
		SenderUID:  senderUID,
		RawText:    rawText,
		Summary:    summary,
	}

	replyMsgCache.Store(messageID, info)
	return info, nil
}
