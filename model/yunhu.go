package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

type FlexibleString string

func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*fs = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*fs = FlexibleString(s)
		return nil
	}
	var n interface{}
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*fs = FlexibleString(fmt.Sprintf("%v", n))
	return nil
}

func (fs FlexibleString) String() string {
	return string(fs)
}

type YunhuHeader struct {
	EventID   FlexibleString `json:"eventId"`
	EventType FlexibleString `json:"eventType"`
	EventTime int64          `json:"eventTime"`
}

type YunhuSender struct {
	SenderID        FlexibleString `json:"senderId"`
	SenderType      FlexibleString `json:"senderType"`
	SenderUserLevel FlexibleString `json:"senderUserLevel"`
	SenderNickname  FlexibleString `json:"senderNickname"`
}

type YunhuChat struct {
	ChatID   FlexibleString `json:"chatId"`
	ChatType FlexibleString `json:"chatType"`
}

type YunhuContent struct {
	Text          string                 `json:"text"`
	ImageURL      string                 `json:"imageUrl"`
	ImageName     string                 `json:"imageName"`
	VideoURL      string                 `json:"videoUrl"`
	VideoDuration int                    `json:"videoDuration"`
	Etag          string                 `json:"etag"`
	Parent        string                 `json:"parent"`
	FormJson      map[string]interface{} `json:"formJson"`
}

func (c *YunhuContent) GetFormFieldVal(fieldID string) (interface{}, bool) {
	if c.FormJson == nil {
		return nil, false
	}
	for _, item := range c.FormJson {
		if m, ok := item.(map[string]interface{}); ok {
			id, _ := m["id"].(string)
			label, _ := m["label"].(string)

			if id == fieldID || label == fieldID || strings.EqualFold(id, fieldID) || strings.EqualFold(label, fieldID) {
				if sval, ok := m["selectValue"]; ok && sval != nil && fmt.Sprintf("%v", sval) != "" {
					return sval, true
				}
				if val, ok := m["value"]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
					return val, true
				}
			}
		}
	}
	return nil, false
}

// GetFormFieldByFuzzyLabel looks up a field value whose label contains any of the keywords
func (c *YunhuContent) GetFormFieldByFuzzyLabel(keywords ...string) string {
	if c.FormJson == nil {
		return ""
	}
	for _, item := range c.FormJson {
		if m, ok := item.(map[string]interface{}); ok {
			label, _ := m["label"].(string)
			id, _ := m["id"].(string)
			for _, kw := range keywords {
				if (label != "" && strings.Contains(label, kw)) || (id != "" && strings.EqualFold(id, kw)) {
					if sval, ok := m["selectValue"]; ok && sval != nil && fmt.Sprintf("%v", sval) != "" {
						return strings.TrimSpace(fmt.Sprintf("%v", sval))
					}
					if val, ok := m["value"]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
						return strings.TrimSpace(fmt.Sprintf("%v", val))
					}
				}
			}
		}
	}
	return ""
}

// GetFirstInputFieldValue returns the value of the first non-empty input field
func (c *YunhuContent) GetFirstInputFieldValue() string {
	if c.FormJson == nil {
		return ""
	}
	for _, item := range c.FormJson {
		if m, ok := item.(map[string]interface{}); ok {
			fieldType, _ := m["type"].(string)
			if fieldType == "input" || fieldType == "textarea" {
				if val, ok := m["value"]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
					return strings.TrimSpace(fmt.Sprintf("%v", val))
				}
			}
		}
	}
	return ""
}

func (c *YunhuContent) GetFormFieldString(fieldID string) string {
	val, ok := c.GetFormFieldVal(fieldID)
	if !ok || val == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", val))
}

func (c *YunhuContent) GetFormFieldBool(fieldID string) bool {
	val, ok := c.GetFormFieldVal(fieldID)
	if !ok || val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	s := strings.ToLower(fmt.Sprintf("%v", val))
	return s == "true" || s == "1"
}

type YunhuMessage struct {
	MsgID           FlexibleString `json:"msgId"`
	ParentID        FlexibleString `json:"parentId"`
	SendTime        int64          `json:"sendTime"`
	ChatID          FlexibleString `json:"chatId"`
	ChatType        FlexibleString `json:"chatType"`
	ContentType     FlexibleString `json:"contentType"`
	Content         YunhuContent   `json:"content"`
	InstructionID   FlexibleString `json:"instructionId"`
	InstructionName FlexibleString `json:"instructionName"`
	CommandID       FlexibleString `json:"commandId"`
	CommandName     FlexibleString `json:"commandName"`
}

type YunhuEventBody struct {
	MsgID             FlexibleString         `json:"msgId"`
	RecvID            FlexibleString         `json:"recvId"`
	RecvType          FlexibleString         `json:"recvType"`
	UserID            FlexibleString         `json:"userId"`
	UserName          FlexibleString         `json:"userName"`
	Sender            YunhuSender            `json:"sender"`
	Chat              YunhuChat              `json:"chat"`
	Message           YunhuMessage           `json:"message"`
	SettingJson       FlexibleString         `json:"settingJson"`
	GroupID           FlexibleString         `json:"groupId"`
	ActionName        FlexibleString         `json:"actionName"`
	SourceComponentID FlexibleString         `json:"sourceComponentId"`
	FormContext       map[string]interface{} `json:"formContext"`
	Value             FlexibleString         `json:"value"`
	Data              map[string]interface{} `json:"data"`
}

func (b *YunhuEventBody) GetMsgID() string {
	if b.MsgID.String() != "" {
		return b.MsgID.String()
	}
	return b.Message.MsgID.String()
}

func (b *YunhuEventBody) GetChatID() string {
	if b.RecvID.String() != "" {
		return b.RecvID.String()
	}
	if b.Message.ChatID.String() != "" {
		return b.Message.ChatID.String()
	}
	return b.Chat.ChatID.String()
}

func (b *YunhuEventBody) GetChatType() string {
	if b.RecvType.String() != "" {
		return b.RecvType.String()
	}
	if b.Message.ChatType.String() != "" {
		return b.Message.ChatType.String()
	}
	return b.Chat.ChatType.String()
}

type YunhuEvent struct {
	Version string         `json:"version"`
	Header  YunhuHeader    `json:"header"`
	Event   YunhuEventBody `json:"event"`
}

// Struct for API sending payloads
type YunhuSendRequest struct {
	RecvID      string `json:"recvId"`
	RecvType    string `json:"recvType"` // "group" or "user"
	ContentType string `json:"contentType"` // "text", "html", "markdown"
	Content     string `json:"content"`
}

type YunhuBoardRequest struct {
	RecvID   string `json:"recvId"`
	RecvType string `json:"recvType"`
	Content  string `json:"content"`
}
