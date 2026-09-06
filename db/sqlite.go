package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	dbConn *sql.DB
	dbMu   sync.Mutex
)

type BindingItem struct {
	ID          string `json:"id"`
	Sync        bool   `json:"sync"`
	BindingSync bool   `json:"binding_sync,omitempty"`
}

type BindInfoResult struct {
	Status int                    `json:"status"`
	Msg    string                 `json:"msg"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

func CloseSQLite() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbConn != nil {
		err := dbConn.Close()
		dbConn = nil
		return err
	}
	return nil
}

func InitSQLite(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建SQLite目录失败: %w", err)
	}

	dbMu.Lock()
	if dbConn != nil {
		_ = dbConn.Close()
		dbConn = nil
	}
	dbMu.Unlock()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开SQLite数据库失败: %w", err)
	}

	dbConn = db

	createQQTable := `
	CREATE TABLE IF NOT EXISTS QQ_table (
		QQ_group_id TEXT PRIMARY KEY,
		YH_group_ids TEXT
	);`

	createYHTable := `
	CREATE TABLE IF NOT EXISTS YH_table (
		YH_group_id TEXT PRIMARY KEY,
		QQ_group_ids TEXT
	);`

	if _, err := db.Exec(createQQTable); err != nil {
		return fmt.Errorf("创建QQ_table失败: %w", err)
	}
	if _, err := db.Exec(createYHTable); err != nil {
		return fmt.Errorf("创建YH_table失败: %w", err)
	}

	return nil
}

// HasActiveBinding checks if a group has any active bindings with sync enabled.
func HasActiveBinding(platform, id string) bool {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbConn == nil {
		return false
	}

	if platform == "QQ" {
		var yhGroupIDsJSON sql.NullString
		err := dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", id).Scan(&yhGroupIDsJSON)
		if err != nil || !yhGroupIDsJSON.Valid || yhGroupIDsJSON.String == "" {
			return false
		}
		var yhItems []BindingItem
		_ = json.Unmarshal([]byte(yhGroupIDsJSON.String), &yhItems)
		for _, item := range yhItems {
			if item.Sync {
				return true
			}
		}
		return false
	} else if platform == "YH" {
		var qqGroupIDsJSON sql.NullString
		err := dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", id).Scan(&qqGroupIDsJSON)
		if err != nil || !qqGroupIDsJSON.Valid || qqGroupIDsJSON.String == "" {
			return false
		}
		var qqItems []BindingItem
		_ = json.Unmarshal([]byte(qqGroupIDsJSON.String), &qqItems)
		for _, item := range qqItems {
			if item.Sync {
				return true
			}
		}
		return false
	}
	return false
}

func GetInfo(platform, id string) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	if platform == "QQ" {
		var yhGroupIDsJSON sql.NullString
		err := dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", id).Scan(&yhGroupIDsJSON)
		if err == sql.ErrNoRows {
			return BindInfoResult{Status: -1, Msg: fmt.Sprintf("QQ群 %s 未找到绑定数据", id)}
		} else if err != nil {
			return BindInfoResult{Status: -1, Msg: fmt.Sprintf("查询失败: %v", err)}
		}

		var yhItems []BindingItem
		if yhGroupIDsJSON.Valid && yhGroupIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(yhGroupIDsJSON.String), &yhItems)
		}

		// Enrich binding_sync status from YH_table
		for i, item := range yhItems {
			var qqJSON sql.NullString
			_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", item.ID).Scan(&qqJSON)
			if qqJSON.Valid && qqJSON.String != "" {
				var qqItems []BindingItem
				_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
				for _, qItem := range qqItems {
					if qItem.ID == id {
						yhItems[i].BindingSync = qItem.Sync
						break
					}
				}
			}
		}

		return BindInfoResult{
			Status: 0,
			Msg:    "获取信息成功",
			Data: map[string]interface{}{
				"QQ_group_id":  id,
				"YH_group_ids": yhItems,
			},
		}

	} else if platform == "YH" {
		var qqGroupIDsJSON sql.NullString
		err := dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", id).Scan(&qqGroupIDsJSON)
		if err == sql.ErrNoRows {
			return BindInfoResult{Status: -1, Msg: fmt.Sprintf("云湖群 %s 未找到绑定数据", id)}
		} else if err != nil {
			return BindInfoResult{Status: -1, Msg: fmt.Sprintf("查询失败: %v", err)}
		}

		var qqItems []BindingItem
		if qqGroupIDsJSON.Valid && qqGroupIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(qqGroupIDsJSON.String), &qqItems)
		}

		for i, item := range qqItems {
			var yhJSON sql.NullString
			_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", item.ID).Scan(&yhJSON)
			if yhJSON.Valid && yhJSON.String != "" {
				var yhItems []BindingItem
				_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
				for _, yItem := range yhItems {
					if yItem.ID == id {
						qqItems[i].BindingSync = yItem.Sync
						break
					}
				}
			}
		}

		return BindInfoResult{
			Status: 0,
			Msg:    "获取信息成功",
			Data: map[string]interface{}{
				"YH_group_id":  id,
				"QQ_group_ids": qqItems,
			},
		}
	}

	return BindInfoResult{Status: -1, Msg: "不支持的平台"}
}

func Bind(platformA, platformB, idA, idB string) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	if (platformA == "QQ" && platformB == "YH") || (platformA == "YH" && platformB == "QQ") {
		qqID := idA
		yhID := idB
		if platformA == "YH" {
			qqID = idB
			yhID = idA
		}

		// Update QQ_table
		var yhJSON sql.NullString
		_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", qqID).Scan(&yhJSON)
		var yhItems []BindingItem
		if yhJSON.Valid && yhJSON.String != "" {
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
		}

		found := false
		for _, item := range yhItems {
			if item.ID == yhID {
				found = true
				break
			}
		}
		if !found {
			yhItems = append(yhItems, BindingItem{ID: yhID, Sync: true})
			bytes, _ := json.Marshal(yhItems)
			_, _ = dbConn.Exec("INSERT INTO QQ_table (QQ_group_id, YH_group_ids) VALUES (?, ?) ON CONFLICT(QQ_group_id) DO UPDATE SET YH_group_ids=?", qqID, string(bytes), string(bytes))
		}

		// Update YH_table
		var qqJSON sql.NullString
		_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", yhID).Scan(&qqJSON)
		var qqItems []BindingItem
		if qqJSON.Valid && qqJSON.String != "" {
			_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
		}

		found = false
		for _, item := range qqItems {
			if item.ID == qqID {
				found = true
				break
			}
		}
		if !found {
			qqItems = append(qqItems, BindingItem{ID: qqID, Sync: true})
			bytes, _ := json.Marshal(qqItems)
			_, _ = dbConn.Exec("INSERT INTO YH_table (YH_group_id, QQ_group_ids) VALUES (?, ?) ON CONFLICT(YH_group_id) DO UPDATE SET QQ_group_ids=?", yhID, string(bytes), string(bytes))
		}

		return BindInfoResult{Status: 0, Msg: "绑定成功"}
	}

	return BindInfoResult{Status: -1, Msg: "不支持的绑定类型"}
}

func Unbind(platformA, platformB, idA, idB string) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	if (platformA == "QQ" && platformB == "YH") || (platformA == "YH" && platformB == "QQ") {
		qqID := idA
		yhID := idB
		if platformA == "YH" {
			qqID = idB
			yhID = idA
		}

		// Remove from QQ_table
		var yhJSON sql.NullString
		_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", qqID).Scan(&yhJSON)
		if yhJSON.Valid && yhJSON.String != "" {
			var yhItems []BindingItem
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
			newYhItems := make([]BindingItem, 0, len(yhItems))
			for _, item := range yhItems {
				if item.ID != yhID {
					newYhItems = append(newYhItems, item)
				}
			}
			bytes, _ := json.Marshal(newYhItems)
			_, _ = dbConn.Exec("UPDATE QQ_table SET YH_group_ids=? WHERE QQ_group_id=?", string(bytes), qqID)
		}

		// Remove from YH_table
		var qqJSON sql.NullString
		_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", yhID).Scan(&qqJSON)
		if qqJSON.Valid && qqJSON.String != "" {
			var qqItems []BindingItem
			_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
			newQqItems := make([]BindingItem, 0, len(qqItems))
			for _, item := range qqItems {
				if item.ID != qqID {
					newQqItems = append(newQqItems, item)
				}
			}
			bytes, _ := json.Marshal(newQqItems)
			_, _ = dbConn.Exec("UPDATE YH_table SET QQ_group_ids=? WHERE YH_group_id=?", string(bytes), yhID)
		}

		return BindInfoResult{Status: 0, Msg: "解绑成功"}
	}

	return BindInfoResult{Status: -1, Msg: "不支持的解绑平台"}
}

func UnbindAll(platform, id string) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	if platform == "QQ" {
		var yhJSON sql.NullString
		_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", id).Scan(&yhJSON)
		if yhJSON.Valid && yhJSON.String != "" {
			var yhItems []BindingItem
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
			for _, item := range yhItems {
				// Remove id from YH_table
				var qqJSON sql.NullString
				_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", item.ID).Scan(&qqJSON)
				if qqJSON.Valid && qqJSON.String != "" {
					var qqItems []BindingItem
					_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
					newItems := make([]BindingItem, 0)
					for _, qItem := range qqItems {
						if qItem.ID != id {
							newItems = append(newItems, qItem)
						}
					}
					bytes, _ := json.Marshal(newItems)
					_, _ = dbConn.Exec("UPDATE YH_table SET QQ_group_ids=? WHERE YH_group_id=?", string(bytes), item.ID)
				}
			}
		}
		_, _ = dbConn.Exec("DELETE FROM QQ_table WHERE QQ_group_id=?", id)
		return BindInfoResult{Status: 0, Msg: "已解绑所有关联平台"}

	} else if platform == "YH" {
		var qqJSON sql.NullString
		_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", id).Scan(&qqJSON)
		if qqJSON.Valid && qqJSON.String != "" {
			var qqItems []BindingItem
			_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
			for _, item := range qqItems {
				// Remove id from QQ_table
				var yhJSON sql.NullString
				_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", item.ID).Scan(&yhJSON)
				if yhJSON.Valid && yhJSON.String != "" {
					var yhItems []BindingItem
					_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
					newItems := make([]BindingItem, 0)
					for _, yItem := range yhItems {
						if yItem.ID != id {
							newItems = append(newItems, yItem)
						}
					}
					bytes, _ := json.Marshal(newItems)
					_, _ = dbConn.Exec("UPDATE QQ_table SET YH_group_ids=? WHERE QQ_group_id=?", string(bytes), item.ID)
				}
			}
		}
		_, _ = dbConn.Exec("DELETE FROM YH_table WHERE YH_group_id=?", id)
		return BindInfoResult{Status: 0, Msg: "已解绑所有关联平台"}
	}

	return BindInfoResult{Status: -1, Msg: "解绑失败"}
}

func SetSync(platformA, platformB, idA, idB string, syncData map[string]bool) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	qqID := idA
	yhID := idB
	if platformA == "YH" {
		qqID = idB
		yhID = idA
	}

	qqToYhSync, hasQQToYH := syncData["QQ_TO_YH"]
	if !hasQQToYH {
		qqToYhSync, hasQQToYH = syncData["QQ"]
	}

	yhToQqSync, hasYHToQQ := syncData["YH_TO_QQ"]
	if !hasYHToQQ {
		yhToQqSync, hasYHToQQ = syncData["YH"]
	}

	if hasQQToYH {
		// Update QQ->YH sync status in QQ_table
		var yhJSON sql.NullString
		_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", qqID).Scan(&yhJSON)
		if yhJSON.Valid && yhJSON.String != "" {
			var yhItems []BindingItem
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
			for i, item := range yhItems {
				if item.ID == yhID {
					yhItems[i].Sync = qqToYhSync
					break
				}
			}
			bytes, _ := json.Marshal(yhItems)
			_, _ = dbConn.Exec("UPDATE QQ_table SET YH_group_ids=? WHERE QQ_group_id=?", string(bytes), qqID)
		}
	}

	if hasYHToQQ {
		// Update YH->QQ sync status in YH_table
		var qqJSON sql.NullString
		_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", yhID).Scan(&qqJSON)
		if qqJSON.Valid && qqJSON.String != "" {
			var qqItems []BindingItem
			_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
			for i, item := range qqItems {
				if item.ID == qqID {
					qqItems[i].Sync = yhToQqSync
					break
				}
			}
			bytes, _ := json.Marshal(qqItems)
			_, _ = dbConn.Exec("UPDATE YH_table SET QQ_group_ids=? WHERE YH_group_id=?", string(bytes), yhID)
		}
	}

	return BindInfoResult{Status: 0, Msg: "设置同步模式成功"}
}

func SetAllSync(platform, id string, syncData map[string]bool) BindInfoResult {
	dbMu.Lock()
	defer dbMu.Unlock()

	qqToYhSync, hasQQToYH := syncData["QQ_TO_YH"]
	if !hasQQToYH {
		qqToYhSync = syncData["QQ"]
	}
	yhToQqSync, hasYHToQQ := syncData["YH_TO_QQ"]
	if !hasYHToQQ {
		yhToQqSync = syncData["YH"]
	}

	if platform == "QQ" {
		var yhJSON sql.NullString
		_ = dbConn.QueryRow("SELECT YH_group_ids FROM QQ_table WHERE QQ_group_id=?", id).Scan(&yhJSON)
		if yhJSON.Valid && yhJSON.String != "" {
			var yhItems []BindingItem
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
			for i := range yhItems {
				yhItems[i].Sync = qqToYhSync
			}
			bytes, _ := json.Marshal(yhItems)
			_, _ = dbConn.Exec("UPDATE QQ_table SET YH_group_ids=? WHERE QQ_group_id=?", string(bytes), id)
		}
		return BindInfoResult{Status: 0, Msg: "所有绑定设置成功"}
	} else if platform == "YH" {
		var qqJSON sql.NullString
		_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", id).Scan(&qqJSON)
		if qqJSON.Valid && qqJSON.String != "" {
			var qqItems []BindingItem
			_ = json.Unmarshal([]byte(qqJSON.String), &qqItems)
			for i := range qqItems {
				qqItems[i].Sync = yhToQqSync
			}
			bytes, _ := json.Marshal(qqItems)
			_, _ = dbConn.Exec("UPDATE YH_table SET QQ_group_ids=? WHERE YH_group_id=?", string(bytes), id)
		}
		return BindInfoResult{Status: 0, Msg: "所有绑定设置成功"}
	}

	return BindInfoResult{Status: -1, Msg: "设置失败"}
}

type BindingDetail struct {
	QQGroupID  string `json:"qq_group_id"`
	YHGroupID  string `json:"yh_group_id"`
	QQToYHSync bool   `json:"qq_to_yh_sync"`
	YHToQQSync bool   `json:"yh_to_qq_sync"`
	SyncMode   string `json:"sync_mode"` // "全同步", "QQ到云湖", "云湖到QQ", "停止"
}

func GetAllBindings() []BindingDetail {
	dbMu.Lock()
	defer dbMu.Unlock()

	var result []BindingDetail
	if dbConn == nil {
		return result
	}

	rows, err := dbConn.Query("SELECT QQ_group_id, YH_group_ids FROM QQ_table")
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var qqID string
		var yhJSON sql.NullString
		if err := rows.Scan(&qqID, &yhJSON); err != nil {
			continue
		}

		var yhItems []BindingItem
		if yhJSON.Valid && yhJSON.String != "" {
			_ = json.Unmarshal([]byte(yhJSON.String), &yhItems)
		}

		for _, item := range yhItems {
			detail := BindingDetail{
				QQGroupID:  qqID,
				YHGroupID:  item.ID,
				QQToYHSync: item.Sync,
			}

			var qqBackJSON sql.NullString
			_ = dbConn.QueryRow("SELECT QQ_group_ids FROM YH_table WHERE YH_group_id=?", item.ID).Scan(&qqBackJSON)
			if qqBackJSON.Valid && qqBackJSON.String != "" {
				var qqItems []BindingItem
				_ = json.Unmarshal([]byte(qqBackJSON.String), &qqItems)
				for _, q := range qqItems {
					if q.ID == qqID {
						detail.YHToQQSync = q.Sync
						break
					}
				}
			}

			if detail.QQToYHSync && detail.YHToQQSync {
				detail.SyncMode = "全同步"
			} else if detail.QQToYHSync && !detail.YHToQQSync {
				detail.SyncMode = "QQ到云湖"
			} else if !detail.QQToYHSync && detail.YHToQQSync {
				detail.SyncMode = "云湖到QQ"
			} else {
				detail.SyncMode = "停止"
			}

			result = append(result, detail)
		}
	}

	return result
}
