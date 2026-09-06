package yunhu

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"amer/config"
)

func TestUploadImage(t *testing.T) {
	fakeData := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46} // Fake JPEG header
	hash := md5.Sum(fakeData)
	expectedHash := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token != "test_token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 1, "data": {"imageKey": "mock_key"}, "msg": "success"}`))
	}))
	defer server.Close()

	config.AppConfig.YH.Token = "test_token"
	ImageUploadBaseURL = server.URL
	defer func() {
		ImageUploadBaseURL = "https://chat-go.jwzhd.com"
	}()

	client := &Client{httpClient: server.Client()}
	url, err := client.UploadImage(fakeData, "test.jpg")
	if err != nil {
		t.Fatalf("UploadImage failed: %v", err)
	}

	expectedURL := fmt.Sprintf("https://chat-img.jwznb.com/%s.jpg", expectedHash)
	if url != expectedURL {
		t.Errorf("UploadImage() = %s, want %s", url, expectedURL)
	}
}

func TestFetchGroupInfoAndCache(t *testing.T) {
	// 1. 预先测试缓存直接命中
	yhGroupNameCache.Store("cached_group_123", "测试云湖群A")
	client := &Client{}
	name := client.GetGroupName("cached_group_123")
	if name != "测试云湖群A" {
		t.Errorf("GetGroupName from cache = %v, want 测试云湖群A", name)
	}

	// 2. 空 groupID
	if empty := client.GetGroupName(""); empty != "" {
		t.Errorf("GetGroupName('') = %v, want empty", empty)
	}

	// 3. Mock 服务测试 FetchGroupInfo
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["groupId"] == "mock_grp_999" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code": 1, "data": {"group": {"groupId": "mock_grp_999", "name": "全员测试群"}}, "msg": "success"}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	oldURL := GroupInfoBaseURL
	GroupInfoBaseURL = server.URL
	defer func() {
		GroupInfoBaseURL = oldURL
	}()

	mockClient := &Client{httpClient: server.Client()}
	info, err := mockClient.FetchGroupInfo("mock_grp_999")
	if err != nil {
		t.Fatalf("FetchGroupInfo failed: %v", err)
	}
	if info.Name != "全员测试群" {
		t.Errorf("FetchGroupInfo name = %s, want 全员测试群", info.Name)
	}

	// 4. 测试通过 GetGroupName 自动拉取
	fetchedName := mockClient.GetGroupName("mock_grp_999")
	if fetchedName != "全员测试群" {
		t.Errorf("GetGroupName fetchedName = %s, want 全员测试群", fetchedName)
	}
}

func TestSendExtractsMessageInfoMsgID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":1,"data":{"messageInfo":{"msgId":"bd46cef37ef04a2893fa10ee6b9fd7b5","recvId":"180845683","recvType":"group"}},"msg":"success"}`))
	}))
	defer server.Close()

	oldURL := BotSendBaseURL
	BotSendBaseURL = server.URL
	defer func() {
		BotSendBaseURL = oldURL
	}()

	config.AppConfig.YH.Token = "test_token"
	client := &Client{httpClient: server.Client()}
	msgID, err := client.Send("180845683", "group", "html", "测试内容")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if msgID != "bd46cef37ef04a2893fa10ee6b9fd7b5" {
		t.Errorf("Send msgID = %s, want bd46cef37ef04a2893fa10ee6b9fd7b5", msgID)
	}
}



