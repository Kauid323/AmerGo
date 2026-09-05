package model

import (
	"encoding/json"
	"fmt"
	"regexp"
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

	// Request / Notice / Meta fields
	RequestType   string        `json:"request_type"`    // "friend", "group"
	NoticeType    string        `json:"notice_type"`     // "group_increase", "group_decrease", etc.
	MetaEventType string        `json:"meta_event_type"` // "heartbeat", "lifecycle"
	DetailType    string        `json:"detail_type"`
	OperatorID    FlexibleInt64 `json:"operator_id"`
	Comment       string        `json:"comment"`
	Flag          string        `json:"flag"`
	Echo          string        `json:"echo"`
}

func (e *OneBotEvent) GetRequestType() string {
	if e.RequestType != "" {
		return e.RequestType
	}
	return e.DetailType
}

func (e *OneBotEvent) GetNoticeType() string {
	if e.NoticeType != "" {
		return e.NoticeType
	}
	return e.DetailType
}

func (e *OneBotEvent) GetSenderUserID() int64 {
	if e.Sender.UserID.Int64() != 0 {
		return e.Sender.UserID.Int64()
	}
	return e.UserID.Int64()
}

// ParseMarkdownSegment parses QQ Bot markdown message content into clean text and OneBot CQ codes.
func ParseMarkdownSegment(content string) string {
	if content == "" {
		return ""
	}

	// 1. 过滤版本元数据，例如 [](version...) 或 [](http...)
	reMeta := regexp.MustCompile(`\[\s*\]\([^\)]*\)`)
	res := reMeta.ReplaceAllString(content, "")

	// 2. 转换 markdown 图片: ![#150px #84px](url) -> [CQ:image,url=url]
	reImg := regexp.MustCompile(`!\[[^\]]*\]\((https?://[^\s\)]+)\)`)
	res = reImg.ReplaceAllString(res, "\n[CQ:image,url=$1]\n")

	// 3. 转换 mention: [@Amer](mqqapi://markdown/mention?at_type=1&at_tinyid=3218936228) -> @Amer
	reMention := regexp.MustCompile(`\[@([^\s\]]+)\]\(mqqapi://markdown/mention\?[^\)]*\)`)
	res = reMention.ReplaceAllString(res, "@$1 ")

	// 4. 转换内部命令或操作跳转: [开关入群欢迎](mqqapi://aio/inlinecmd?command=...) -> [开关入群欢迎]
	reCmd := regexp.MustCompile(`\[([^\]]+)\]\(mqqapi://[^\)]*\)`)
	res = reCmd.ReplaceAllString(res, "[$1]")

	// 5. 规整多余连续空行
	reNewlines := regexp.MustCompile(`\n{3,}`)
	res = reNewlines.ReplaceAllString(res, "\n\n")

	return strings.TrimSpace(res)
}

// ParseInlineKeyboardButtons extracts buttons text from inline_keyboard payload.
func ParseInlineKeyboardButtons(data map[string]interface{}) [][]string {
	var rowsList [][]string
	rowsVal, ok := data["rows"]
	if !ok {
		return rowsList
	}

	rowsArr, ok := rowsVal.([]interface{})
	if !ok {
		return rowsList
	}

	for _, r := range rowsArr {
		rowMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		btnVal, ok := rowMap["buttons"]
		if !ok {
			continue
		}
		btnArr, ok := btnVal.([]interface{})
		if !ok {
			continue
		}

		var rowButtons []string
		for _, b := range btnArr {
			bMap, ok := b.(map[string]interface{})
			if !ok {
				continue
			}
			label, _ := bMap["label"].(string)
			if label == "" {
				label, _ = bMap["visited_label"].(string)
			}
			if label == "" {
				label, _ = bMap["data"].(string)
			}
			label = strings.TrimSpace(label)
			if label != "" {
				rowButtons = append(rowButtons, label)
			}
		}
		if len(rowButtons) > 0 {
			rowsList = append(rowsList, rowButtons)
		}
	}
	return rowsList
}

// ParseInlineKeyboardSegment formats inline_keyboard buttons into CQ code for downstream rendering.
func ParseInlineKeyboardSegment(data map[string]interface{}) string {
	rowsList := ParseInlineKeyboardButtons(data)
	if len(rowsList) == 0 {
		return ""
	}

	var rowStrs []string
	for _, row := range rowsList {
		rowStrs = append(rowStrs, strings.Join(row, "|"))
	}
	encoded := strings.Join(rowStrs, "//")
	return fmt.Sprintf("[CQ:inline_keyboard,buttons=%s]", encoded)
}

func (e *OneBotEvent) GetRawMessage() string {
	// 如果 raw_message 包含没有具体参数的 [CQ:markdown] 或 [CQ:inline_keyboard]，说明被上报层压缩截断，必须优先从 e.Message 深度解析！
	needsDeepParse := false
	if strings.Contains(e.RawMessage, "[CQ:markdown]") ||
		strings.Contains(e.RawMessage, "[CQ:inline_keyboard]") ||
		strings.Contains(e.RawMessage, "[CQ:markdown,") ||
		strings.Contains(e.RawMessage, "[CQ:inline_keyboard,") {
		needsDeepParse = true
	}

	if e.RawMessage != "" && !needsDeepParse {
		return e.RawMessage
	}
	if len(e.Message) == 0 {
		return e.RawMessage
	}

	var strMsg string
	if err := json.Unmarshal(e.Message, &strMsg); err == nil && strMsg != "" {
		return strMsg
	}

	var msgSegments []map[string]interface{}
	if err := json.Unmarshal(e.Message, &msgSegments); err == nil && len(msgSegments) > 0 {
		var sb strings.Builder
		for _, seg := range msgSegments {
			segType, _ := seg["type"].(string)
			data, _ := seg["data"].(map[string]interface{})
			switch segType {
			case "text":
				if text, ok := data["text"].(string); ok {
					sb.WriteString(text)
				}
			case "image":
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:image,file=%s]", file))
				}
			case "at":
				qq, _ := data["qq"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:at,qq=%s]", qq))
			case "face":
				id, _ := data["id"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:face,id=%s]", id))
			case "forward":
				id, _ := data["id"].(string)
				sb.WriteString(fmt.Sprintf("[CQ:forward,id=%s]", id))
			case "record":
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:record,file=%s]", file))
				}
			case "video":
				file, _ := data["file"].(string)
				url, _ := data["url"].(string)
				if url != "" && file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,file=%s,url=%s]", file, url))
				} else if url != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,url=%s]", url))
				} else if file != "" {
					sb.WriteString(fmt.Sprintf("[CQ:video,file=%s]", file))
				}
			case "markdown":
				if content, ok := data["content"].(string); ok {
					parsed := ParseMarkdownSegment(content)
					if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
						sb.WriteString("\n")
					}
					sb.WriteString(parsed)
				}
			case "inline_keyboard":
				parsed := ParseInlineKeyboardSegment(data)
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
				sb.WriteString(parsed)
			}
		}
		result := strings.TrimSpace(sb.String())
		if result != "" {
			return result
		}
	}

	return e.RawMessage
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

type OneBotMemberInfo struct {
	GroupID  FlexibleInt64 `json:"group_id"`
	UserID   FlexibleInt64 `json:"user_id"`
	Nickname string        `json:"nickname"`
	Card     string        `json:"card"`
	Role     string        `json:"role"`
}

type OneBotStrangerInfo struct {
	UserID   FlexibleInt64 `json:"user_id"`
	Nickname string        `json:"nickname"`
}
