package main

import (
	"amer/adapter/message"
	"amer/adapter/qq"
	"amer/adapter/yunhu"
	"amer/config"
	"amer/db"
	"amer/web"
	"log"
	"os"
)

func main() {
	web.InitLogCollector(2000)

	log.Println("==========================================")
	log.Println("        Amer Golang 机器人启动中...        ")
	log.Println("==========================================")

	configPath := "config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("配置文件 %s 不存在，请检查！", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置文件出错: %v", err)
	}

	if cfg.TempFolder != "" {
		_ = os.MkdirAll(cfg.TempFolder, 0755)
	}

	if err := db.InitSQLite(cfg.SQLite.DBPath); err != nil {
		log.Fatalf("初始化 SQLite 数据库失败: %v", err)
	}
	log.Printf("[SQLite] 成功加载数据库文件: %s", cfg.SQLite.DBPath)

	if err := db.InitRedis(cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.DB, cfg.Redis.Password); err != nil {
		log.Fatalf("初始化 Redis 数据库失败: %v", err)
	}

	// Register senders for cross-platform messaging
	message.RegisterQQSender(qq.GlobalOneBotServer)
	message.RegisterYHSender(yunhu.YHClient)

	// Check Yunhu message reception mode
	log.Printf("[Yunhu] 当前接收消息模式: %s", cfg.YH.Mode)
	if cfg.YH.Mode == "websocket" {
		log.Printf("[Yunhu] 启动 WebSocket 订阅模式 (wss://ws.jwzhd.com/subscribe?token=...)")
		yunhu.StartWebSocketSubscriber()
	} else {
		log.Printf("[Yunhu] 使用 Webhook 模式接收消息，监听路径: %s", cfg.YH.Webhook.Path)
	}

	// Start Web and OneBot reverse WS server
	web.StartWebServer()
}
