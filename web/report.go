package web

import (
	"amer/adapter/yunhu"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ReportRequest struct {
	Platform     string `json:"platform" form:"platform"`
	GroupID      string `json:"group_id" form:"groupId"`
	MsgID        string `json:"msg_id" form:"msgId"`
	SenderID     string `json:"sender_id" form:"senderId"`
	SenderName   string `json:"sender_name" form:"senderName"`
	ReporterID   string `json:"reporter_id" form:"reporterId"`
	ReporterName string `json:"reporter_name" form:"reporterName"`
	Reason       string `json:"reason" form:"reason"`
}

// ProcessReport delegates to yunhu.ProcessReportData.
func ProcessReport(req ReportRequest) (bool, string) {
	return yunhu.ProcessReportData(yunhu.ReportData{
		Platform:     req.Platform,
		GroupID:      req.GroupID,
		MsgID:        req.MsgID,
		SenderID:     req.SenderID,
		SenderName:   req.SenderName,
		ReporterID:   req.ReporterID,
		ReporterName: req.ReporterName,
		Reason:       req.Reason,
	})
}

// ReportHandler handles HTTP web page report form (GET & POST /report)
func ReportHandler(c *gin.Context) {
	msgID := c.Query("msgId")
	senderID := c.Query("senderId")
	groupID := c.Query("groupId")

	if c.Request.Method == http.MethodGet {
		if msgID == "" {
			c.String(http.StatusBadRequest, "缺少必要参数 msgId")
			return
		}

		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Amer - 违规消息举报中心</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f0f2f5; display: flex; justify-content: center; align-items: center; min-height: 100vh; padding: 20px; }
        .card { background: white; padding: 30px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.08); max-width: 480px; width: 100%%; }
        h2 { color: #e74c3c; margin-bottom: 15px; font-size: 22px; display: flex; align-items: center; gap: 8px; }
        .info { background: #f8f9fa; border-left: 4px solid #e74c3c; padding: 12px 15px; margin-bottom: 20px; border-radius: 4px; font-size: 14px; color: #555; }
        .form-group { margin-bottom: 18px; text-align: left; }
        label { display: block; margin-bottom: 6px; font-weight: 600; color: #333; font-size: 14px; }
        textarea, input[type="text"] { width: 100%%; padding: 10px 12px; border: 1px solid #dcdfe6; border-radius: 6px; font-size: 14px; transition: border-color 0.2s; }
        textarea:focus, input[type="text"]:focus { outline: none; border-color: #3498db; }
        .btn-submit { width: 100%%; background: #e74c3c; color: white; border: none; padding: 12px; border-radius: 6px; font-size: 16px; font-weight: 600; cursor: pointer; transition: background 0.2s; }
        .btn-submit:hover { background: #c0392b; }
    </style>
</head>
<body>
    <div class="card">
        <h2>🚨 违规消息举报</h2>
        <div class="info">
            <p><strong>消息 ID:</strong> %s</p>
            <p><strong>被举报用户 ID:</strong> %s</p>
            <p><strong>关联群聊:</strong> %s</p>
        </div>
        <form method="POST" action="/report">
            <input type="hidden" name="msgId" value="%s">
            <input type="hidden" name="senderId" value="%s">
            <input type="hidden" name="groupId" value="%s">
            <div class="form-group">
                <label for="reason">举报原因：</label>
                <textarea id="reason" name="reason" rows="4" placeholder="请描述具体的违规行为（例如：垃圾广告、违法不良言论、色情低俗等）" required></textarea>
            </div>
            <div class="form-group">
                <label for="reporterName">您的昵称（选填）：</label>
                <input type="text" id="reporterName" name="reporterName" placeholder="匿名用户">
            </div>
            <button type="submit" class="btn-submit">确认提交举报</button>
        </form>
    </div>
</body>
</html>`, msgID, senderID, groupID, msgID, senderID, groupID)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
		return
	}

	if c.Request.Method == http.MethodPost {
		var req ReportRequest
		if err := c.ShouldBind(&req); err != nil {
			req.MsgID = c.PostForm("msgId")
			req.SenderID = c.PostForm("senderId")
			req.GroupID = c.PostForm("groupId")
			req.Reason = c.PostForm("reason")
			req.ReporterName = c.PostForm("reporterName")
		}

		success, msg := ProcessReport(req)
		c.Header("Content-Type", "text/html; charset=utf-8")
		if success {
			c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>举报提交成功</title><style>body{font-family:sans-serif;background:#f0f2f5;display:flex;justify-content:center;align-items:center;min-height:100vh;}.box{background:white;padding:40px;border-radius:12px;text-align:center;box-shadow:0 4px 16px rgba(0,0,0,0.1);max-width:400px;}</style></head><body><div class="box"><h2 style="color:#2ecc71;">✅ %s</h2><p style="margin-top:15px;color:#666;">系统已收到您的举报信息，感谢您维护社区秩序！</p></div></body></html>`, msg))
		} else {
			c.String(http.StatusBadRequest, fmt.Sprintf(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>提交失败</title></head><body><div style="text-align:center;padding:50px;"><h2 style="color:#e74c3c;">❌ 提交失败</h2><p>%s</p></div></body></html>`, msg))
		}
	}
}

// APIReportHandler handles JSON API report submission (/api/report)
func APIReportHandler(c *gin.Context) {
	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "无效的 JSON 请求数据"})
		return
	}

	success, msg := ProcessReport(req)
	if success {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "msg": msg})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": msg})
	}
}
