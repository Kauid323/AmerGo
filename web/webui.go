package web

import (
	"amer/db"
	"net/http"

	"github.com/gin-gonic/gin"
)

func StatsAPIHandler(c *gin.Context) {
	qqKeys, _ := db.RDB.Keys(db.Ctx, "QQ:*:QQ:*").Result()
	yhKeys, _ := db.RDB.Keys(db.Ctx, "YH:*:YH:*").Result()

	qqCount := 0
	for _, key := range qqKeys {
		n, _ := db.RDB.LLen(db.Ctx, key).Result()
		qqCount += int(n)
	}

	yhCount := 0
	for _, key := range yhKeys {
		n, _ := db.RDB.LLen(db.Ctx, key).Result()
		yhCount += int(n)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"qq_messages":    qqCount,
			"yh_messages":    yhCount,
			"total_messages": qqCount + yhCount,
		},
	})
}

func WebUIHandler(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="zh">
<head>
    <meta charset="UTF-8">
    <title>Amer 控制面板</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 40px; }
        .card { background: #1e293b; border-radius: 12px; padding: 24px; box-shadow: 0 10px 25px rgba(0,0,0,0.3); max-width: 800px; margin: 0 auto; }
        h1 { color: #38bdf8; font-size: 28px; margin-top: 0; }
        .badge { display: inline-block; padding: 4px 12px; background: #10b981; color: white; border-radius: 20px; font-weight: bold; }
        .stats-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-top: 24px; }
        .stat-item { background: #334155; padding: 16px; border-radius: 8px; text-align: center; }
        .stat-value { font-size: 24px; font-weight: bold; color: #f43f5e; margin-top: 8px; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Amer 机器人控制中心 <span class="badge">Golang 运行中</span></h1>
        <p>基于 Go 语言的 QQ 与云湖跨平台高并发消息同步系统。</p>
        <div class="stats-grid">
            <div class="stat-item">
                <div>QQ 消息量</div>
                <div class="stat-value" id="qq-count">-</div>
            </div>
            <div class="stat-item">
                <div>云湖消息量</div>
                <div class="stat-value" id="yh-count">-</div>
            </div>
            <div class="stat-item">
                <div>总同步消息</div>
                <div class="stat-value" id="total-count">-</div>
            </div>
        </div>
    </div>
    <script>
        fetch('/api/stats').then(res => res.json()).then(data => {
            if(data.status === 'success') {
                document.getElementById('qq-count').innerText = data.data.qq_messages;
                document.getElementById('yh-count').innerText = data.data.yh_messages;
                document.getElementById('total-count').innerText = data.data.total_messages;
            }
        });
    </script>
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
