package message

import (
	"amer/config"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var imageCache sync.Map // key: file or URL, value: imageCacheItem or string

type imageCacheItem struct {
	url    string
	width  int
	height int
}

const (
	maxImageDisplayWidth  = 300.0
	maxImageDisplayHeight = 320.0
)

// getImageDimensions parses width and height from image binary data (JPEG, PNG, GIF, WebP) without decoding full pixels.
func getImageDimensions(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	// 1. 标准库解码 JPEG, PNG, GIF 配置
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil && cfg.Width > 0 && cfg.Height > 0 {
		return cfg.Width, cfg.Height
	}

	// 2. 解析 WebP 格式
	if len(data) >= 30 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		format := string(data[12:16])
		switch format {
		case "VP8 ":
			// 有损 VP8: 检查起始码 0x9d 0x01 0x2a
			if len(data) >= 30 && data[23] == 0x9d && data[24] == 0x01 && data[25] == 0x2a {
				w := int(binary.LittleEndian.Uint16(data[26:28])) & 0x3fff
				h := int(binary.LittleEndian.Uint16(data[28:30])) & 0x3fff
				return w, h
			}
		case "VP8L":
			// 无损 VP8L: signature 0x2f
			if len(data) >= 25 && data[20] == 0x2f {
				n := uint32(data[21]) | (uint32(data[22]) << 8) | (uint32(data[23]) << 16) | (uint32(data[24]) << 24)
				w := int(n&0x3fff) + 1
				h := int((n>>14)&0x3fff) + 1
				return w, h
			}
		case "VP8X":
			// 扩展 VP8X: 24-bit canvas width-1, canvas height-1
			if len(data) >= 30 {
				w := int(data[24]) | (int(data[25]) << 8) | (int(data[26]) << 16) + 1
				h := int(data[27]) | (int(data[28]) << 8) | (int(data[29]) << 16) + 1
				return w, h
			}
		}
	}

	return 0, 0
}

// calculateDisplayDimensions scales original image dimensions down proportionally according to user config.
func calculateDisplayDimensions(origW, origH int) (int, int) {
	if origW <= 0 || origH <= 0 {
		return 0, 0
	}

	cfg := config.AppConfig.Image
	scale := cfg.Scale
	maxW := float64(cfg.MaxWidth)
	maxH := float64(cfg.MaxHeight)

	if maxW <= 0 {
		maxW = maxImageDisplayWidth
	}
	if maxH <= 0 {
		maxH = maxImageDisplayHeight
	}

	w := float64(origW)
	h := float64(origH)

	// 1. 如果配置了自定义缩小比例 (如 scale: 0.5 或 scale: 50)
	if scale > 0 {
		if scale > 1.0 && scale <= 100.0 {
			scale = scale / 100.0 // 容错兼容百分比写法: 50 -> 0.5
		}
		dW := int(w*scale + 0.5)
		dH := int(h*scale + 0.5)
		if dW < 1 {
			dW = 1
		}
		if dH < 1 {
			dH = 1
		}
		return dW, dH
	}

	// 2. 默认模式：按最大宽高阈值自动等比例缩小
	// 如果原图在最大限制以内，保持原尺寸展示，不放大
	if w <= maxW && h <= maxH {
		return origW, origH
	}

	scaleW := maxW / w
	scaleH := maxH / h
	autoScale := scaleW
	if scaleH < autoScale {
		autoScale = scaleH
	}

	dW := int(w*autoScale + 0.5)
	dH := int(h*autoScale + 0.5)
	if dW < 1 {
		dW = 1
	}
	if dH < 1 {
		dH = 1
	}
	return dW, dH
}

// CompressHTML minimizes whitespace and redundant tags in HTML to reduce payload and rendering height.
func CompressHTML(s string) string {
	s = regexp.MustCompile(`>\s+<`).ReplaceAllString(s, "><")
	s = regexp.MustCompile(`(<br\s*/?>\s*){2,}`).ReplaceAllString(s, "<br>")
	return strings.TrimSpace(s)
}

// formatImageHTML generates an <img> tag with auto-scaled display width and height in a compact layout.
func formatImageHTML(imgURL string, origW, origH int) string {
	escapedURL := html.EscapeString(imgURL)
	if origW > 0 && origH > 0 {
		dW, dH := calculateDisplayDimensions(origW, origH)
		return fmt.Sprintf(`<img src="%s" width="%d" height="%d" style="display:block;width:%dpx;height:%dpx;max-width:100%%;object-fit:contain;border-radius:4px;margin:2px 0;">`, escapedURL, dW, dH, dW, dH)
	}
	maxW := config.AppConfig.Image.MaxWidth
	maxH := config.AppConfig.Image.MaxHeight
	if maxW <= 0 {
		maxW = int(maxImageDisplayWidth)
	}
	if maxH <= 0 {
		maxH = int(maxImageDisplayHeight)
	}
	return fmt.Sprintf(`<img src="%s" style="display:block;max-width:%dpx;max-height:%dpx;border-radius:4px;margin:2px 0;object-fit:contain;">`, escapedURL, maxW, maxH)
}

func downloadImageData(imgURL string) ([]byte, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		contentType = "image/jpeg"
		lower := strings.ToLower(imgURL)
		if strings.Contains(lower, ".png") {
			contentType = "image/png"
		} else if strings.Contains(lower, ".gif") {
			contentType = "image/gif"
		} else if strings.Contains(lower, ".webp") {
			contentType = "image/webp"
		}
	}

	return data, contentType, nil
}

// ProcessCQImage downloads QQ image, uploads to Yunhu image CDN, and returns <img> tag with resolution-scaled dimensions.
func ProcessCQImage(imgURL, fileVal string) string {
	if imgURL == "" && fileVal != "" {
		if strings.HasPrefix(fileVal, "http://") || strings.HasPrefix(fileVal, "https://") {
			imgURL = fileVal
		}
	}
	if imgURL == "" {
		return "[图片]"
	}
	imgURL = html.UnescapeString(imgURL)

	cacheKey := fileVal
	if cacheKey == "" {
		cacheKey = imgURL
	}
	if val, ok := imageCache.Load(cacheKey); ok {
		if item, ok := val.(imageCacheItem); ok && item.url != "" {
			return formatImageHTML(item.url, item.width, item.height)
		}
		if cachedURL, ok := val.(string); ok && cachedURL != "" {
			return formatImageHTML(cachedURL, 0, 0)
		}
	}

	// 1. 下载图片二进制
	data, contentType, err := downloadImageData(imgURL)
	if err != nil {
		log.Printf("[CQ Image] 下载图片失败: %v (URL: %s)", err, imgURL)
		return formatImageHTML(imgURL, 0, 0)
	}

	// 解析图片原始宽高
	origW, origH := getImageDimensions(data)

	// 2. 优先上传到云湖官方图床，解决防盗链并规避 Base64 导致消息内容过长 (code 1002) 的问题
	if GlobalYHSender != nil {
		yhURL, err := GlobalYHSender.UploadImage(data, fileVal)
		if err == nil && yhURL != "" {
			imageCache.Store(cacheKey, imageCacheItem{url: yhURL, width: origW, height: origH})
			return formatImageHTML(yhURL, origW, origH)
		}
		log.Printf("[CQ Image] 上传到云湖失败: %v，准备降级处理", err)
	}

	// 3. 降级处理：小图片 (<= 25KB) 在上传失败时退回到 base64
	if len(data) <= 25*1024 {
		base64Str := base64.StdEncoding.EncodeToString(data)
		base64Src := fmt.Sprintf("data:%s;base64,%s", contentType, base64Str)
		imageCache.Store(cacheKey, imageCacheItem{url: base64Src, width: origW, height: origH})
		return formatImageHTML(base64Src, origW, origH)
	}

	// 4. 大图降级直接展示外链，防止几百 KB 的 base64 撑爆云湖载荷
	imageCache.Store(cacheKey, imageCacheItem{url: imgURL, width: origW, height: origH})
	return formatImageHTML(imgURL, origW, origH)
}

// parseCQParams parses parameter key-value pairs from a CQ code parameter string.
// Key names match (?:^|,) *([a-zA-Z0-9_-]+)= so that query parameters in URLs (e.g. appid=1407) are not misidentified as CQ keys.
func parseCQParams(paramsStr string) map[string]string {
	params := make(map[string]string)
	if paramsStr == "" {
		return params
	}

	keyRe := regexp.MustCompile(`(?:^|,) *([a-zA-Z0-9_-]+)=`)
	matches := keyRe.FindAllStringSubmatchIndex(paramsStr, -1)
	if len(matches) == 0 {
		return params
	}

	for i := 0; i < len(matches); i++ {
		keyName := paramsStr[matches[i][2]:matches[i][3]]
		valStart := matches[i][1]

		valEnd := len(paramsStr)
		if i+1 < len(matches) {
			valEnd = matches[i+1][0]
		}

		val := paramsStr[valStart:valEnd]
		val = strings.TrimSuffix(val, ",")
		params[keyName] = strings.TrimSpace(val)
	}

	return params
}

// UnescapeCQ decodes OneBot/CQ-escaped characters (&#91; -> [, &#93; -> ], &#44; -> ,, &#38; -> &).
func UnescapeCQ(s string) string {
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&#44;", ",")
	s = strings.ReplaceAll(s, "&#38;", "&")
	return s
}

// CQToHTML converts OneBot CQ codes in a message string to HTML elements for display in Yunhu HTML messages.
func CQToHTML(msg string) string {
	return CQToHTMLWithGroup(msg, 0)
}

// CQToHTMLWithGroup converts OneBot CQ codes in a message string to HTML elements, resolving @ mentions with group member nicknames/cards.
func CQToHTMLWithGroup(msg string, groupID int64) string {
	re := regexp.MustCompile(`\[CQ:([a-zA-Z0-9_-]+)(?:,([^\]]*))?\]`)
	res := re.ReplaceAllStringFunc(msg, func(cq string) string {
		matches := re.FindStringSubmatch(cq)
		if len(matches) < 2 {
			return cq
		}
		cqType := matches[1]
		paramsStr := ""
		if len(matches) >= 3 {
			paramsStr = matches[2]
		}
		params := parseCQParams(paramsStr)

		switch cqType {
		case "image":
			imgURL := params["url"]
			fileVal := params["file"]
			return ProcessCQImage(imgURL, fileVal)

		case "at":
			qq := params["qq"]
			if qq == "all" {
				return "@全体成员"
			}
			displayName := qq
			if GlobalQQSender != nil {
				if uid, err := strconv.ParseInt(qq, 10, 64); err == nil && uid != 0 {
					if name := GlobalQQSender.GetGroupMemberName(groupID, uid); name != "" {
						displayName = name
					}
				}
			}
			return fmt.Sprintf("<b>@%s</b> ", html.EscapeString(displayName))

		case "face":
			return "[表情]"

		case "record":
			return "[语音消息]"

		case "video":
			videoURL := params["url"]
			if videoURL == "" {
				fileVal := params["file"]
				if strings.HasPrefix(fileVal, "http://") || strings.HasPrefix(fileVal, "https://") {
					videoURL = fileVal
				}
			}
			if videoURL != "" {
				videoURL = html.UnescapeString(videoURL)
				return fmt.Sprintf(`<video src="%s" controls style="display:block;max-width:100%%;max-height:240px;margin:2px 0;border-radius:4px;"></video>`, videoURL)
			}
			return "[视频消息]"

		case "reply":
			replyIDStr := params["id"]
			if replyIDStr == "" {
				return ""
			}
			var replyInfo *ReplyMsgInfo
			if GlobalQQSender != nil {
				if replyID, err := strconv.ParseInt(replyIDStr, 10, 64); err == nil && replyID != 0 {
					replyInfo, _ = GlobalQQSender.GetReplyMsg(replyID, groupID)
				}
			}
			if replyInfo != nil && replyInfo.SenderName != "" {
				escapedSender := html.EscapeString(UnescapeCQ(replyInfo.SenderName))
				escapedSummary := html.EscapeString(UnescapeCQ(replyInfo.Summary))
				return fmt.Sprintf(`<div style="background:#edf2f7;border-left:3px solid #2563eb;padding:3px 6px;margin-bottom:3px;border-radius:3px;font-size:12px;color:#4a5568;"><strong style="color:#2563eb;">@%s</strong>: %s</div>`, escapedSender, escapedSummary)
			}
			return `<div style="background:#edf2f7;border-left:3px solid #2563eb;padding:3px 6px;margin-bottom:3px;border-radius:3px;font-size:12px;color:#4a5568;"><strong style="color:#2563eb;">[引用消息]</strong></div>`

		case "inline_keyboard":
			buttonsVal := params["buttons"]
			if buttonsVal == "" {
				return ""
			}
			var rowsHTML strings.Builder
			rowsHTML.WriteString(`<div style="margin-top:3px;display:flex;flex-direction:column;gap:3px;">`)
			rows := strings.Split(buttonsVal, "//")
			for _, row := range rows {
				if strings.TrimSpace(row) == "" {
					continue
				}
				rowsHTML.WriteString(`<div style="display:flex;flex-wrap:wrap;gap:4px;">`)
				for _, btn := range strings.Split(row, "|") {
					btn = strings.TrimSpace(btn)
					if btn != "" {
						rowsHTML.WriteString(fmt.Sprintf(
							`<span style="display:inline-block;background:#eff6ff;color:#2563eb;border:1px solid #bfdbfe;border-radius:3px;padding:1px 6px;font-size:11px;font-weight:500;">%s</span>`,
							html.EscapeString(btn),
						))
					}
				}
				rowsHTML.WriteString(`</div>`)
			}
			rowsHTML.WriteString(`</div>`)
			return rowsHTML.String()

		case "markdown":
			contentVal := params["content"]
			if contentVal != "" {
				return CQToHTMLWithGroup(contentVal, groupID)
			}
			return ""

		default:
			return fmt.Sprintf("[%s消息]", cqType)
		}
	})
	res = strings.ReplaceAll(res, "&#91;", "[")
	res = strings.ReplaceAll(res, "&#93;", "]")
	res = strings.ReplaceAll(res, "&#44;", ",")
	res = ConvertYunhuEmoji(res)
	res = regexp.MustCompile(`(</b>)  +`).ReplaceAllString(res, "$1 ")
	return res
}

// ExtractVideoURL extracts the video HTTP/HTTPS URL from a CQ:video code string.
func ExtractVideoURL(msg string) string {
	re := regexp.MustCompile(`\[CQ:video(?:,([^\]]*))?\]`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	params := parseCQParams(matches[1])
	videoURL := params["url"]
	if videoURL == "" {
		fileVal := params["file"]
		if strings.HasPrefix(fileVal, "http://") || strings.HasPrefix(fileVal, "https://") {
			videoURL = fileVal
		}
	}
	if videoURL != "" {
		return html.UnescapeString(videoURL)
	}
	return ""
}

// BuildVideoA2UI generates updateComponents JSON object string for video message forwarding with nested report BottomSheet and input field.
func BuildVideoA2UI(surfaceID, groupName, groupIDStr, senderName, senderIDStr, videoURL, msgID string) string {
	timeStr := time.Now().Format("2006-01-02 15:04:05")

	updateComponentsMsg := map[string]interface{}{
		"version": "v0.9",
		"updateComponents": map[string]interface{}{
			"surfaceId": surfaceID,
			"components": []map[string]interface{}{
				{
					"id":        "root",
					"component": "Column",
					"children": []string{
						"headerText",
						"modalTrigger",
					},
				},
				{
					"id":        "headerText",
					"component": "Text",
					"text":      fmt.Sprintf("[%s] %s(%s):", groupName, senderName, senderIDStr),
					"variant":   "body",
				},
				{
					"id":        "modalTrigger",
					"component": "Modal",
					"trigger":   "openBtn",
					"content":   "modalContent",
				},
				{
					"id":        "openBtn",
					"component": "Button",
					"child":     "btnText",
					"variant":   "primary",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "open_video_modal",
						},
					},
				},
				{
					"id":        "btnText",
					"component": "Text",
					"text":      "🎬 点击播放视频",
				},
				{
					"id":        "modalContent",
					"component": "Column",
					"children": []string{
						"spacerTop",
						"videoPlayer",
						"spacerMid1",
						"divider1",
						"infoTitle",
						"infoText",
						"spacerMid2",
						"divider2",
						"reportModalTrigger",
						"spacerBottom",
					},
				},
				{
					"id":        "spacerTop",
					"component": "Text",
					"text":      "\n",
				},
				{
					"id":        "videoPlayer",
					"component": "Video",
					"url":       videoURL,
				},
				{
					"id":        "spacerMid1",
					"component": "Text",
					"text":      "\n\n",
				},
				{
					"id":        "divider1",
					"component": "Divider",
					"axis":      "horizontal",
				},
				{
					"id":        "infoTitle",
					"component": "Text",
					"text":      "📌 消息详细信息",
					"variant":   "h5",
				},
				{
					"id":        "infoText",
					"component": "Text",
					"text":      fmt.Sprintf("来源群聊：%s (%s)\n发送者：%s (ID: %s)\n发送时间：%s", groupName, groupIDStr, senderName, senderIDStr, timeStr),
					"variant":   "body",
				},
				{
					"id":        "spacerMid2",
					"component": "Text",
					"text":      "\n\n",
				},
				{
					"id":        "divider2",
					"component": "Divider",
					"axis":      "horizontal",
				},
				// Modal trigger for report BottomSheet
				{
					"id":        "reportModalTrigger",
					"component": "Modal",
					"trigger":   "reportBtn",
					"content":   "reportModalContent",
				},
				{
					"id":        "reportBtn",
					"component": "Button",
					"child":     "reportBtnText",
					"variant":   "borderless",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "open_report_modal",
						},
					},
				},
				{
					"id":        "reportBtnText",
					"component": "Text",
					"text":      "🚨 举报该视频消息",
				},
				// Report BottomSheet content
				{
					"id":        "reportModalContent",
					"component": "Column",
					"children": []string{
						"reportTitle",
						"reportReasonInput",
						"divider3",
						"submitReportBtn",
					},
				},
				{
					"id":        "reportTitle",
					"component": "Text",
					"text":      "🚨 提交视频消息举报",
					"variant":   "h4",
				},
				{
					"id":        "reportReasonInput",
					"component": "TextField",
					"label":     "请输入举报原因",
					"variant":   "longText",
					"value": map[string]interface{}{
						"path": "/report_reason",
					},
				},
				{
					"id":        "divider3",
					"component": "Divider",
					"axis":      "horizontal",
				},
				{
					"id":        "submitReportBtn",
					"component": "Button",
					"child":     "submitReportBtnText",
					"variant":   "primary",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "submit_report",
							"context": map[string]interface{}{
								"group_id":  groupIDStr,
								"msg_id":    msgID,
								"sender_id": senderIDStr,
								"reason": map[string]interface{}{
									"path": "/report_reason",
								},
							},
						},
					},
				},
				{
					"id":        "submitReportBtnText",
					"component": "Text",
					"text":      "确认提交举报",
				},
				{
					"id":        "spacerBottom",
					"component": "Text",
					"text":      "\n\n\n\n",
				},
			},
		},
	}

	bytes, _ := json.Marshal(updateComponentsMsg)
	return string(bytes)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FetchAudioDataURI attempts to download the audio with proper headers and convert it to Base64 data URI
func FetchAudioDataURI(audioURL, fileVal string) string {
	if audioURL != "" {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", audioURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Referer", "https://multimedia.nt.qq.com.cn/")
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err == nil && len(data) > 0 && !strings.Contains(string(data[:minInt(len(data), 100)]), "invalid rkey") {
					base64Str := base64.StdEncoding.EncodeToString(data)
					mime := "audio/amr"
					if strings.Contains(audioURL, ".mp3") {
						mime = "audio/mp3"
					}
					return fmt.Sprintf("data:%s;base64,%s", mime, base64Str)
				}
			}
		}
	}
	return audioURL
}

// ExtractAudioURL extracts the audio HTTP/HTTPS URL and file parameter from a CQ:record code string.
func ExtractAudioURL(msg string) (string, string) {
	re := regexp.MustCompile(`\[CQ:record(?:,([^\]]*))?\]`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return "", ""
	}
	params := parseCQParams(matches[1])
	audioURL := params["url"]
	fileVal := params["file"]
	if audioURL == "" {
		if strings.HasPrefix(fileVal, "http://") || strings.HasPrefix(fileVal, "https://") {
			audioURL = fileVal
		}
	}
	if audioURL != "" {
		audioURL = html.UnescapeString(audioURL)
	}
	return audioURL, fileVal
}

// BuildAudioA2UI generates updateComponents JSON object string for voice/audio message forwarding with nested report BottomSheet and input field.
func BuildAudioA2UI(surfaceID, groupName, groupIDStr, senderName, senderIDStr, audioURL, msgID string) string {
	timeStr := time.Now().Format("2006-01-02 15:04:05")

	updateComponentsMsg := map[string]interface{}{
		"version": "v0.9",
		"updateComponents": map[string]interface{}{
			"surfaceId": surfaceID,
			"components": []map[string]interface{}{
				{
					"id":        "root",
					"component": "Column",
					"children": []string{
						"headerText",
						"modalTrigger",
					},
				},
				{
					"id":        "headerText",
					"component": "Text",
					"text":      fmt.Sprintf("[%s] %s(%s):", groupName, senderName, senderIDStr),
					"variant":   "body",
				},
				{
					"id":        "modalTrigger",
					"component": "Modal",
					"trigger":   "openBtn",
					"content":   "modalContent",
				},
				{
					"id":        "openBtn",
					"component": "Button",
					"child":     "btnText",
					"variant":   "primary",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "open_audio_modal",
						},
					},
				},
				{
					"id":        "btnText",
					"component": "Text",
					"text":      "🎵 点击播放语音",
				},
				{
					"id":        "modalContent",
					"component": "Column",
					"children": []string{
						"spacerTop",
						"audioPlayer",
						"spacerMid1",
						"divider1",
						"infoTitle",
						"infoText",
						"spacerMid2",
						"divider2",
						"reportModalTrigger",
						"spacerBottom",
					},
				},
				{
					"id":        "spacerTop",
					"component": "Text",
					"text":      "\n",
				},
				{
					"id":        "audioPlayer",
					"component": "AudioPlayer",
					"url":       audioURL,
				},
				{
					"id":        "spacerMid1",
					"component": "Text",
					"text":      "\n\n",
				},
				{
					"id":        "divider1",
					"component": "Divider",
					"axis":      "horizontal",
				},
				{
					"id":        "infoTitle",
					"component": "Text",
					"text":      "📌 消息详细信息",
					"variant":   "h5",
				},
				{
					"id":        "infoText",
					"component": "Text",
					"text":      fmt.Sprintf("来源群聊：%s (%s)\n发送者：%s (ID: %s)\n发送时间：%s", groupName, groupIDStr, senderName, senderIDStr, timeStr),
					"variant":   "body",
				},
				{
					"id":        "spacerMid2",
					"component": "Text",
					"text":      "\n\n",
				},
				{
					"id":        "divider2",
					"component": "Divider",
					"axis":      "horizontal",
				},
				// Modal trigger for report BottomSheet
				{
					"id":        "reportModalTrigger",
					"component": "Modal",
					"trigger":   "reportBtn",
					"content":   "reportModalContent",
				},
				{
					"id":        "reportBtn",
					"component": "Button",
					"child":     "reportBtnText",
					"variant":   "borderless",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "open_report_modal",
						},
					},
				},
				{
					"id":        "reportBtnText",
					"component": "Text",
					"text":      "🚨 举报该语音消息",
				},
				// Report BottomSheet content
				{
					"id":        "reportModalContent",
					"component": "Column",
					"children": []string{
						"reportTitle",
						"reportReasonInput",
						"divider3",
						"submitReportBtn",
					},
				},
				{
					"id":        "reportTitle",
					"component": "Text",
					"text":      "🚨 提交语音消息举报",
					"variant":   "h4",
				},
				{
					"id":        "reportReasonInput",
					"component": "TextField",
					"label":     "请输入举报原因",
					"variant":   "longText",
					"value": map[string]interface{}{
						"path": "/report_reason",
					},
				},
				{
					"id":        "divider3",
					"component": "Divider",
					"axis":      "horizontal",
				},
				{
					"id":        "submitReportBtn",
					"component": "Button",
					"child":     "submitReportBtnText",
					"variant":   "primary",
					"action": map[string]interface{}{
						"event": map[string]interface{}{
							"name": "submit_report",
							"context": map[string]interface{}{
								"group_id":  groupIDStr,
								"msg_id":    msgID,
								"sender_id": senderIDStr,
								"reason": map[string]interface{}{
									"path": "/report_reason",
								},
							},
						},
					},
				},
				{
					"id":        "submitReportBtnText",
					"component": "Text",
					"text":      "确认提交举报",
				},
				{
					"id":        "spacerBottom",
					"component": "Text",
					"text":      "\n\n\n\n",
				},
			},
		},
	}

	bytes, _ := json.Marshal(updateComponentsMsg)
	return string(bytes)
}

// ExtractForwardID extracts the forward message ID from a CQ:forward code string.
func ExtractForwardID(msg string) string {
	re := regexp.MustCompile(`\[CQ:forward,id=([^,\]]+)\]`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// FormatCQAtText replaces [CQ:at,qq=xxx] in plain text messages with @MemberName for clearer readability.
func FormatCQAtText(msg string, groupID int64) string {
	// 格式化 inline_keyboard 按钮为友好纯文本标记
	reKeyboard := regexp.MustCompile(`\[CQ:inline_keyboard,buttons=([^\]]+)\]`)
	msg = reKeyboard.ReplaceAllStringFunc(msg, func(cq string) string {
		matches := reKeyboard.FindStringSubmatch(cq)
		if len(matches) < 2 {
			return ""
		}
		rawRows := strings.Split(matches[1], "//")
		var sb strings.Builder
		for _, r := range rawRows {
			btns := strings.Split(r, "|")
			var validBtns []string
			for _, b := range btns {
				b = strings.TrimSpace(b)
				if b != "" {
					validBtns = append(validBtns, fmt.Sprintf("[%s]", b))
				}
			}
			if len(validBtns) > 0 {
				sb.WriteString("\n🔘 " + strings.Join(validBtns, " "))
			}
		}
		return sb.String()
	})

	// 格式化 reply 引用消息为友好纯文本标记
	reReply := regexp.MustCompile(`\[CQ:reply,id=([-\d]+)[^\]]*\] ?`)
	msg = reReply.ReplaceAllStringFunc(msg, func(cq string) string {
		matches := reReply.FindStringSubmatch(cq)
		if len(matches) < 2 {
			return ""
		}
		replyIDStr := matches[1]
		var replyInfo *ReplyMsgInfo
		if GlobalQQSender != nil {
			if replyID, err := strconv.ParseInt(replyIDStr, 10, 64); err == nil && replyID != 0 {
				replyInfo, _ = GlobalQQSender.GetReplyMsg(replyID, groupID)
			}
		}
		if replyInfo != nil && replyInfo.SenderName != "" {
			return fmt.Sprintf("「引用 @%s: %s」\n", UnescapeCQ(replyInfo.SenderName), UnescapeCQ(replyInfo.Summary))
		}
		return "「引用消息」\n"
	})

	re := regexp.MustCompile(`\[CQ:at,qq=([^,\]]+)(?:,[^\]]*)?\] ?`)
	msg = re.ReplaceAllStringFunc(msg, func(cq string) string {
		matches := re.FindStringSubmatch(cq)
		if len(matches) < 2 {
			return cq
		}
		qq := strings.TrimSpace(matches[1])
		if qq == "all" {
			return "@全体成员 "
		}
		displayName := qq
		if GlobalQQSender != nil {
			if uid, err := strconv.ParseInt(qq, 10, 64); err == nil && uid != 0 {
				if name := GlobalQQSender.GetGroupMemberName(groupID, uid); name != "" {
					displayName = name
				}
			}
		}
		return fmt.Sprintf("@%s ", displayName)
	})

	msg = UnescapeCQ(msg)
	return ConvertYunhuEmoji(msg)
}

