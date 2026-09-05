package message

import (
	"amer/config"
	"strings"
	"testing"
)

type mockQQSender struct{}

func (m *mockQQSender) SendGroupMsg(groupID int64, message string) error            { return nil }
func (m *mockQQSender) SendGroupForwardMsg(groupID int64, nodes []interface{}) error { return nil }
func (m *mockQQSender) GetGroupName(groupID int64) string                           { return "测试群" }
func (m *mockQQSender) GetSelfInfo() (int64, string)                                { return 3218936228, "Amer" }
func (m *mockQQSender) GetGroupMemberName(groupID int64, userID int64) string {
	if userID == 3218936228 {
		return "Amer(QQ云湖互通机器人..)"
	}
	if userID == 241638640 {
		return "那狗吧"
	}
	return ""
}
func (m *mockQQSender) GetReplyMsg(messageID int64, groupID int64) (*ReplyMsgInfo, error) {
	if messageID == 80174758 {
		return &ReplyMsgInfo{
			SenderName: "那狗吧",
			SenderUID:  241638640,
			RawText:    "我周五先走了",
			Summary:    "我周五先走了",
		}, nil
	}
	if messageID == 80174759 {
		return &ReplyMsgInfo{
			SenderName: "Amer(QQ云湖互通机器人)",
			SenderUID:  3218936228,
			RawText:    "&#91;云湖群-857744874&#93; 那狗吧(8516939): html按钮没有回调，别想着用",
			Summary:    "&#91;云湖群-857744874&#93; 那狗吧(8516939): html按钮没有回调，别想着用",
		}, nil
	}
	return nil, nil
}

func TestCQToHTML(t *testing.T) {
	input := "[CQ:at,qq=123456]"
	got := CQToHTML(input)
	expected := "<b>@123456</b> "
	if got != expected {
		t.Errorf("CQToHTML() = %v, want %v", got, expected)
	}
}

func TestCQToHTMLWithGroupMember(t *testing.T) {
	RegisterQQSender(&mockQQSender{})
	defer func() {
		GlobalQQSender = nil
	}()

	input := "[CQ:at,qq=3218936228] 111"
	got := CQToHTMLWithGroup(input, 1013637348)
	expected := "<b>@Amer(QQ云湖互通机器人..)</b> 111"
	if got != expected {
		t.Errorf("CQToHTMLWithGroup() = %v, want %v", got, expected)
	}

	text := FormatCQAtText("[CQ:at,qq=241638640] 收到", 1013637348)
	expectedText := "@那狗吧 收到"
	if text != expectedText {
		t.Errorf("FormatCQAtText() = %v, want %v", text, expectedText)
	}

	allText := FormatCQAtText("[CQ:at,qq=all] 开会", 1013637348)
	expectedAll := "@全体成员 开会"
	if allText != expectedAll {
		t.Errorf("FormatCQAtText() = %v, want %v", allText, expectedAll)
	}
}

type mockYHSender struct {
	uploadedURL string
}

func (m *mockYHSender) Send(recvID, recvType, contentType, content string) (string, error) {
	return "mock_msg_id", nil
}
func (m *mockYHSender) SetBoard(recvID, recvType, content string) error {
	return nil
}
func (m *mockYHSender) GetGroupName(groupID string) string {
	return "测试云湖群"
}
func (m *mockYHSender) UploadImage(imgData []byte, filename string) (string, error) {
	return m.uploadedURL, nil
}

func TestCQImageWithYunhuUpload(t *testing.T) {
	mockURL := "https://chat-img.jwznb.com/c91bb351c5fc283dfd9c95d0ec5d6c88.jpg"
	RegisterYHSender(&mockYHSender{uploadedURL: mockURL})
	defer func() {
		GlobalYHSender = nil
	}()

	// 1. 测试从缓存读取带分辨率尺寸的对象 (例如 1920x1080 -> 300x169)
	imageCache.Store("test_scaled_image.jpg", imageCacheItem{url: mockURL, width: 1920, height: 1080})
	cqMsg := "[CQ:image,file=test_scaled_image.jpg,url=http://example.com/test.jpg]"
	got := CQToHTML(cqMsg)
	expected := `<img src="https://chat-img.jwznb.com/c91bb351c5fc283dfd9c95d0ec5d6c88.jpg" width="300" height="169" style="display:block;width:300px;height:169px;max-width:100%;object-fit:contain;border-radius:4px;margin:2px 0;">`
	if got != expected {
		t.Errorf("CQToHTML image with dimensions = %v, want %v", got, expected)
	}

	// 2. 测试降级/无尺寸时的兼容性
	imageCache.Store("test_image_file.jpg", mockURL)
	cqMsg2 := "[CQ:image,file=test_image_file.jpg,url=http://example.com/test.jpg]"
	got2 := CQToHTML(cqMsg2)
	expected2 := `<img src="https://chat-img.jwznb.com/c91bb351c5fc283dfd9c95d0ec5d6c88.jpg" style="display:block;max-width:300px;max-height:320px;border-radius:4px;margin:2px 0;object-fit:contain;">`
	if got2 != expected2 {
		t.Errorf("CQToHTML image fallback = %v, want %v", got2, expected2)
	}
}

func TestCompressHTML(t *testing.T) {
	input := `<div>   <p>Hello</p>   <br><br><br>   <span>World</span>   </div>`
	got := CompressHTML(input)
	if strings.Contains(got, ">   <") || strings.Contains(got, "<br><br>") {
		t.Errorf("CompressHTML failed to compact whitespace or multiple br: %s", got)
	}
}

func TestCalculateDisplayDimensions(t *testing.T) {
	config.AppConfig.Image.Scale = 0
	config.AppConfig.Image.MaxWidth = 300
	config.AppConfig.Image.MaxHeight = 320

	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"16:9 大图等比例缩小", 1920, 1080, 300, 169},
		{"竖向长图等比例缩小", 800, 1200, 213, 320},
		{"正方形大图等比例缩小", 1000, 1000, 300, 300},
		{"小图表情包不放大", 120, 120, 120, 120},
		{"临界边界尺寸保持不变", 300, 320, 300, 320},
		{"异常零尺寸", 0, 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := calculateDisplayDimensions(tc.w, tc.h)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Errorf("calculateDisplayDimensions(%d, %d) = (%d, %d), want (%d, %d)",
					tc.w, tc.h, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestCalculateDisplayDimensionsWithCustomConfig(t *testing.T) {
	// 1. 自定义缩小比例为 0.5 (50%)
	config.AppConfig.Image.Scale = 0.5
	gotW, gotH := calculateDisplayDimensions(1000, 800)
	if gotW != 500 || gotH != 400 {
		t.Errorf("Scale 0.5: got (%d, %d), want (500, 400)", gotW, gotH)
	}

	// 2. 自定义缩小比例百分比写法 50 (转换为 0.5)
	config.AppConfig.Image.Scale = 50
	gotW, gotH = calculateDisplayDimensions(1000, 800)
	if gotW != 500 || gotH != 400 {
		t.Errorf("Scale 50: got (%d, %d), want (500, 400)", gotW, gotH)
	}

	// 3. 自定义最大宽高 (max_width: 500, max_height: 400)
	config.AppConfig.Image.Scale = 0
	config.AppConfig.Image.MaxWidth = 500
	config.AppConfig.Image.MaxHeight = 400
	gotW, gotH = calculateDisplayDimensions(1000, 800)
	if gotW != 500 || gotH != 400 {
		t.Errorf("Custom Max: got (%d, %d), want (500, 400)", gotW, gotH)
	}

	// 恢复默认
	config.AppConfig.Image.Scale = 0
	config.AppConfig.Image.MaxWidth = 300
	config.AppConfig.Image.MaxHeight = 320
}

func TestGetImageDimensionsWebP(t *testing.T) {
	// 测试 VP8X 头部解析: canvas 800x600 -> width-1=799 (0x031f), height-1=599 (0x0257)
	webpData := make([]byte, 30)
	copy(webpData[0:4], "RIFF")
	copy(webpData[8:12], "WEBP")
	copy(webpData[12:16], "VP8X")
	webpData[24] = 0x1f
	webpData[25] = 0x03
	webpData[26] = 0x00
	webpData[27] = 0x57
	webpData[28] = 0x02
	webpData[29] = 0x00

	w, h := getImageDimensions(webpData)
	if w != 800 || h != 600 {
		t.Errorf("getImageDimensions(webp) = (%d, %d), want (800, 600)", w, h)
	}
}

func TestInlineKeyboardRendering(t *testing.T) {
	cqMsg := "[CQ:inline_keyboard,buttons=抽老婆|萌属性|打卡]"
	html := CQToHTML(cqMsg)
	if !strings.Contains(html, "抽老婆") || !strings.Contains(html, "萌属性") || !strings.Contains(html, "打卡") {
		t.Errorf("expected buttons in html, got: %s", html)
	}

	text := FormatCQAtText(cqMsg, 0)
	if !strings.Contains(text, "[抽老婆]") || !strings.Contains(text, "[萌属性]") || !strings.Contains(text, "[打卡]") {
		t.Errorf("expected buttons in text, got: %s", text)
	}
}

func TestReplyMessageParsing(t *testing.T) {
	RegisterQQSender(&mockQQSender{})
	defer func() {
		GlobalQQSender = nil
	}()

	rawMsg := "[CQ:reply,id=80174758]我周末选择跑路"

	// 1. 测试 HTML 引用卡片格式化
	htmlGot := CQToHTMLWithGroup(rawMsg, 721141253)
	if !strings.Contains(htmlGot, "@那狗吧") || !strings.Contains(htmlGot, "我周五先走了") || !strings.Contains(htmlGot, "我周末选择跑路") {
		t.Errorf("CQToHTMLWithGroup reply = %v", htmlGot)
	}

	// 2. 测试纯文本引用格式化
	textGot := FormatCQAtText(rawMsg, 721141253)
	if !strings.Contains(textGot, "「引用 @那狗吧: 我周五先走了」") || !strings.Contains(textGot, "我周末选择跑路") {
		t.Errorf("FormatCQAtText reply = %v", textGot)
	}

	// 3. 测试未知引用消息降级
	unknownMsg := "[CQ:reply,id=99999999]收到"
	textUnknown := FormatCQAtText(unknownMsg, 721141253)
	if !strings.Contains(textUnknown, "「引用消息」") || !strings.Contains(textUnknown, "收到") {
		t.Errorf("FormatCQAtText unknown reply = %v", textUnknown)
	}
}

func TestUnescapeCQAndReplyWithEntities(t *testing.T) {
	RegisterQQSender(&mockQQSender{})
	defer func() {
		GlobalQQSender = nil
	}()

	// 1. 测试引用消息中含有 &#91; 和 &#93; 转义字符时，正确还原为 [ 和 ]，绝不出现 &amp;#91;
	rawReply := "[CQ:reply,id=80174759]6666"
	htmlGot := CQToHTMLWithGroup(rawReply, 721141253)
	if strings.Contains(htmlGot, "&amp;#91;") || strings.Contains(htmlGot, "&#91;") {
		t.Errorf("CQToHTMLWithGroup contains escaped entity: %v", htmlGot)
	}
	if !strings.Contains(htmlGot, "[云湖群-857744874]") {
		t.Errorf("expected [云湖群-857744874] in html, got: %v", htmlGot)
	}

	textGot := FormatCQAtText(rawReply, 721141253)
	if strings.Contains(textGot, "&#91;") || strings.Contains(textGot, "&#93;") {
		t.Errorf("FormatCQAtText contains raw entity: %v", textGot)
	}
	if !strings.Contains(textGot, "[云湖群-857744874]") {
		t.Errorf("expected [云湖群-857744874] in text, got: %v", textGot)
	}

	// 2. 测试正文中含有 &#91; 和 &#93; 的解码
	bodyRaw := "&#91;测试群&#93; 消息内容"
	bodyHTML := CQToHTMLWithGroup(bodyRaw, 0)
	if strings.Contains(bodyHTML, "&#91;") {
		t.Errorf("bodyHTML contains &#91;: %v", bodyHTML)
	}
	if !strings.Contains(bodyHTML, "[测试群]") {
		t.Errorf("expected [测试群], got: %v", bodyHTML)
	}
}






