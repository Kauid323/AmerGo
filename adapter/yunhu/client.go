package yunhu

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"amer/config"
	"amer/db"
)

type Client struct {
	httpClient *http.Client
}

var YHClient = &Client{
	httpClient: &http.Client{Timeout: 10 * time.Second},
}

type commonResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		MsgID       string `json:"msgId"`
		MessageInfo struct {
			MsgID string `json:"msgId"`
		} `json:"messageInfo"`
	} `json:"data"`
}

func isSuccess(code int, msg string) bool {
	if code == 0 || code == 1 || strings.EqualFold(msg, "success") {
		return true
	}
	return false
}

var BotSendBaseURL = "https://chat-go.jwzhd.com"

func (c *Client) Send(recvID, recvType, contentType, content string) (string, error) {
	token := config.AppConfig.YH.Token
	if token == "" {
		return "", fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("%s/open-apis/v1/bot/send?token=%s", BotSendBaseURL, token)

	payload := map[string]interface{}{
		"recvId":      recvID,
		"recvType":    recvType,
		"contentType": contentType,
		"content": map[string]interface{}{
			"text": content,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu] 发送消息请求失败: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Yunhu] 发送消息响应 (%s:%s): %s", recvType, recvID, string(respBody))

	var apiResp commonResp
	if err := json.Unmarshal(respBody, &apiResp); err == nil {
		if !isSuccess(apiResp.Code, apiResp.Msg) {
			return "", fmt.Errorf("%s", apiResp.Msg)
		}
		msgID := apiResp.Data.MessageInfo.MsgID
		if msgID == "" {
			msgID = apiResp.Data.MsgID
		}
		if msgID != "" {
			db.SaveYunhuMsgCache(msgID, recvID, recvType, "", msgID)
			return msgID, nil
		}
	}

	return "", nil
}

// SendMessageItem resends a YunhuMessageItem using its original contentType and content
func (c *Client) SendMessageItem(recvID, recvType string, item *YunhuMessageItem) (string, error) {
	if item == nil {
		return "", fmt.Errorf("消息项为空")
	}
	token := config.AppConfig.YH.Token
	if token == "" {
		return "", fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("%s/open-apis/v1/bot/send?token=%s", BotSendBaseURL, token)

	payload := map[string]interface{}{
		"recvId":      recvID,
		"recvType":    recvType,
		"contentType": item.ContentType,
		"content":     item.Content,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu] 转发被举报消息请求失败: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Yunhu] 转发被举报消息响应 (%s:%s): %s", recvType, recvID, string(respBody))

	var apiResp commonResp
	if err := json.Unmarshal(respBody, &apiResp); err == nil {
		if !isSuccess(apiResp.Code, apiResp.Msg) {
			return "", fmt.Errorf("%s", apiResp.Msg)
		}
		msgID := apiResp.Data.MessageInfo.MsgID
		if msgID == "" {
			msgID = apiResp.Data.MsgID
		}
		return msgID, nil
	}
	return "", nil
}

func (c *Client) SendWithButtons(recvID, recvType, contentType, text string, buttons []interface{}) error {
	token := config.AppConfig.YH.Token
	if token == "" {
		return fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("https://chat-go.jwzhd.com/open-apis/v1/bot/send?token=%s", token)

	contentMap := map[string]interface{}{
		"text": text,
	}
	if len(buttons) > 0 {
		contentMap["buttons"] = buttons
	}

	payload := map[string]interface{}{
		"recvId":      recvID,
		"recvType":    recvType,
		"contentType": contentType,
		"content":     contentMap,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu] 发送带按钮消息请求失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Yunhu] 发送带按钮消息响应 (%s:%s): %s", recvType, recvID, string(respBody))

	var apiResp commonResp
	if err := json.Unmarshal(respBody, &apiResp); err == nil {
		if !isSuccess(apiResp.Code, apiResp.Msg) {
			return fmt.Errorf("%s", apiResp.Msg)
		}
		if apiResp.Data.MsgID != "" {
			db.SaveYunhuMsgCache(apiResp.Data.MsgID, recvID, recvType, "", apiResp.Data.MsgID)
		}
	}

	return nil
}

func (c *Client) Recall(msgID, chatID, chatType string) error {
	token := config.AppConfig.YH.Token
	if token == "" {
		return fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("https://chat-go.jwzhd.com/open-apis/v1/bot/recall?token=%s", token)

	payload := map[string]interface{}{
		"msgId":    msgID,
		"chatId":   chatID,
		"chatType": chatType,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu] 撤回消息请求失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Yunhu] 撤回消息响应 (%s:%s msgId:%s): %s", chatType, chatID, msgID, string(respBody))

	var apiResp commonResp
	if err := json.Unmarshal(respBody, &apiResp); err == nil {
		if !isSuccess(apiResp.Code, apiResp.Msg) {
			return fmt.Errorf("%s", apiResp.Msg)
		}
	}

	return nil
}

func (c *Client) SetBoard(recvID, recvType, content string) error {
	token := config.AppConfig.YH.Token
	if token == "" {
		return fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("https://chat-go.jwzhd.com/open-apis/v1/bot/board?token=%s", token)

	payload := map[string]interface{}{
		"recvId":      recvID,
		"recvType":    recvType,
		"contentType": "text",
		"content":     content,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu] 设置看板请求失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	return nil
}

type YunhuGroupInfo struct {
	ID           int64  `json:"id"`
	GroupID      string `json:"groupId"`
	Name         string `json:"name"`
	Introduction string `json:"introduction"`
	CreateBy     string `json:"createBy"`
	AvatarURL    string `json:"avatarUrl"`
	Headcount    int64  `json:"headcount"`
}

type YunhuGroupInfoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Group YunhuGroupInfo `json:"group"`
	} `json:"data"`
}

var (
	yhGroupNameCache sync.Map // map[string]string (groupID -> groupName)
	yhGroupInfoCache sync.Map // map[string]*YunhuGroupInfo
)

var GroupInfoBaseURL = "https://chat-web-go.jwzhd.com/v1/group/group-info"

// FetchGroupInfo fetches official group metadata from Yunhu API https://chat-web-go.jwzhd.com/v1/group/group-info
func (c *Client) FetchGroupInfo(groupID string) (*YunhuGroupInfo, error) {
	if groupID == "" {
		return nil, fmt.Errorf("empty groupID")
	}

	reqBody := map[string]string{"groupId": groupID}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", GroupInfoBaseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result YunhuGroupInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 1 && result.Code != 0 && !strings.EqualFold(result.Msg, "success") {
		return nil, fmt.Errorf("api error: code=%d msg=%s", result.Code, result.Msg)
	}

	group := &result.Data.Group
	if group.Name != "" {
		yhGroupNameCache.Store(groupID, group.Name)
		yhGroupInfoCache.Store(groupID, group)
	}
	return group, nil
}

// GetGroupInfo returns cached or newly fetched Yunhu group information
func (c *Client) GetGroupInfo(groupID string) *YunhuGroupInfo {
	if groupID == "" {
		return nil
	}
	if val, ok := yhGroupInfoCache.Load(groupID); ok {
		if g, ok := val.(*YunhuGroupInfo); ok && g != nil {
			return g
		}
	}

	info, err := c.FetchGroupInfo(groupID)
	if err == nil && info != nil && info.Name != "" {
		return info
	}
	return nil
}

// GetGroupName returns the human-readable Yunhu group name, fetching it dynamically from the official API.
func (c *Client) GetGroupName(groupID string) string {
	if groupID == "" {
		return ""
	}
	if val, ok := yhGroupNameCache.Load(groupID); ok {
		if name, ok := val.(string); ok && name != "" {
			return name
		}
	}

	// 远程拉取群信息并缓存
	info, err := c.FetchGroupInfo(groupID)
	if err == nil && info != nil && info.Name != "" {
		return info.Name
	}

	fallback := fmt.Sprintf("云湖群-%s", groupID)
	// 避免短时间内因网络异常频繁重试，缓存兜底值
	yhGroupNameCache.Store(groupID, fallback)
	return fallback
}

type YunhuUserInfo struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	IsVip     int    `json:"isVip"`
}

type YunhuUserInfoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		User YunhuUserInfo `json:"user"`
	} `json:"data"`
}

var (
	yhUserInfoCache sync.Map // map[string]*YunhuUserInfo
)

var UserHomepageBaseURL = "https://chat-web-go.jwzhd.com/v1/user/homepage"

// FetchUserInfo queries user homepage info from Yunhu API https://chat-web-go.jwzhd.com/v1/user/homepage?userId={userId}
func (c *Client) FetchUserInfo(userID string) (*YunhuUserInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("empty userID")
	}

	apiURL := fmt.Sprintf("%s?userId=%s", UserHomepageBaseURL, userID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result YunhuUserInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 1 && result.Code != 0 && !strings.EqualFold(result.Msg, "success") {
		return nil, fmt.Errorf("api error: code=%d msg=%s", result.Code, result.Msg)
	}

	user := &result.Data.User
	if user.Nickname != "" {
		yhUserInfoCache.Store(userID, user)
	}
	return user, nil
}

// GetUserInfo returns cached or newly fetched Yunhu user information
func (c *Client) GetUserInfo(userID string) *YunhuUserInfo {
	if userID == "" {
		return nil
	}
	if val, ok := yhUserInfoCache.Load(userID); ok {
		if u, ok := val.(*YunhuUserInfo); ok && u != nil {
			return u
		}
	}

	info, err := c.FetchUserInfo(userID)
	if err == nil && info != nil && info.Nickname != "" {
		return info
	}
	return nil
}

var ImageUploadBaseURL = "https://chat-go.jwzhd.com"

// UploadImage uploads image binary data to Yunhu official image upload API and returns the public CDN URL.
func (c *Client) UploadImage(imgData []byte, filename string) (string, error) {
	token := config.AppConfig.YH.Token
	if token == "" {
		return "", fmt.Errorf("云湖机器人 token 未配置")
	}

	// 1. 推断扩展名与 MIME 类型（云湖要求 jpeg 必须使用 .jpg 后缀）
	ext := ".jpg"
	mimeType := "image/jpeg"
	if len(imgData) >= 8 {
		if imgData[0] == 0xff && imgData[1] == 0xd8 && imgData[2] == 0xff {
			ext = ".jpg"
			mimeType = "image/jpeg"
		} else if imgData[0] == 0x89 && imgData[1] == 'P' && imgData[2] == 'N' && imgData[3] == 'G' {
			ext = ".png"
			mimeType = "image/png"
		} else if string(imgData[0:6]) == "GIF87a" || string(imgData[0:6]) == "GIF89a" {
			ext = ".gif"
			mimeType = "image/gif"
		} else if len(imgData) >= 12 && string(imgData[0:4]) == "RIFF" && string(imgData[8:12]) == "WEBP" {
			ext = ".webp"
			mimeType = "image/webp"
		}
	}

	hash := md5.Sum(imgData)
	imageHash := hex.EncodeToString(hash[:])
	targetFilename := imageHash + ext
	if filename != "" && strings.Contains(filename, ".") {
		origExt := strings.ToLower(filepath.Ext(filename))
		if origExt == ".jpeg" {
			origExt = ".jpg"
		}
		if origExt == ".jpg" || origExt == ".png" || origExt == ".gif" || origExt == ".webp" {
			targetFilename = imageHash + origExt
			ext = origExt
		}
	}

	apiURL := fmt.Sprintf("%s/open-apis/v1/image/upload?token=%s", ImageUploadBaseURL, token)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, targetFilename))
	h.Set("Content-Type", mimeType)

	part, err := writer.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("创建表单分块失败: %v", err)
	}
	if _, err := part.Write(imgData); err != nil {
		return "", fmt.Errorf("写入图片数据失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("关闭表单写入器失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return "", fmt.Errorf("创建上传请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	uploadClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := uploadClient.Do(req)
	if err != nil {
		log.Printf("[Yunhu Upload Error] 上传图片到云湖网络错误: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[Yunhu Upload] 上传图片响应: %s", string(respBytes))

	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ImageKey string `json:"imageKey"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", fmt.Errorf("解析云湖图片上传响应失败: %v", err)
	}

	if res.Code != 1 && !strings.EqualFold(res.Msg, "success") {
		return "", fmt.Errorf("云湖图片上传接口返回失败: %s (code: %d)", res.Msg, res.Code)
	}

	yunhuImgURL := fmt.Sprintf("https://chat-img.jwznb.com/%s%s", imageHash, ext)
	log.Printf("[Yunhu Upload Success] 图片成功上传至云湖，访问地址: %s", yunhuImgURL)
	return yunhuImgURL, nil
}

type YunhuMessageItem struct {
	MsgID          string      `json:"msgId"`
	ParentID       string      `json:"parentId"`
	SenderID       string      `json:"senderId"`
	SenderType     string      `json:"senderType"`
	SenderNickname string      `json:"senderNickname"`
	ContentType    string      `json:"contentType"`
	Content        interface{} `json:"content"`
	SendTime       int64       `json:"sendTime"`
	CommandName    string      `json:"commandName"`
	CommandID      int64       `json:"commandId"`
}

type YunhuMessagesResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		List  []YunhuMessageItem `json:"list"`
		Total int                `json:"total"`
	} `json:"data"`
}

// GetMessage fetches a specific message from Yunhu chat by msgId, chatId and chatType according to official document 400-450
func (c *Client) GetMessage(chatID, chatType, msgID string) (*YunhuMessageItem, error) {
	token := config.AppConfig.YH.Token
	if token == "" {
		return nil, fmt.Errorf("云湖机器人 token 未配置")
	}
	if chatID == "" || chatType == "" || msgID == "" {
		return nil, fmt.Errorf("参数不能为空: chatID=%s, chatType=%s, msgID=%s", chatID, chatType, msgID)
	}

	apiURL := fmt.Sprintf("https://chat-go.jwzhd.com/open-apis/v1/bot/messages?token=%s&chat-id=%s&chat-type=%s&message-id=%s&before=1&after=0",
		token, chatID, chatType, msgID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res YunhuMessagesResp
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("解析云湖消息列表响应失败: %v, raw: %s", err, string(bodyBytes))
	}

	if !isSuccess(res.Code, res.Msg) {
		return nil, fmt.Errorf("云湖获取消息失败: %s (code: %d)", res.Msg, res.Code)
	}

	if len(res.Data.List) == 0 {
		return nil, fmt.Errorf("未找到对应的云湖消息: %s", msgID)
	}

	for _, item := range res.Data.List {
		if item.MsgID == msgID {
			return &item, nil
		}
	}

	return &res.Data.List[0], nil
}

// FormatYunhuMessageContent parses text or html content from a YunhuMessageItem
func FormatYunhuMessageContent(item *YunhuMessageItem) string {
	if item == nil {
		return ""
	}
	if m, ok := item.Content.(map[string]interface{}); ok {
		if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if html, ok := m["html"].(string); ok && strings.TrimSpace(html) != "" {
			return strings.TrimSpace(html)
		}
		if img, ok := m["imageUrl"].(string); ok && img != "" {
			return fmt.Sprintf("[图片: %s]", img)
		}
		if video, ok := m["videoUrl"].(string); ok && video != "" {
			return fmt.Sprintf("[视频: %s]", video)
		}
	} else if s, ok := item.Content.(string); ok {
		return strings.TrimSpace(s)
	}
	return fmt.Sprintf("[%s 消息]", item.ContentType)
}

