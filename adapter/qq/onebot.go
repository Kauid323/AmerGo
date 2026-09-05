package qq

import (
	"amer/model"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
