package web

import (
	"amer/adapter/qq"
	"amer/adapter/yunhu"
	"amer/config"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetLocalIPs returns non-loopback IPv4 addresses of the current machine.
func GetLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				ips = append(ips, ip4.String())
			}
		}
	}
	return ips
}

func StartWebServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// OneBot V11 Reverse WebSocket endpoint
	r.GET("/ws", qq.GlobalOneBotServer.WsHandler)

	// Yunhu Webhook endpoint
	webhookPath := config.AppConfig.YH.Webhook.Path
	if webhookPath == "" {
		webhookPath = "/yh/webhook"
	}
	r.POST(webhookPath, yunhu.WebhookHandler)

	// Report & Monitoring
	r.Any("/report", ReportHandler)
	r.POST("/api/report", APIReportHandler)

	registerWebUIRoutes(r)

	serverPort := config.AppConfig.Server.Port
	serverHost := config.AppConfig.Server.Host
	serverAddr := fmt.Sprintf("%s:%d", serverHost, serverPort)

	webuiPort := config.AppConfig.WebUI.Port
	webuiHost := config.AppConfig.WebUI.Host
	if webuiPort == 0 {
		webuiPort = serverPort
	}
	if webuiHost == "" {
		webuiHost = serverHost
	}

	localIPs := GetLocalIPs()
	log.Println("==================================================")
	log.Println("[WebUI] 控制台面板已就绪:")
	log.Printf("  - 本机本地访问: http://127.0.0.1:%d/webui", webuiPort)
	for _, ip := range localIPs {
		log.Printf("  - 局域网内网访问: http://%s:%d/webui (允许内网其他设备直接访问)", ip, webuiPort)
	}
	if len(localIPs) == 0 {
		log.Printf("  - 局域网内网访问: http://<本机内网IP>:%d/webui", webuiPort)
	}
	log.Printf("[Web Server] OneBot/Webhook 监听地址: http://%s", serverAddr)
	log.Println("==================================================")

	// 如果 WebUI 指定了不同端口，则开启独立监听
	if webuiPort != serverPort {
		go func() {
			webuiApp := gin.Default()
			registerWebUIRoutes(webuiApp)
			webuiAddr := fmt.Sprintf("%s:%d", webuiHost, webuiPort)
			log.Printf("[WebUI Standalone] 独立 WebUI 服务正在监听: %s (开放内网设备访问)", webuiAddr)
			if err := webuiApp.Run(webuiAddr); err != nil {
				log.Printf("[WebUI Error] 独立 WebUI 运行异常: %v", err)
			}
		}()
	}

	if err := r.Run(serverAddr); err != nil {
		log.Fatalf("[Web Server Error] 服务器启动失败: %v", err)
	}
}

func registerWebUIRoutes(r *gin.Engine) {
	// 允许局域网设备跨域调用 API
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

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
	r.GET("/api/group_blacklist", GroupBlacklistAPIHandler)
	r.POST("/api/group_blacklist/add", AddGroupBlacklistHandler)
	r.POST("/api/group_blacklist/remove", RemoveGroupBlacklistHandler)
	r.GET("/api/system", SystemInfoAPIHandler)
	r.GET("/api/logs", LogsAPIHandler)
	r.GET("/api/logs/stream", LogsStreamHandler)
	r.GET("/api/blocked_words", BlockedWordsAPIHandler)
	r.POST("/api/blocked_words/add", AddBlockedWordsHandler)
	r.POST("/api/blocked_words/delete", DeleteBlockedWordHandler)
	r.POST("/api/blocked_words/delete_category", DeleteBlockedCategoryHandler)
	r.POST("/api/config/reload", ReloadConfigAPIHandler)
}
