package yunhu

import (
	"amer/config"
	"amer/model"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func StartWebSocketSubscriber() {
	token := config.AppConfig.YH.Token
	if token == "" {
		log.Printf("[Yunhu WSS Error] 机器人 token 未配置，无法启动 WebSocket 订阅模式")
		return
	}

	baseURL := config.AppConfig.YH.WebSocket.URL
	if baseURL == "" {
		baseURL = "wss://ws.jwzhd.com/subscribe?token="
	}

	wsURL := fmt.Sprintf("%s%s", baseURL, token)
	log.Printf("[Yunhu WSS] 正在尝试连接订阅地址: %s", wsURL)

	go func() {
		for {
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				if resp != nil {
					log.Printf("[Yunhu WSS Error] 连接失败，状态码: %d, 错误: %v", resp.StatusCode, err)
				} else {
					log.Printf("[Yunhu WSS Error] 连接失败: %v", err)
				}
				log.Printf("[Yunhu WSS] 5秒后将进行自动重连...")
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("[Yunhu WSS Success] 成功建立 WebSocket 消息订阅长连接！")

			for {
				_, messageBytes, err := conn.ReadMessage()
				if err != nil {
					log.Printf("[Yunhu WSS Disconnect] 链接断开: %v", err)
					conn.Close()
					break
				}

				log.Printf("[Yunhu WSS Recv] 收到原始消息: %s", string(messageBytes))

				var event model.YunhuEvent
				if err := json.Unmarshal(messageBytes, &event); err != nil {
					log.Printf("[Yunhu WSS Parse Error] 解析消息失败: %v, 内容: %s", err, string(messageBytes))
					continue
				}

				go HandleYunhuEvent(event)
			}

			log.Printf("[Yunhu WSS] 3秒后尝试重新连接...")
			time.Sleep(3 * time.Second)
		}
	}()
}
