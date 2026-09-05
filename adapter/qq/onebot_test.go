package qq

import (
	"testing"
)

func TestOneBotMemberCache(t *testing.T) {
	server := &OneBotServer{
		memberCache:  make(map[string]memberCacheItem),
		groupCache:   make(map[int64]groupCacheItem),
		SelfID:       3218936228,
		SelfNickname: "Amer",
	}

	// 1. 测试从缓存更新和读取
	server.UpdateMemberNameCache(1013637348, 241638640, "那狗吧")
	name := server.GetGroupMemberName(1013637348, 241638640)
	if name != "那狗吧" {
		t.Errorf("expected '那狗吧', got '%s'", name)
	}

	// 2. 测试机器人自身默认昵称兜底
	selfName := server.GetGroupMemberName(1013637348, 3218936228)
	if selfName != "Amer" {
		t.Errorf("expected 'Amer', got '%s'", selfName)
	}

	// 3. 测试群名片覆盖
	server.UpdateMemberNameCache(1013637348, 3218936228, "Amer(QQ云湖互通机器人..)")
	cardName := server.GetGroupMemberName(1013637348, 3218936228)
	if cardName != "Amer(QQ云湖互通机器人..)" {
		t.Errorf("expected 'Amer(QQ云湖互通机器人..)', got '%s'", cardName)
	}
}
