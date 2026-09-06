package model

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
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

type OneBotFile struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Size  FlexibleInt64 `json:"size"`
	BusID int64         `json:"busid"`
	URL   string        `json:"url"`
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
	File        OneBotFile      `json:"file"`

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

// CardInfo represents extracted information from rich share cards (JSON, XML, Miniapp, Music, etc.)
type CardInfo struct {
	Tag     string `json:"tag"`     // 标签，如 "QQ小程序", "音乐", "图文", "分享"
	Title   string `json:"title"`   // 标题 / 来源名
	Desc    string `json:"desc"`    // 描述 / 摘要 / 内容
	Preview string `json:"preview"` // 预览缩略图 URL
	URL     string `json:"url"`     // 跳转链接 URL
}

// EscapeCQParam escapes special CQ characters in a parameter value
func EscapeCQParam(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "[", "&#91;")
	s = strings.ReplaceAll(s, "]", "&#93;")
	s = strings.ReplaceAll(s, ",", "&#44;")
	return s
}

// UnescapeCQParam reverses CQ character escapes
func UnescapeCQParam(s string) string {
	s = strings.ReplaceAll(s, "&#44;", ",")
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

// FormatCardSegment formats CardInfo into a standard [CQ:card,...] string
func FormatCardSegment(card *CardInfo) string {
	if card == nil {
		return ""
	}
	if card.Title == "" && card.Desc == "" && card.Preview == "" && card.URL == "" && card.Tag == "" {
		return ""
	}
	tag := EscapeCQParam(card.Tag)
	title := EscapeCQParam(card.Title)
	desc := EscapeCQParam(card.Desc)
	preview := EscapeCQParam(card.Preview)
	url := EscapeCQParam(card.URL)
	return fmt.Sprintf("[CQ:card,tag=%s,title=%s,desc=%s,preview=%s,url=%s]", tag, title, desc, preview, url)
}

func findStringField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// FormatIntOrFloat converts numbers (int, int64, float64, float32) or strings to clean integer decimal strings without scientific notation
func FormatIntOrFloat(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case string:
		v = strings.TrimSpace(v)
		if f, err := strconv.ParseFloat(v, 64); err == nil && !strings.Contains(v, ".") {
			return strconv.FormatInt(int64(f), 10)
		}
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ParseJSONCardData parses raw card data (string or map) and returns CardInfo
func ParseJSONCardData(data interface{}) *CardInfo {
	if data == nil {
		return nil
	}

	var root map[string]interface{}
	switch v := data.(type) {
	case map[string]interface{}:
		if dVal, ok := v["data"]; ok {
			if dStr, ok := dVal.(string); ok {
				if err := json.Unmarshal([]byte(dStr), &root); err != nil {
					unescaped := html.UnescapeString(dStr)
					_ = json.Unmarshal([]byte(unescaped), &root)
				}
			} else if dMap, ok := dVal.(map[string]interface{}); ok {
				root = dMap
			}
		}
		if root == nil {
			root = v
		}
	case string:
		str := strings.TrimSpace(v)
		if str == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(str), &root); err != nil {
			unescaped := html.UnescapeString(str)
			_ = json.Unmarshal([]byte(unescaped), &root)
		}
	}

	if root == nil {
		return nil
	}

	card := &CardInfo{}

	// 1. 解析 prompt 与 Tag
	prompt, _ := root["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	var promptText string
	if prompt != "" {
		rePrompt := regexp.MustCompile(`^\[([^\]]+)\](.*)$`)
		if matches := rePrompt.FindStringSubmatch(prompt); len(matches) >= 3 {
			card.Tag = strings.TrimSpace(matches[1])
			promptText = strings.TrimSpace(matches[2])
		} else {
			promptText = prompt
		}
	}

	app, _ := root["app"].(string)
	if card.Tag == "" {
		if strings.Contains(app, "miniapp") {
			card.Tag = "QQ小程序"
		} else if strings.Contains(app, "music") {
			card.Tag = "音乐"
		} else if strings.Contains(app, "game") {
			card.Tag = "游戏"
		} else if strings.Contains(app, "tuwen") || strings.Contains(app, "news") {
			card.Tag = "图文"
		} else {
			card.Tag = "分享"
		}
	}

	// 2. 解析 meta 内部对象
	var metaTitle, metaDesc, metaPreview, metaURL, metaSource string
	if metaVal, ok := root["meta"].(map[string]interface{}); ok && metaVal != nil {
		for _, subVal := range metaVal {
			subMap, ok := subVal.(map[string]interface{})
			if !ok || subMap == nil {
				continue
			}
			if t := findStringField(subMap, "title", "name"); t != "" && metaTitle == "" {
				metaTitle = t
			}
			if d := findStringField(subMap, "desc", "summary", "digest", "author", "content"); d != "" && metaDesc == "" {
				metaDesc = d
			}
			if p := findStringField(subMap, "preview", "cover", "img", "pic", "avatar", "icon", "thumb"); p != "" && metaPreview == "" {
				metaPreview = p
			}
			if u := findStringField(subMap, "qqdocurl", "jumpUrl", "jump_url", "url", "link", "weburl", "page"); u != "" && metaURL == "" {
				metaURL = u
			}
			if s := findStringField(subMap, "sourcename", "source_name", "app_name", "tag"); s != "" && metaSource == "" {
				metaSource = s
			}
		}
	}

	// 3. 补全 root 级属性
	if metaTitle == "" {
		metaTitle = findStringField(root, "title", "name")
	}
	if metaDesc == "" {
		metaDesc = findStringField(root, "desc", "summary")
	}
	if metaPreview == "" {
		metaPreview = findStringField(root, "preview", "cover", "icon")
	}
	if metaURL == "" {
		metaURL = findStringField(root, "url", "jumpUrl", "jump_url", "link")
	}

	card.Title = metaTitle
	card.Desc = metaDesc
	card.Preview = metaPreview
	card.URL = metaURL

	if card.Title == "" && metaSource != "" {
		card.Title = metaSource
	}
	if card.Desc == "" && promptText != "" {
		card.Desc = promptText
	}
	if card.Title == "" && promptText != "" && card.Desc != promptText {
		card.Title = promptText
	}

	card.URL = strings.ReplaceAll(card.URL, `\/`, "/")
	card.Preview = strings.ReplaceAll(card.Preview, `\/`, "/")

	if card.Title == "" && card.Desc == "" && card.URL == "" && card.Preview == "" {
		if prompt != "" {
			card.Desc = prompt
		} else {
			return nil
		}
	}

	return card
}

// ParseXMLCardData parses XML card raw string and returns CardInfo
func ParseXMLCardData(rawXML string) *CardInfo {
	rawXML = strings.TrimSpace(rawXML)
	if rawXML == "" {
		return nil
	}

	card := &CardInfo{}

	// Tag
	reBrief := regexp.MustCompile(`brief="\[([^\]]+)\]`)
	if m := reBrief.FindStringSubmatch(rawXML); len(m) >= 2 {
		card.Tag = m[1]
	} else {
		card.Tag = "分享"
	}

	// Title
	reTitle := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	if m := reTitle.FindStringSubmatch(rawXML); len(m) >= 2 {
		card.Title = html.UnescapeString(m[1])
	}

	// Desc
	reSummary := regexp.MustCompile(`<summary[^>]*>([^<]+)</summary>`)
	if m := reSummary.FindStringSubmatch(rawXML); len(m) >= 2 {
		card.Desc = html.UnescapeString(m[1])
	}

	// Preview
	reCover := regexp.MustCompile(`cover="([^"]+)"`)
	if m := reCover.FindStringSubmatch(rawXML); len(m) >= 2 {
		card.Preview = html.UnescapeString(m[1])
	}

	// URL
	reURL := regexp.MustCompile(`url="([^"]+)"`)
	if m := reURL.FindStringSubmatch(rawXML); len(m) >= 2 {
		card.URL = html.UnescapeString(m[1])
	} else {
		reURLTag := regexp.MustCompile(`<url[^>]*>([^<]+)</url>`)
		if m := reURLTag.FindStringSubmatch(rawXML); len(m) >= 2 {
			card.URL = html.UnescapeString(m[1])
		}
	}

	// Source
	reSource := regexp.MustCompile(`<source[^>]*name="([^"]+)"`)
	if m := reSource.FindStringSubmatch(rawXML); len(m) >= 2 && card.Title == "" {
		card.Title = html.UnescapeString(m[1])
	}

	if card.Title == "" && card.Desc == "" && card.URL == "" {
		return nil
	}
	return card
}

// ReplaceCardCQ finds [CQ:json,...] and [CQ:xml,...] in text and converts them to standard [CQ:card,...]
func ReplaceCardCQ(text string) string {
	if !strings.Contains(text, "[CQ:json") && !strings.Contains(text, "[CQ:xml") {
		return text
	}

	// 1. 处理 [CQ:json,data=...]
	for {
		idx := strings.Index(text, "[CQ:json")
		if idx == -1 {
			break
		}
		dataIdx := strings.Index(text[idx:], "data=")
		if dataIdx == -1 {
			endIdx := strings.Index(text[idx:], "]")
			if endIdx == -1 {
				break
			}
			text = text[:idx] + text[idx+endIdx+1:]
			continue
		}
		dataStart := idx + dataIdx + 5
		var jsonStr string
		var endIdx int
		if dataStart < len(text) && text[dataStart] == '{' {
			braceCount := 0
			foundEnd := false
			inQuote := false
			escaped := false
			for i := dataStart; i < len(text); i++ {
				c := text[i]
				if escaped {
					escaped = false
					continue
				}
				if c == '\\' {
					escaped = true
					continue
				}
				if c == '"' {
					inQuote = !inQuote
					continue
				}
				if !inQuote {
					if c == '{' {
						braceCount++
					} else if c == '}' {
						braceCount--
						if braceCount == 0 {
							// 找到最外层大括号闭合点
							jsonStr = text[dataStart : i+1]
							// 紧接着找后面的 ']'
							closeBracket := strings.Index(text[i+1:], "]")
							if closeBracket != -1 {
								endIdx = i + 1 + closeBracket
								foundEnd = true
							}
							break
						}
					}
				}
			}
			if !foundEnd {
				closeBracket := strings.Index(text[idx:], "]")
				if closeBracket == -1 {
					break
				}
				endIdx = idx + closeBracket
				jsonStr = text[dataStart:endIdx]
			}
		} else {
			closeBracket := strings.Index(text[idx:], "]")
			if closeBracket == -1 {
				break
			}
			endIdx = idx + closeBracket
			jsonStr = text[dataStart:endIdx]
		}

		card := ParseJSONCardData(jsonStr)
		var replacement string
		if card != nil {
			replacement = FormatCardSegment(card)
		} else {
			replacement = "[卡片分享]"
		}
		text = text[:idx] + replacement + text[endIdx+1:]
	}

	// 2. 处理 [CQ:xml,data=...]
	for {
		idx := strings.Index(text, "[CQ:xml")
		if idx == -1 {
			break
		}
		// 寻找匹配的最外层 ']'（跳过双引号和单引号内部的 ']'）
		inQuote := byte(0)
		closeBracket := -1
		for i := idx + 7; i < len(text); i++ {
			c := text[i]
			if inQuote != 0 {
				if c == inQuote {
					inQuote = 0
				}
			} else {
				if c == '"' || c == '\'' {
					inQuote = c
				} else if c == ']' {
					closeBracket = i
					break
				}
			}
		}
		if closeBracket == -1 {
			break
		}
		cqPart := text[idx : closeBracket+1]
		dataIdx := strings.Index(cqPart, "data=")
		var rawXML string
		if dataIdx != -1 {
			rawXML = cqPart[dataIdx+5 : len(cqPart)-1]
		}
		card := ParseXMLCardData(rawXML)
		var replacement string
		if card != nil {
			replacement = FormatCardSegment(card)
		} else {
			replacement = "[卡片分享]"
		}
		text = text[:idx] + replacement + text[closeBracket+1:]
	}

	return text
}

func (e *OneBotEvent) GetRawMessage() string {
	// 如果 raw_message 包含没有具体参数的 [CQ:markdown]、[CQ:inline_keyboard] 或卡片消息，优先从 e.Message 深度解析！
	needsDeepParse := false
	if strings.Contains(e.RawMessage, "[CQ:markdown]") ||
		strings.Contains(e.RawMessage, "[CQ:inline_keyboard]") ||
		strings.Contains(e.RawMessage, "[CQ:markdown,") ||
		strings.Contains(e.RawMessage, "[CQ:inline_keyboard,") ||
		strings.Contains(e.RawMessage, "[CQ:json") ||
		strings.Contains(e.RawMessage, "[CQ:xml") ||
		(strings.Contains(e.RawMessage, "[CQ:file") && !strings.Contains(e.RawMessage, "url=")) {
		needsDeepParse = true
	}

	if e.RawMessage != "" && !needsDeepParse {
		return ReplaceCardCQ(e.RawMessage)
	}
	if len(e.Message) == 0 {
		return ReplaceCardCQ(e.RawMessage)
	}

	var strMsg string
	if err := json.Unmarshal(e.Message, &strMsg); err == nil && strMsg != "" {
		return ReplaceCardCQ(strMsg)
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
			case "file":
				name := findStringField(data, "name", "file")
				url := findStringField(data, "url")
				sizeStr := ""
				if sz, ok := data["size"]; ok && sz != nil {
					sizeStr = FormatIntOrFloat(sz)
				} else if sz, ok := data["file_size"]; ok && sz != nil {
					sizeStr = FormatIntOrFloat(sz)
				}
				sb.WriteString(fmt.Sprintf("[CQ:file,name=%s,size=%s,url=%s]", EscapeCQParam(name), EscapeCQParam(sizeStr), EscapeCQParam(url)))
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
				id := FormatIntOrFloat(data["id"])
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
			case "json":
				card := ParseJSONCardData(data)
				if card != nil {
					parsed := FormatCardSegment(card)
					if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
						sb.WriteString("\n")
					}
					sb.WriteString(parsed)
				}
			case "xml":
				rawXML := ""
				if dStr, ok := data["data"].(string); ok {
					rawXML = dStr
				}
				card := ParseXMLCardData(rawXML)
				if card != nil {
					parsed := FormatCardSegment(card)
					if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
						sb.WriteString("\n")
					}
					sb.WriteString(parsed)
				}
			}
		}
		result := strings.TrimSpace(sb.String())
		if result != "" {
			return ReplaceCardCQ(result)
		}
	}

	return ReplaceCardCQ(e.RawMessage)
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
