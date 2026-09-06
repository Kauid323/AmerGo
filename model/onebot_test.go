package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMarkdownSegment(t *testing.T) {
	input := "[](%7B%22version%22%3A2%7D)\n![#150px #84px](https://qqbot.ugcimg.cn/102084850/52444d3a9350ba7e8da5f1901a2b95c40789d398/beb95747df1bd19f711057e1b3b52ae5)\n\n[@Amer(QQ云湖互通机器人)](mqqapi://markdown/mention?at_type=1&at_tinyid=3218936228) 欢迎新人~!\n\n> 群管理员可以[开关入群欢迎](mqqapi://aio/inlinecmd?command=%E5%85%B3%E9%97%AD%E5%8A%9F%E8%83%BD%20515&enter=false&reply=false)哦"
	result := ParseMarkdownSegment(input)

	if !strings.Contains(result, "[CQ:image,url=https://qqbot.ugcimg.cn/102084850/52444d3a9350ba7e8da5f1901a2b95c40789d398/beb95747df1bd19f711057e1b3b52ae5]") {
		t.Errorf("expected parsed image CQ code, got: %s", result)
	}

	if !strings.Contains(result, "@Amer(QQ云湖互通机器人)") {
		t.Errorf("expected mention conversion, got: %s", result)
	}

	if !strings.Contains(result, "[开关入群欢迎]") {
		t.Errorf("expected command button conversion, got: %s", result)
	}

	if strings.Contains(result, "version") {
		t.Errorf("version metadata should be removed, got: %s", result)
	}
}

func TestParseInlineKeyboardSegment(t *testing.T) {
	data := map[string]interface{}{
		"bot_appid": "102084850",
		"rows": []interface{}{
			map[string]interface{}{
				"buttons": []interface{}{
					map[string]interface{}{"label": "抽老婆"},
					map[string]interface{}{"label": "萌属性"},
					map[string]interface{}{"label": "打卡"},
				},
			},
		},
	}

	result := ParseInlineKeyboardSegment(data)
	expected := "[CQ:inline_keyboard,buttons=抽老婆|萌属性|打卡]"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetRawMessageWithRichSegments(t *testing.T) {
	rawJSON := `{
		"raw_message": "[CQ:at,qq=3218936228][CQ:markdown][CQ:inline_keyboard]",
		"message": [
			{"type": "at", "data": {"qq": "3218936228"}},
			{"type": "markdown", "data": {"content": "欢迎光临"}},
			{"type": "inline_keyboard", "data": {"rows": [{"buttons": [{"label": "签到"}]}]}}
		]
	}`

	var event OneBotEvent
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	rawMsg := event.GetRawMessage()
	if !strings.Contains(rawMsg, "[CQ:at,qq=3218936228]") {
		t.Errorf("missing at in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "欢迎光临") {
		t.Errorf("missing markdown content in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "[CQ:inline_keyboard,buttons=签到]") {
		t.Errorf("missing keyboard in rawMsg: %s", rawMsg)
	}
}

func TestGetRawMessageWithJSONCard(t *testing.T) {
	rawJSON := `{
		"raw_message": "[CQ:json,data={\"app\":\"com.tencent.miniapp_01\",\"meta\":{\"detail_1\":{\"title\":\"哔哩哔哩\",\"desc\":\"或许他真的是好 汉呢\",\"qqdocurl\":\"https://b23.tv/aogcC5V\"}}}]",
		"message": [
			{
				"type": "json",
				"data": {
					"data": "{\"app\":\"com.tencent.miniapp_01\",\"prompt\":\"[QQ小程序]或许他真的是好 汉呢\",\"meta\":{\"detail_1\":{\"title\":\"哔哩哔哩\",\"desc\":\"或许他真的是好 汉呢\",\"qqdocurl\":\"https://b23.tv/aogcC5V\",\"preview\":\"https://pic.ugcimg.cn/test.jpg\"}}}"
				}
			}
		]
	}`

	var event OneBotEvent
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	rawMsg := event.GetRawMessage()
	if !strings.HasPrefix(rawMsg, "[CQ:card,") {
		t.Errorf("expected parsed CQ:card, got: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "title=哔哩哔哩") {
		t.Errorf("missing title in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "tag=QQ小程序") {
		t.Errorf("missing tag in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "url=https://b23.tv/aogcC5V") {
		t.Errorf("missing url in rawMsg: %s", rawMsg)
	}
}

func TestGetRawMessageWithFileSegment(t *testing.T) {
	rawJSON := `{
		"time": 1788656123,
		"self_id": 3218936228,
		"post_type": "message",
		"message_type": "group",
		"sub_type": "normal",
		"message_id": 1710849642,
		"group_id": 929814964,
		"user_id": 171989292,
		"message": [
			{
				"type": "file",
				"data": {
					"file": "QQ20260906-085033.mp4",
					"file_id": "/0099084d-ea23-4497-b796-83677e49acd1",
					"file_size": 69528934,
					"name": "QQ20260906-085033.mp4",
					"size": 69528934,
					"url": "https://gzc-download.ftn.qq.com/ftn_handler/test"
				}
			}
		],
		"raw_message": "[CQ:file,name=QQ20260906-085033.mp4]"
	}`

	var event OneBotEvent
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	rawMsg := event.GetRawMessage()
	if !strings.HasPrefix(rawMsg, "[CQ:file,") {
		t.Errorf("expected [CQ:file, got: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "name=QQ20260906-085033.mp4") {
		t.Errorf("missing name in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "size=69528934") {
		t.Errorf("missing size in rawMsg: %s", rawMsg)
	}
	if !strings.Contains(rawMsg, "url=https://gzc-download.ftn.qq.com/ftn_handler/test") {
		t.Errorf("missing url in rawMsg: %s", rawMsg)
	}
}
