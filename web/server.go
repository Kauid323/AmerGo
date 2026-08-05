package web

import (
	"amer/adapter/qq"
	"amer/adapter/yunhu"
	"amer/config"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
)

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
	r.GET("/webui", WebUIHandler)
	r.GET("/api/stats", StatsAPIHandler)

	addr := fmt.Sprintf("%s:%d", config.AppConfig.Server.Host, config.AppConfig.Server.Port)
	log.Printf("[Web Server] HTTP/WS 服务正在启动，监听地址: %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("[Web Server Error] 服务器启动失败: %v", err)
	}
}
