package yunhu

import (
	"crypto/md5"
	"encoding/hex"
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

