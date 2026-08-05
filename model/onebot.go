package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

type FlexibleInt64 int64

func (fi *FlexibleInt64) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*fi = 0
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		var val int64
		fmt.Sscanf(s, "%d", &val)
		*fi = FlexibleInt64(val)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*fi = FlexibleInt64(n)
	return nil
}

func (fi FlexibleInt64) Int64() int64 {
	return int64(fi)
}

func (fi FlexibleInt64) String() string {
	return fmt.Sprintf("%d", fi)
}

type OneBotSender struct {
	UserID   FlexibleInt64 `json:"user_id"`
	Nickname string        `json:"nickname"`
	Card     string        `json:"card"`
	Role     string        `json:"role"`
}

type OneBotEvent struct {
	SelfID      FlexibleInt64   `json:"self_id"`
	UserID      FlexibleInt64   `json:"user_id"`
	Time        int64           `json:"time"`
	MessageID   FlexibleInt64   `json:"message_id"`
	MessageSeq  FlexibleInt64   `json:"message_seq"`
	RealID      FlexibleInt64   `json:"real_id"`
	MessageType string          `json:"message_type"` // "private" or "group"
	RawMessage  string          `json:"raw_message"`
	Message     json.RawMessage `json:"message"`
	Font        int32           `json:"font"`
	SubType     string          `json:"sub_type"`
	PostType    string          `json:"post_type"` // "message", "request", "notice", "meta_event"
	GroupID     FlexibleInt64   `json:"group_id"`
	Sender      OneBotSender    `json:"sender"`

	// Request / Notice fields
	DetailType string `json:"detail_type"`
	Flag       string `json:"flag"`
	Echo       string `json:"echo"`
}

func (e *OneBotEvent) GetSenderUserID() int64 {
	if e.Sender.UserID.Int64() != 0 {
		return e.Sender.UserID.Int64()
	}
	return e.UserID.Int64()
}

func (e *OneBotEvent) GetRawMessage() string {
	if e.RawMessage != "" {
		return e.RawMessage
	}
	if len(e.Message) == 0 {
		return ""
	}

	var strMsg string
	if err := json.Unmarshal(e.Message, &strMsg); err == nil {
		return strMsg
	}

	var msgSegments []map[string]interface{}
	if err := json.Unmarshal(e.Message, &msgSegments); err == nil {
		var sb strings.Builder
		for _, seg := range msgSegments {
			segType, _ := seg["type"].(string)
			data, _ := seg["data"].(map[string]interface{})
			if segType == "text" {
				if text, ok := data["text"].(string); ok {
					sb.WriteString(text)
				}
			} else if segType == "image" {
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,file=%s]", file))
				}
			} else if segType == "at" {
				qq, _ := data["qq"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:at,qq=%s]", qq))
			} else if segType == "face" {
				id, _ := data["id"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:face,id=%s]", id))
			} else if segType == "forward" {
				id, _ := data["id"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:forward,id=%s]", id))
			} else if segType == "record" {
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,file=%s]", file))
				}
			} else if segType == "video" {
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,file=%s]", file))
				}
			}
		}
		return sb.String()
	}

	return string(e.Message)
}

type OneBotAction struct {
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
	Echo   string                 `json:"echo"`
}

type OneBotResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

type OneBotGroupInfo struct {
	GroupID   FlexibleInt64 `json:"group_id"`
	GroupName string        `json:"group_name"`
}
