package message

import (
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

	// 先用缓存 key 预存或通过 ProcessCQImage 验证
	imageCache.Store("test_image_file.jpg", mockURL)
	cqMsg := "[CQ:image,file=test_image_file.jpg,url=http://example.com/test.jpg]"
	got := CQToHTML(cqMsg)
	expected := `<br><img src="https://chat-img.jwznb.com/c91bb351c5fc283dfd9c95d0ec5d6c88.jpg" style="max-width: 100%; margin: 5px 0; border-radius: 4px;"><br>`
	if got != expected {
		t.Errorf("CQToHTML image = %v, want %v", got, expected)
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



