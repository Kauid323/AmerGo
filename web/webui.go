package web

import (
	"amer/adapter/message"
	"amer/config"
	"amer/db"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

// StatsAPIHandler returns real-time system stats and message counters.
func StatsAPIHandler(c *gin.Context) {
	qqCount := 0
	yhCount := 0

	if db.RDB != nil {
		qqKeys, _ := db.RDB.Keys(db.Ctx, "QQ:*:QQ:*").Result()
		yhKeys, _ := db.RDB.Keys(db.Ctx, "YH:*:YH:*").Result()

		for _, key := range qqKeys {
			n, _ := db.RDB.LLen(db.Ctx, key).Result()
			qqCount += int(n)
		}
		for _, key := range yhKeys {
			n, _ := db.RDB.LLen(db.Ctx, key).Result()
			yhCount += int(n)
		}
	}

	bindings := db.GetAllBindings()
	blacklist := db.GetBlacklist()
	uptimeSeconds := int64(time.Since(startTime).Seconds())

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"qq_messages":     qqCount,
			"yh_messages":     yhCount,
			"total_messages":  qqCount + yhCount,
			"total_bindings":  len(bindings),
			"total_blacklist": len(blacklist),
			"uptime_seconds":  uptimeSeconds,
		},
	})
}

// BindingsAPIHandler returns all group bindings enriched with group names.
func BindingsAPIHandler(c *gin.Context) {
	rawBindings := db.GetAllBindings()

	type EnrichedBinding struct {
		QQGroupID   string `json:"qq_group_id"`
		QQGroupName string `json:"qq_group_name"`
		YHGroupID   string `json:"yh_group_id"`
		YHGroupName string `json:"yh_group_name"`
		QQToYHSync  bool   `json:"qq_to_yh_sync"`
		YHToQQSync  bool   `json:"yh_to_qq_sync"`
		SyncMode    string `json:"sync_mode"`
	}

	var list []EnrichedBinding
	for _, b := range rawBindings {
		qqName := b.QQGroupID
		if message.GlobalQQSender != nil {
			var qid int64
			fmt.Sscanf(b.QQGroupID, "%d", &qid)
			if qid != 0 {
				if name := message.GlobalQQSender.GetGroupName(qid); name != "" {
					qqName = name
				}
			}
		}

		yhName := b.YHGroupID
		if message.GlobalYHSender != nil {
			if name := message.GlobalYHSender.GetGroupName(b.YHGroupID); name != "" {
				yhName = name
			}
		}

		list = append(list, EnrichedBinding{
			QQGroupID:   b.QQGroupID,
			QQGroupName: qqName,
			YHGroupID:   b.YHGroupID,
			YHGroupName: yhName,
			QQToYHSync:  b.QQToYHSync,
			YHToQQSync:  b.YHToQQSync,
			SyncMode:    b.SyncMode,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   list,
	})
}

type BindRequest struct {
	QQGroupID string `json:"qq_group_id"`
	YHGroupID string `json:"yh_group_id"`
	SyncMode  string `json:"sync_mode"`
}

// BindActionHandler binds a QQ group with a Yunhu group.
func BindActionHandler(c *gin.Context) {
	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "QQ群号和云湖群号不能为空"})
		return
	}

	res := db.Bind("QQ", "YH", req.QQGroupID, req.YHGroupID)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	if req.SyncMode != "" && req.SyncMode != "全同步" {
		validModes := map[string]map[string]bool{
			"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
			"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
			"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
			"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
		}
		if syncData, ok := validModes[req.SyncMode]; ok {
			_ = db.SetSync("QQ", "YH", req.QQGroupID, req.YHGroupID, syncData)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "绑定成功"})
}

// UnbindActionHandler unbinds a specific QQ group and Yunhu group pair.
func UnbindActionHandler(c *gin.Context) {
	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "QQ群号和云湖群号不能为空"})
		return
	}

	res := db.Unbind("QQ", "YH", req.QQGroupID, req.YHGroupID)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "解绑成功"})
}

type SetModeRequest struct {
	QQGroupID string `json:"qq_group_id"`
	YHGroupID string `json:"yh_group_id"`
	SyncMode  string `json:"sync_mode"`
}

// SetModeActionHandler sets the forwarding sync mode for a bound pair.
func SetModeActionHandler(c *gin.Context) {
	var req SetModeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.QQGroupID == "" || req.YHGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "参数不完整"})
		return
	}

	validModes := map[string]map[string]bool{
		"全同步":   {"QQ_TO_YH": true, "YH_TO_QQ": true},
		"QQ到云湖": {"QQ_TO_YH": true, "YH_TO_QQ": false},
		"云湖到QQ": {"QQ_TO_YH": false, "YH_TO_QQ": true},
		"停止":     {"QQ_TO_YH": false, "YH_TO_QQ": false},
	}

	syncData, ok := validModes[req.SyncMode]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "无效的同步模式"})
		return
	}

	res := db.SetSync("QQ", "YH", req.QQGroupID, req.YHGroupID, syncData)
	if res.Status != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": res.Msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "同步模式已更新"})
}

// BlacklistAPIHandler returns the list of banned users.
func BlacklistAPIHandler(c *gin.Context) {
	list := db.GetBlacklist()
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   list,
	})
}

type BlacklistRequest struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

// AddBlacklistHandler adds a user to the blacklist.
func AddBlacklistHandler(c *gin.Context) {
	var req BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "用户ID不能为空"})
		return
	}
	if req.Reason == "" {
		req.Reason = "管理员通过 WebUI 手动拉黑"
	}

	err := db.AddToBlacklist(req.UserID, req.Reason, req.Duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "添加黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已将用户加入黑名单"})
}

// RemoveBlacklistHandler removes a user from the blacklist.
func RemoveBlacklistHandler(c *gin.Context) {
	var req BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "用户ID不能为空"})
		return
	}

	err := db.RemoveFromBlacklist(req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "解除黑名单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "msg": "已成功将用户移出黑名单"})
}

// SystemInfoAPIHandler returns overall system and configuration parameters.
func SystemInfoAPIHandler(c *gin.Context) {
	blockedWordGroups := make(map[string]int)
	totalBlockedWords := 0
	for category, words := range config.AppConfig.BlockedWords {
		blockedWordGroups[category] = len(words)
		totalBlockedWords += len(words)
	}

	webuiPort := config.AppConfig.WebUI.Port
	if webuiPort == 0 {
		webuiPort = config.AppConfig.Server.Port
	}
	webuiHost := config.AppConfig.WebUI.Host
	if webuiHost == "" {
		webuiHost = config.AppConfig.Server.Host
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"port":         config.AppConfig.Server.Port,
			"host":         config.AppConfig.Server.Host,
			"webui_port":   webuiPort,
			"webui_host":   webuiHost,
			"lan_ips":      GetLocalIPs(),
			"yh_mode":      config.AppConfig.YH.Mode,
			"bot_qq":       config.AppConfig.QQ.BotQQ,
			"temp_folder":  config.AppConfig.TempFolder,
			"image_scale":  config.AppConfig.Image.Scale,
			"image_max_w":  config.AppConfig.Image.MaxWidth,
			"image_max_h":  config.AppConfig.Image.MaxHeight,
			"blocked_cats": blockedWordGroups,
			"blocked_sum":  totalBlockedWords,
			"uptime_sec":   int64(time.Since(startTime).Seconds()),
		},
	})
}

func WebUIHandler(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Amer 控制台 - 互通管理系统</title>
    <style>
        :root {
            --bg-color: #f8fafc;
            --surface-color: #ffffff;
            --border-color: #e2e8f0;
            --text-primary: #0f172a;
            --text-secondary: #475569;
            --text-muted: #94a3b8;
            --primary: #2563eb;
            --primary-hover: #1d4ed8;
            --success: #10b981;
            --danger: #ef4444;
            --danger-hover: #dc2626;
            --warning: #f59e0b;
            --radius: 6px;
            --font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: var(--font); background: var(--bg-color); color: var(--text-primary); line-height: 1.5; font-size: 14px; }
        
        .navbar { background: var(--surface-color); border-bottom: 1px solid var(--border-color); height: 56px; display: flex; align-items: center; justify-content: space-between; padding: 0 24px; position: sticky; top: 0; z-index: 100; }
        .nav-brand { display: flex; align-items: center; gap: 10px; font-weight: 600; font-size: 16px; color: var(--text-primary); }
        .nav-brand .logo { width: 28px; height: 28px; background: var(--primary); color: white; border-radius: var(--radius); display: flex; align-items: center; justify-content: center; font-weight: bold; font-size: 14px; }
        .status-badge { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: #047857; background: #ecfdf5; padding: 3px 8px; border-radius: 9999px; border: 1px solid #a7f3d0; font-weight: 500; }
        .status-dot { width: 6px; height: 6px; background: var(--success); border-radius: 50%; }

        .container { max-width: 1200px; margin: 24px auto; padding: 0 20px; }
        
        .tabs-header { display: flex; gap: 8px; border-bottom: 1px solid var(--border-color); margin-bottom: 24px; }
        .tab-btn { background: none; border: none; padding: 10px 18px; font-size: 14px; font-weight: 500; color: var(--text-secondary); cursor: pointer; border-bottom: 2px solid transparent; transition: all 0.15s ease; }
        .tab-btn:hover { color: var(--primary); }
        .tab-btn.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }

        .tab-pane { display: none; }
        .tab-pane.active { display: block; }

        .grid-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; margin-bottom: 24px; }
        .stat-card { background: var(--surface-color); border: 1px solid var(--border-color); border-radius: var(--radius); padding: 18px 20px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
        .stat-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; }
        .stat-val { font-size: 26px; font-weight: 700; color: var(--text-primary); }

        .panel { background: var(--surface-color); border: 1px solid var(--border-color); border-radius: var(--radius); margin-bottom: 24px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
        .panel-header { padding: 16px 20px; border-bottom: 1px solid var(--border-color); display: flex; align-items: center; justify-content: space-between; }
        .panel-title { font-size: 15px; font-weight: 600; color: var(--text-primary); display: flex; align-items: center; gap: 8px; }
        .panel-body { padding: 20px; }

        .btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 7px 14px; font-size: 13px; font-weight: 500; border-radius: var(--radius); cursor: pointer; border: 1px solid transparent; transition: background 0.15s; text-decoration: none; }
        .btn-primary { background: var(--primary); color: #fff; }
        .btn-primary:hover { background: var(--primary-hover); }
        .btn-outline { background: #fff; border-color: var(--border-color); color: var(--text-primary); }
        .btn-outline:hover { background: #f1f5f9; }
        .btn-danger { background: #fee2e2; border-color: #fecaca; color: #b91c1c; }
        .btn-danger:hover { background: #fca5a5; }
        .btn-sm { padding: 4px 10px; font-size: 12px; }

        .form-row { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 16px; align-items: flex-end; }
        .form-group { display: flex; flex-direction: column; gap: 6px; flex: 1; min-width: 180px; }
        .form-group label { font-size: 12px; font-weight: 500; color: var(--text-secondary); }
        .form-control { height: 34px; padding: 0 10px; border: 1px solid var(--border-color); border-radius: var(--radius); font-size: 13px; color: var(--text-primary); outline: none; background: #fff; }
        .form-control:focus { border-color: var(--primary); }

        .table-responsive { width: 100%; overflow-x: auto; }
        table { width: 100%; border-collapse: collapse; text-align: left; font-size: 13px; }
        th { background: #f8fafc; color: var(--text-secondary); font-weight: 600; padding: 10px 14px; border-bottom: 1px solid var(--border-color); white-space: nowrap; }
        td { padding: 12px 14px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); vertical-align: middle; }
        tr:last-child td { border-bottom: none; }
        tr:hover td { background: #f8fafc; }

        .tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }
        .tag-all { background: #ecfdf5; color: #047857; border: 1px solid #a7f3d0; }
        .tag-qq { background: #eff6ff; color: #1d4ed8; border: 1px solid #bfdbfe; }
        .tag-yh { background: #fffbeb; color: #b45309; border: 1px solid #fde68a; }
        .tag-stopped { background: #f1f5f9; color: #475569; border: 1px solid #cbd5e1; }

        .select-sm { height: 28px; padding: 0 6px; font-size: 12px; border-radius: var(--radius); border: 1px solid var(--border-color); background: #fff; }

        #toast-container { position: fixed; top: 16px; right: 20px; z-index: 9999; display: flex; flex-direction: column; gap: 8px; }
        .toast { padding: 10px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; color: #fff; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1); opacity: 0; transform: translateY(-10px); transition: all 0.2s ease; }
        .toast.show { opacity: 1; transform: translateY(0); }
        .toast-success { background: #059669; }
        .toast-error { background: #dc2626; }

        .empty-state { text-align: center; padding: 40px 20px; color: var(--text-muted); font-size: 13px; }

        .info-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
        .info-item { background: #f8fafc; border: 1px solid var(--border-color); border-radius: var(--radius); padding: 10px 14px; }
        .info-item .k { font-size: 12px; color: var(--text-secondary); margin-bottom: 2px; }
        .info-item .v { font-weight: 600; color: var(--text-primary); font-size: 13px; }
    </style>
</head>
<body>

    <header class="navbar">
        <div class="nav-brand">
            <div class="logo">A</div>
            <span>Amer 控制中心</span>
            <span class="status-badge"><span class="status-dot"></span> 服务正常</span>
            <span id="nav-lan-badge" style="display:none;font-size:12px;color:#2563eb;background:#eff6ff;padding:2px 8px;border-radius:9999px;border:1px solid #bfdbfe;font-weight:500;"></span>
        </div>
        <div>
            <button class="btn btn-outline btn-sm" onclick="refreshCurrentTab()">🔄 刷新数据</button>
        </div>
    </header>

    <div id="toast-container"></div>

    <main class="container">
        <nav class="tabs-header">
            <button class="tab-btn active" onclick="switchTab('dashboard')">📊 运行概览</button>
            <button class="tab-btn" onclick="switchTab('bindings')">🔗 群聊绑定与转发模式</button>
            <button class="tab-btn" onclick="switchTab('blacklist')">🚫 黑名单管理</button>
            <button class="tab-btn" onclick="switchTab('logs')">📜 控制台日志</button>
            <button class="tab-btn" onclick="switchTab('system')">⚙️ 系统状态与配置</button>
        </nav>

        <!-- 1. 运行概览 Tab -->
        <section id="pane-dashboard" class="tab-pane active">
            <!-- 局域网内网访问指引条 -->
            <div id="lan-access-banner" style="display:none;background:#eff6ff;border:1px solid #bfdbfe;border-left:4px solid #2563eb;border-radius:var(--radius);padding:12px 16px;margin-bottom:20px;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;">
                <div>
                    <div style="font-weight:600;font-size:13px;color:#1e40af;margin-bottom:3px;">🌐 局域网内网访问支持</div>
                    <div id="lan-access-urls" style="font-size:12px;color:#1d4ed8;line-height:1.6;"></div>
                </div>
                <div style="font-size:11px;color:#3b82f6;background:#dbeafe;padding:4px 8px;border-radius:4px;">局域网内任意手机或电脑浏览器输入上方链接均可直接访问控制台</div>
            </div>

            <div class="grid-stats">
                <div class="stat-card">
                    <div class="stat-label">总同步消息数</div>
                    <div class="stat-val" id="stat-total">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">QQ 消息量</div>
                    <div class="stat-val" id="stat-qq">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">云湖消息量</div>
                    <div class="stat-val" id="stat-yh">-</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">当前群绑定对数</div>
                    <div class="stat-val" id="stat-bindings">-</div>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📌 快速快捷入口</div>
                </div>
                <div class="panel-body" style="display: flex; gap: 12px; flex-wrap: wrap;">
                    <button class="btn btn-primary" onclick="switchTab('bindings'); document.getElementById('qq-input').focus();">➕ 绑定新群聊</button>
                    <button class="btn btn-outline" onclick="switchTab('logs')">📜 查看实时控制台日志</button>
                    <button class="btn btn-outline" onclick="switchTab('blacklist')">🚫 查看与添加黑名单</button>
                    <button class="btn btn-outline" onclick="switchTab('system')">⚙️ 查看详细系统配置</button>
                </div>
            </div>
        </section>

        <!-- 2. 群聊绑定与模式管理 Tab -->
        <section id="pane-bindings" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">➕ 新增群聊绑定</div>
                </div>
                <div class="panel-body">
                    <form id="bind-form" onsubmit="handleCreateBinding(event)">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="qq-input">QQ 群号 *</label>
                                <input id="qq-input" class="form-control" type="text" placeholder="例如: 1013637348" required>
                            </div>
                            <div class="form-group">
                                <label for="yh-input">云湖群号 *</label>
                                <input id="yh-input" class="form-control" type="text" placeholder="例如: 644353075" required>
                            </div>
                            <div class="form-group" style="max-width: 180px;">
                                <label for="mode-select">默认转发模式</label>
                                <select id="mode-select" class="form-control">
                                    <option value="全同步">全同步 (双向)</option>
                                    <option value="QQ到云湖">仅 QQ 到云湖</option>
                                    <option value="云湖到QQ">仅 云湖 到 QQ</option>
                                    <option value="停止">暂停同步</option>
                                </select>
                            </div>
                            <button type="submit" class="btn btn-primary" style="height: 34px;">确认绑定</button>
                        </div>
                    </form>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📋 当前已绑定群聊列表 (<span id="binding-count">0</span>)</div>
                    <div style="display: flex; gap: 8px;">
                        <input id="binding-search" class="form-control" style="height: 28px; width: 180px; font-size: 12px;" placeholder="搜索群号/群名..." oninput="filterBindingsTable()">
                    </div>
                </div>
                <div class="table-responsive">
                    <table>
                        <thead>
                            <tr>
                                <th>QQ 群信息</th>
                                <th>云湖群信息</th>
                                <th>当前转发模式</th>
                                <th>修改转发模式</th>
                                <th style="text-align: right;">操作</th>
                            </tr>
                        </thead>
                        <tbody id="bindings-tbody">
                            <tr><td colspan="5" class="empty-state">正在加载绑定数据...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </section>

        <!-- 3. 黑名单管理 Tab -->
        <section id="pane-blacklist" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">🚫 添加用户到黑名单</div>
                </div>
                <div class="panel-body">
                    <form id="blacklist-form" onsubmit="handleAddBlacklist(event)">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="ban-uid">违规用户 ID *</label>
                                <input id="ban-uid" class="form-control" type="text" placeholder="QQ号或云湖用户ID" required>
                            </div>
                            <div class="form-group">
                                <label for="ban-reason">封禁原因</label>
                                <input id="ban-reason" class="form-control" type="text" placeholder="例如: 频繁刷屏 / 违规广告">
                            </div>
                            <div class="form-group" style="max-width: 180px;">
                                <label for="ban-duration">封禁时长</label>
                                <select id="ban-duration" class="form-control">
                                    <option value="1800">30 分钟</option>
                                    <option value="3600" selected>1 小时</option>
                                    <option value="86400">1 天</option>
                                    <option value="604800">7 天</option>
                                    <option value="0">永久封禁</option>
                                </select>
                            </div>
                            <button type="submit" class="btn btn-danger" style="height: 34px;">添加封禁</button>
                        </div>
                    </form>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📋 处于黑名单中的用户 (<span id="blacklist-count">0</span>)</div>
                </div>
                <div class="table-responsive">
                    <table>
                        <thead>
                            <tr>
                                <th>用户 ID</th>
                                <th>封禁原因</th>
                                <th>剩余时间</th>
                                <th style="text-align: right;">操作</th>
                            </tr>
                        </thead>
                        <tbody id="blacklist-tbody">
                            <tr><td colspan="4" class="empty-state">暂无被封禁的用户</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </section>

        <!-- 4. 控制台日志 Tab -->
        <section id="pane-logs" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">📜 命令行控制台日志</div>
                    <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
                        <span id="log-status-badge" class="status-badge" style="background:#ecfdf5;color:#047857;border-color:#a7f3d0;">
                            <span class="status-dot"></span> 实时接收中 (SSE)
                        </span>
                        <button id="btn-toggle-live" class="btn btn-outline btn-sm" onclick="toggleLiveLogs()">⏸️ 暂停实时</button>
                        <button id="btn-refresh-logs" class="btn btn-outline btn-sm" style="display:none;" onclick="loadStaticLogs()">🔄 刷新快照</button>
                        <button class="btn btn-outline btn-sm" onclick="clearLogsScreen()">🗑️ 清屏</button>
                        <button class="btn btn-outline btn-sm" onclick="exportLogs()">📥 导出</button>
                    </div>
                </div>
                <div class="panel-body" style="padding:14px 20px;">
                    <!-- 过滤栏与开关 -->
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px;gap:12px;flex-wrap:wrap;">
                        <div style="display:flex;align-items:center;gap:8px;flex:1;max-width:380px;">
                            <input id="log-search-input" class="form-control" type="text" placeholder="🔍 过滤关键字 (如 QQ, Yunhu, Error...)" oninput="filterLogs()" style="height:32px;font-size:12px;">
                        </div>
                        <div style="display:flex;align-items:center;gap:16px;font-size:12px;color:var(--text-secondary);">
                            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
                                <input id="chk-auto-scroll" type="checkbox" checked onchange="toggleAutoScroll(this.checked)">
                                <span>自动滚动到底部</span>
                            </label>
                            <span id="log-count-text">显示: 0 行</span>
                        </div>
                    </div>

                    <!-- 终端日志容器 -->
                    <div id="terminal-container" style="background:#0f172a;border:1px solid #334155;border-radius:6px;padding:12px 14px;height:560px;overflow-y:auto;font-family:'JetBrains Mono', Consolas, Menlo, Monaco, monospace;font-size:12px;line-height:1.55;color:#cbd5e1;white-space:pre-wrap;word-break:break-all;">
                    </div>
                </div>
            </div>
        </section>

        <!-- 5. 系统设置与状态 Tab -->
        <section id="pane-system" class="tab-pane">
            <div class="panel">
                <div class="panel-header">
                    <div class="panel-title">⚙️ 运行参数与系统配置</div>
                </div>
                <div class="panel-body">
                    <div class="info-grid" id="system-info-grid">
                        <div class="info-item"><div class="k">正在加载系统信息...</div></div>
                    </div>
                </div>
            </div>
        </section>
    </main>

    <script>
        let cachedBindings = [];

        function switchTab(tabId) {
            document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelectorAll('.tab-pane').forEach(pane => pane.classList.remove('active'));
            
            event && event.target && event.target.classList.add('active');
            const targetPane = document.getElementById('pane-' + tabId);
            if (targetPane) targetPane.classList.add('active');

            if (tabId === 'dashboard') loadStats();
            if (tabId === 'bindings') loadBindings();
            if (tabId === 'blacklist') loadBlacklist();
            if (tabId === 'logs') {
                if (isLive && !logEventSource) {
                    startLiveLogs();
                } else if (!isLive && allLogLines.length === 0) {
                    loadStaticLogs();
                }
            }
            if (tabId === 'system') loadSystemInfo();
        }

        function refreshCurrentTab() {
            loadStats();
            loadBindings();
            loadBlacklist();
            loadSystemInfo();
            if (!isLive) {
                loadStaticLogs();
            }
            showToast('数据已刷新', 'success');
        }

        function showToast(msg, type = 'success') {
            const container = document.getElementById('toast-container');
            const toast = document.createElement('div');
            toast.className = 'toast toast-' + type;
            toast.innerText = msg;
            container.appendChild(toast);

            requestAnimationFrame(() => toast.classList.add('show'));
            setTimeout(() => {
                toast.classList.remove('show');
                setTimeout(() => toast.remove(), 200);
            }, 2600);
        }

        function loadStats() {
            fetch('/api/stats')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        const d = res.data;
                        document.getElementById('stat-total').innerText = d.total_messages;
                        document.getElementById('stat-qq').innerText = d.qq_messages;
                        document.getElementById('stat-yh').innerText = d.yh_messages;
                        document.getElementById('stat-bindings').innerText = d.total_bindings;
                    }
                })
                .catch(() => {});
        }

        function loadBindings() {
            fetch('/api/bindings')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        cachedBindings = res.data || [];
                        renderBindings(cachedBindings);
                    }
                })
                .catch(() => {
                    document.getElementById('bindings-tbody').innerHTML = '<tr><td colspan="5" class="empty-state">获取绑定列表失败</td></tr>';
                });
        }

        function renderBindings(list) {
            const tbody = document.getElementById('bindings-tbody');
            document.getElementById('binding-count').innerText = list.length;
            if (!list || list.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" class="empty-state">暂无绑定的群聊，请在上方添加新绑定</td></tr>';
                return;
            }

            let html = '';
            list.forEach(function(item) {
                let tagClass = 'tag-all';
                if (item.sync_mode === 'QQ到云湖') tagClass = 'tag-qq';
                else if (item.sync_mode === '云湖到QQ') tagClass = 'tag-yh';
                else if (item.sync_mode === '停止') tagClass = 'tag-stopped';

                html += '<tr>' +
                    '<td><strong>' + escapeHTML(item.qq_group_name) + '</strong>' +
                    '<div style="font-size:11px;color:#64748b;">群号: ' + escapeHTML(item.qq_group_id) + '</div></td>' +
                    '<td><strong>' + escapeHTML(item.yh_group_name) + '</strong>' +
                    '<div style="font-size:11px;color:#64748b;">群号: ' + escapeHTML(item.yh_group_id) + '</div></td>' +
                    '<td><span class="tag ' + tagClass + '">' + escapeHTML(item.sync_mode) + '</span></td>' +
                    '<td><select class="select-sm" onchange="changeSyncMode(\'' + item.qq_group_id + '\',\'' + item.yh_group_id + '\',this.value)">' +
                    '<option value="全同步"' + (item.sync_mode === '全同步' ? ' selected' : '') + '>全同步 (双向)</option>' +
                    '<option value="QQ到云湖"' + (item.sync_mode === 'QQ到云湖' ? ' selected' : '') + '>仅 QQ 到云湖</option>' +
                    '<option value="云湖到QQ"' + (item.sync_mode === '云湖到QQ' ? ' selected' : '') + '>仅 云湖 到 QQ</option>' +
                    '<option value="停止"' + (item.sync_mode === '停止' ? ' selected' : '') + '>暂停同步</option>' +
                    '</select></td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-danger btn-sm" onclick="handleUnbind(\'' + item.qq_group_id + '\',\'' + item.yh_group_id + '\')">解除绑定</button>' +
                    '</td></tr>';
            });
            tbody.innerHTML = html;
        }

        function filterBindingsTable() {
            const query = document.getElementById('binding-search').value.trim().toLowerCase();
            if (!query) {
                renderBindings(cachedBindings);
                return;
            }
            const filtered = cachedBindings.filter(item => 
                item.qq_group_id.toLowerCase().includes(query) ||
                item.yh_group_id.toLowerCase().includes(query) ||
                (item.qq_group_name && item.qq_group_name.toLowerCase().includes(query)) ||
                (item.yh_group_name && item.yh_group_name.toLowerCase().includes(query))
            );
            renderBindings(filtered);
        }

        function handleCreateBinding(e) {
            e.preventDefault();
            const qq = document.getElementById('qq-input').value.trim();
            const yh = document.getElementById('yh-input').value.trim();
            const mode = document.getElementById('mode-select').value;

            if (!qq || !yh) {
                showToast('QQ群号与云湖群号均不能为空', 'error');
                return;
            }

            fetch('/api/bindings/bind', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh, sync_mode: mode })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('群聊绑定成功！');
                    document.getElementById('qq-input').value = '';
                    document.getElementById('yh-input').value = '';
                    loadBindings();
                    loadStats();
                } else {
                    showToast(res.msg || '绑定失败', 'error');
                }
            })
            .catch(() => showToast('网络请求失败', 'error'));
        }

        function changeSyncMode(qq, yh, newMode) {
            fetch('/api/bindings/mode', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh, sync_mode: newMode })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已将转发模式更新为: ' + newMode);
                    loadBindings();
                } else {
                    showToast(res.msg || '修改模式失败', 'error');
                }
            })
            .catch(() => showToast('修改模式请求异常', 'error'));
        }

        function handleUnbind(qq, yh) {
            if (!confirm('确定要解除 QQ群 (' + qq + ') 与 云湖群 (' + yh + ') 的互通绑定吗？')) {
                return;
            }

            fetch('/api/bindings/unbind', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ qq_group_id: qq, yh_group_id: yh })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('解绑成功');
                    loadBindings();
                    loadStats();
                } else {
                    showToast(res.msg || '解绑失败', 'error');
                }
            })
            .catch(() => showToast('解绑请求异常', 'error'));
        }

        function loadBlacklist() {
            fetch('/api/blacklist')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        renderBlacklist(res.data || []);
                    }
                })
                .catch(() => {
                    document.getElementById('blacklist-tbody').innerHTML = '<tr><td colspan="4" class="empty-state">加载黑名单失败</td></tr>';
                });
        }

        function renderBlacklist(list) {
            const tbody = document.getElementById('blacklist-tbody');
            document.getElementById('blacklist-count').innerText = list.length;
            if (!list || list.length === 0) {
                tbody.innerHTML = '<tr><td colspan="4" class="empty-state">暂无处于黑名单中的用户</td></tr>';
                return;
            }

            let html = '';
            list.forEach(function(item) {
                let timeStr = '永久封禁';
                if (item.remaining_time > 0) {
                    const mins = Math.ceil(item.remaining_time / 60);
                    timeStr = '约 ' + mins + ' 分钟后自动解封';
                }

                html += '<tr>' +
                    '<td><code>' + escapeHTML(item.user_id) + '</code></td>' +
                    '<td>' + escapeHTML(item.reason || '无') + '</td>' +
                    '<td><span class="tag tag-stopped">' + timeStr + '</span></td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-outline btn-sm" onclick="handleRemoveBlacklist(\'' + item.user_id + '\')">解除封禁</button>' +
                    '</td></tr>';
            });
            tbody.innerHTML = html;
        }

        function handleAddBlacklist(e) {
            e.preventDefault();
            const uid = document.getElementById('ban-uid').value.trim();
            const reason = document.getElementById('ban-reason').value.trim();
            const duration = parseInt(document.getElementById('ban-duration').value, 10);

            if (!uid) {
                showToast('用户 ID 不能为空', 'error');
                return;
            }

            fetch('/api/blacklist/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_id: uid, reason: reason, duration: duration })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已添加封禁');
                    document.getElementById('ban-uid').value = '';
                    document.getElementById('ban-reason').value = '';
                    loadBlacklist();
                } else {
                    showToast(res.msg || '添加黑名单失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        function handleRemoveBlacklist(uid) {
            if (!confirm('确认解封用户 ' + uid + ' 吗？')) return;

            fetch('/api/blacklist/remove', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_id: uid })
            })
            .then(r => r.json())
            .then(res => {
                if (res.status === 'success') {
                    showToast('已解除封禁');
                    loadBlacklist();
                } else {
                    showToast(res.msg || '操作失败', 'error');
                }
            })
            .catch(() => showToast('请求失败', 'error'));
        }

        let isLive = true;
        let logEventSource = null;
        let allLogLines = [];
        let autoScroll = true;

        function startLiveLogs() {
            if (logEventSource) {
                logEventSource.close();
            }
            isLive = true;
            const badge = document.getElementById('log-status-badge');
            if (badge) {
                badge.innerHTML = '<span class="status-dot"></span> 实时接收中 (SSE)';
                badge.style.background = '#ecfdf5';
                badge.style.color = '#047857';
                badge.style.borderColor = '#a7f3d0';
            }
            const btnToggle = document.getElementById('btn-toggle-live');
            if (btnToggle) btnToggle.innerHTML = '⏸️ 暂停实时';
            const btnRef = document.getElementById('btn-refresh-logs');
            if (btnRef) btnRef.style.display = 'none';

            logEventSource = new EventSource('/api/logs/stream');
            logEventSource.onmessage = function(e) {
                if (e.data) {
                    appendLogLine(e.data);
                }
            };
            logEventSource.onerror = function() {
                console.warn('SSE 重连中...');
            };
        }

        function stopLiveLogs() {
            isLive = false;
            if (logEventSource) {
                logEventSource.close();
                logEventSource = null;
            }
            const badge = document.getElementById('log-status-badge');
            if (badge) {
                badge.innerHTML = '⚪ 静态快照 (已暂停实时)';
                badge.style.background = '#f1f5f9';
                badge.style.color = '#475569';
                badge.style.borderColor = '#cbd5e1';
            }
            const btnToggle = document.getElementById('btn-toggle-live');
            if (btnToggle) btnToggle.innerHTML = '▶️ 恢复实时接收';
            const btnRef = document.getElementById('btn-refresh-logs');
            if (btnRef) btnRef.style.display = 'inline-flex';
        }

        function toggleLiveLogs() {
            if (isLive) {
                stopLiveLogs();
            } else {
                startLiveLogs();
            }
        }

        function loadStaticLogs() {
            fetch('/api/logs?lines=500')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success' && res.data && res.data.logs) {
                        allLogLines = res.data.logs;
                        renderTerminalLines();
                        showToast('静态日志快照已刷新');
                    }
                })
                .catch(() => showToast('获取日志失败', 'error'));
        }

        function appendLogLine(rawText) {
            allLogLines.push(rawText);
            if (allLogLines.length > 2500) {
                allLogLines.shift();
            }

            const term = document.getElementById('terminal-container');
            if (!term) return;
            const kw = document.getElementById('log-search-input').value.trim().toLowerCase();
            if (!kw || rawText.toLowerCase().indexOf(kw) !== -1) {
                const lineDiv = document.createElement('div');
                lineDiv.innerHTML = colorizeLogLine(rawText);
                term.appendChild(lineDiv);
                if (autoScroll) {
                    term.scrollTop = term.scrollHeight;
                }
            }
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: ' + term.children.length + ' 行';
        }

        function colorizeLogLine(text) {
            const s = escapeHTML(text);
            if (s.indexOf('Error') !== -1 || s.indexOf('失败') !== -1 || s.indexOf('panic') !== -1 || s.indexOf('FAIL') !== -1) {
                return '<span style="color:#f87171;">' + s + '</span>';
            }
            if (s.indexOf('Success') !== -1 || s.indexOf('成功') !== -1) {
                return '<span style="color:#34d399;">' + s + '</span>';
            }
            if (s.indexOf('Warning') !== -1 || s.indexOf('警告') !== -1) {
                return '<span style="color:#fbbf24;">' + s + '</span>';
            }
            if (s.indexOf('[QQ') !== -1 || s.indexOf('[OneBot') !== -1) {
                return '<span style="color:#38bdf8;">' + s + '</span>';
            }
            if (s.indexOf('[Yunhu') !== -1 || s.indexOf('[YH') !== -1) {
                return '<span style="color:#a78bfa;">' + s + '</span>';
            }
            if (s.indexOf('[WebUI') !== -1 || s.indexOf('[Web Server') !== -1) {
                return '<span style="color:#60a5fa;">' + s + '</span>';
            }
            return '<span style="color:#cbd5e1;">' + s + '</span>';
        }

        function renderTerminalLines() {
            const term = document.getElementById('terminal-container');
            if (!term) return;
            const kw = document.getElementById('log-search-input').value.trim().toLowerCase();
            term.innerHTML = '';
            let count = 0;
            for (let i = 0; i < allLogLines.length; i++) {
                const line = allLogLines[i];
                if (!kw || line.toLowerCase().indexOf(kw) !== -1) {
                    const div = document.createElement('div');
                    div.innerHTML = colorizeLogLine(line);
                    term.appendChild(div);
                    count++;
                }
            }
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: ' + count + ' 行';
            if (autoScroll) {
                term.scrollTop = term.scrollHeight;
            }
        }

        function filterLogs() {
            renderTerminalLines();
        }

        function clearLogsScreen() {
            allLogLines = [];
            const term = document.getElementById('terminal-container');
            if (term) term.innerHTML = '<div style="color:#64748b;padding:8px 0;">已清空屏幕，等待新日志...</div>';
            const countEl = document.getElementById('log-count-text');
            if (countEl) countEl.innerText = '显示: 0 行';
        }

        function toggleAutoScroll(enabled) {
            autoScroll = enabled;
            if (autoScroll) {
                const term = document.getElementById('terminal-container');
                if (term) term.scrollTop = term.scrollHeight;
            }
        }

        function exportLogs() {
            if (allLogLines.length === 0) {
                showToast('暂无日志可导出', 'error');
                return;
            }
            const blob = new Blob([allLogLines.join('\n')], { type: 'text/plain;charset=utf-8' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'amer-console-' + new Date().toISOString().replace(/[:.]/g, '-') + '.log';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            showToast('日志已成功导出');
        }

        function loadSystemInfo() {
            fetch('/api/system')
                .then(r => r.json())
                .then(res => {
                    if (res.status === 'success') {
                        const d = res.data;
                        const container = document.getElementById('system-info-grid');
                        const port = d.webui_port || d.port;

                        // 更新局域网内网访问提示
                        if (d.lan_ips && d.lan_ips.length > 0) {
                            const mainLanIP = d.lan_ips[0];
                            const navBadge = document.getElementById('nav-lan-badge');
                            if (navBadge) {
                                navBadge.style.display = 'inline-block';
                                navBadge.innerText = '内网: http://' + mainLanIP + ':' + port + '/webui';
                            }
                            const banner = document.getElementById('lan-access-banner');
                            const bannerUrls = document.getElementById('lan-access-urls');
                            if (banner && bannerUrls) {
                                banner.style.display = 'flex';
                                let urlsHtml = '';
                                for (let i = 0; i < d.lan_ips.length; i++) {
                                    const u = 'http://' + d.lan_ips[i] + ':' + port + '/webui';
                                    urlsHtml += '<div>• 内网访问地址 ' + (i + 1) + ': <a href="' + u + '" target="_blank" style="color:#1d4ed8;font-weight:600;text-decoration:underline;">' + u + '</a></div>';
                                }
                                bannerUrls.innerHTML = urlsHtml;
                            }
                        }

                        let lanIPsStr = '未检测到局域网 IP';
                        if (d.lan_ips && d.lan_ips.length > 0) {
                            lanIPsStr = d.lan_ips.map(ip => 'http://' + ip + ':' + port + '/webui').join('<br>');
                        }

                        container.innerHTML =
                            '<div class="info-item"><div class="k">WebUI 独立运行端口</div><div class="v">' + port + '</div></div>' +
                            '<div class="info-item"><div class="k">绑定监听主机 (0.0.0.0 允许局域网访问)</div><div class="v">' + d.webui_host + '</div></div>' +
                            '<div class="info-item" style="grid-column: 1 / -1;"><div class="k">局域网内网访问地址 (其他手机/电脑访问)</div><div class="v" style="color:var(--primary);word-break:break-all;">' + lanIPsStr + '</div></div>' +
                            '<div class="info-item"><div class="k">云湖接收模式</div><div class="v">' + d.yh_mode + '</div></div>' +
                            '<div class="info-item"><div class="k">QQ 机器人账号</div><div class="v">' + (d.bot_qq || '未指定') + '</div></div>' +
                            '<div class="info-item"><div class="k">图片缩放比例</div><div class="v">' + (d.image_scale || '默认') + '</div></div>' +
                            '<div class="info-item"><div class="k">图片限制尺寸 (宽x高)</div><div class="v">' + (d.image_max_w || 300) + 'px × ' + (d.image_max_h || 320) + 'px</div></div>' +
                            '<div class="info-item"><div class="k">屏蔽词总数</div><div class="v">' + d.blocked_sum + ' 个</div></div>' +
                            '<div class="info-item"><div class="k">临时缓存目录</div><div class="v">' + (d.temp_folder || 'temp') + '</div></div>';
                    }
                })
                .catch(() => {});
        }

        function escapeHTML(str) {
            if (!str) return '';
            return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        }

        loadStats();
        loadSystemInfo();
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
