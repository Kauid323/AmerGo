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
		MsgID string `json:"msgId"`
	} `json:"data"`
}

func isSuccess(code int, msg string) bool {
	if code == 0 || code == 1 || strings.EqualFold(msg, "success") {
		return true
	}
	return false
}

func (c *Client) Send(recvID, recvType, contentType, content string) (string, error) {
	token := config.AppConfig.YH.Token
	if token == "" {
		return "", fmt.Errorf("云湖机器人 token 未配置")
	}

	apiURL := fmt.Sprintf("https://chat-go.jwzhd.com/open-apis/v1/bot/send?token=%s", token)

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
		if apiResp.Data.MsgID != "" {
			db.SaveYunhuMsgCache(apiResp.Data.MsgID, recvID, recvType, "", apiResp.Data.MsgID)
			return apiResp.Data.MsgID, nil
		}
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

func (c *Client) GetGroupName(groupID string) string {
	return fmt.Sprintf("云湖群-%s", groupID)
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

