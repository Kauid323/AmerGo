package yunhu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
