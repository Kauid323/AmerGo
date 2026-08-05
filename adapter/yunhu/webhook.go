package yunhu

import (
	"amer/model"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func WebhookHandler(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "读取请求体失败"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	log.Printf("[Yunhu Webhook Recv] 收到原始消息: %s", string(bodyBytes))

	var event model.YunhuEvent
	if err := json.Unmarshal(bodyBytes, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": err.Error()})
		return
	}

	go HandleYunhuEvent(event)

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
