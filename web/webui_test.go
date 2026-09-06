package web

import (
	"amer/db"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)

	// 初始化临时测试数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_amer.db")
	_ = db.InitSQLite(dbPath)
	t.Cleanup(func() {
		_ = db.CloseSQLite()
	})

	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/webui")
	})
	r.GET("/webui", WebUIHandler)
	r.GET("/api/stats", StatsAPIHandler)
	r.GET("/api/bindings", BindingsAPIHandler)
	r.POST("/api/bindings/bind", BindActionHandler)
	r.POST("/api/bindings/unbind", UnbindActionHandler)
	r.POST("/api/bindings/mode", SetModeActionHandler)
	r.GET("/api/blacklist", BlacklistAPIHandler)
	r.POST("/api/blacklist/add", AddBlacklistHandler)
	r.POST("/api/blacklist/remove", RemoveBlacklistHandler)
	r.GET("/api/system", SystemInfoAPIHandler)
	r.GET("/api/logs", LogsAPIHandler)
	r.GET("/api/logs/stream", LogsStreamHandler)

	return r
}

func TestWebUIEndpoints(t *testing.T) {
	r := setupTestRouter(t)

	// 1. 测试根路径重定向
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect for /, got %d", w.Code)
	}

	// 2. 测试 WebUI 页面访问
	req, _ = http.NewRequest("GET", "/webui", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /webui, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Amer 控制中心") || !strings.Contains(body, "群聊绑定与转发模式") {
		t.Errorf("WebUI html missing key components: %s", body[:min(len(body), 200)])
	}

	// 3. 测试 stats API
	req, _ = http.NewRequest("GET", "/api/stats", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/stats, got %d", w.Code)
	}

	// 4. 测试系统信息 API
	req, _ = http.NewRequest("GET", "/api/system", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/system, got %d", w.Code)
	}

	// 5. 测试群聊绑定 API
	bindPayload := map[string]string{
		"qq_group_id": "123456",
		"yh_group_id": "789012",
		"sync_mode":   "全同步",
	}
	bindJSON, _ := json.Marshal(bindPayload)
	req, _ = http.NewRequest("POST", "/api/bindings/bind", bytes.NewReader(bindJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("bind failed, got %d: %s", w.Code, w.Body.String())
	}

	// 验证绑定列表
	req, _ = http.NewRequest("GET", "/api/bindings", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "123456") || !strings.Contains(w.Body.String(), "789012") {
		t.Errorf("bindings list missing newly bound pair: %s", w.Body.String())
	}

	// 6. 测试修改同步模式 API
	modePayload := map[string]string{
		"qq_group_id": "123456",
		"yh_group_id": "789012",
		"sync_mode":   "QQ到云湖",
	}
	modeJSON, _ := json.Marshal(modePayload)
	req, _ = http.NewRequest("POST", "/api/bindings/mode", bytes.NewReader(modeJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("set mode failed, got %d: %s", w.Code, w.Body.String())
	}

	// 7. 测试解绑 API
	unbindPayload := map[string]string{
		"qq_group_id": "123456",
		"yh_group_id": "789012",
	}
	unbindJSON, _ := json.Marshal(unbindPayload)
	req, _ = http.NewRequest("POST", "/api/bindings/unbind", bytes.NewReader(unbindJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("unbind failed, got %d: %s", w.Code, w.Body.String())
	}

	// 8. 测试黑名单添加与移除
	banPayload := map[string]interface{}{
		"user_id":  "999888",
		"reason":   "单元测试封禁",
		"duration": 3600,
	}
	banJSON, _ := json.Marshal(banPayload)
	req, _ = http.NewRequest("POST", "/api/blacklist/add", bytes.NewReader(banJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("add blacklist failed, got %d: %s", w.Code, w.Body.String())
	}

	// 查询黑名单
	req, _ = http.NewRequest("GET", "/api/blacklist", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "999888") {
		t.Errorf("blacklist list missing user: %s", w.Body.String())
	}

	// 解除黑名单
	unbanPayload := map[string]string{"user_id": "999888"}
	unbanJSON, _ := json.Marshal(unbanPayload)
	req, _ = http.NewRequest("POST", "/api/blacklist/remove", bytes.NewReader(unbanJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("remove blacklist failed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogsEndpoints(t *testing.T) {
	InitLogCollector(100)
	r := setupTestRouter(t)

	// 写入测试日志
	testLogMsg := "Amer 单元测试控制台输出 ABC123XYZ"
	_, _ = GlobalLogCollector.Write([]byte(testLogMsg + "\n"))

	// 1. 测试静态日志快照获取
	req, _ := http.NewRequest("GET", "/api/logs?lines=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/logs, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), testLogMsg) {
		t.Errorf("logs snapshot missing test message: %s", w.Body.String())
	}

	// 2. 测试获取本地网络 IP
	ips := GetLocalIPs()
	t.Logf("Detected local IPs: %v", ips)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
