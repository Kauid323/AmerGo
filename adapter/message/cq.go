package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func imageURLToBase64(imgURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
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

	base64Str := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64Str), nil
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

// CQToHTML converts OneBot CQ codes in a message string to HTML elements for display in Yunhu HTML messages.
func CQToHTML(msg string) string {
	re := regexp.MustCompile(`\[CQ:([a-zA-Z0-9_-]+)(?:,([^\]]*))?\]`)
	return re.ReplaceAllStringFunc(msg, func(cq string) string {
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
			if imgURL == "" {
				fileVal := params["file"]
				if strings.HasPrefix(fileVal, "http://") || strings.HasPrefix(fileVal, "https://") {
					imgURL = fileVal
				}
			}
			if imgURL != "" {
				imgURL = html.UnescapeString(imgURL)
				src := imgURL
				if base64Src, err := imageURLToBase64(imgURL); err == nil && base64Src != "" {
					src = base64Src
				}
				return fmt.Sprintf(`<br><img src="%s" style="max-width: 100%%; margin: 5px 0; border-radius: 4px;"><br>`, src)
			}
			return "[图片]"

		case "at":
			qq := params["qq"]
			if qq == "all" {
				return "@全体成员"
			}
			return fmt.Sprintf("<b>@%s</b> ", qq)

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
				return fmt.Sprintf(`<br><video src="%s" controls style="max-width: 100%%; margin: 5px 0;"></video><br>`, videoURL)
			}
			return "[视频消息]"

		case "reply":
			return ""

		default:
			return fmt.Sprintf("[%s消息]", cqType)
		}
	})
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
