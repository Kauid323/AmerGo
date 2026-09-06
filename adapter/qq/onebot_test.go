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

func TestCheckAndExtractQQCommand(t *testing.T) {
	botSelfID := int64(3218936228)
	botQQ := "3218936228"
	botName := "amer"

	tests := []struct {
		input     string
		wantCmd   bool
		wantText  string
		wantAtBot bool
	}{
		// 1. Direct slash commands
		{input: "/绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: false},
		{input: "/帮助", wantCmd: true, wantText: "/帮助", wantAtBot: false},

		// 2. Direct commands without slash
		{input: "绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: false},
		{input: "帮助", wantCmd: true, wantText: "/帮助", wantAtBot: false},
		{input: "绑定列表", wantCmd: true, wantText: "/绑定列表", wantAtBot: false},

		// 3. AT text + slash command
		{input: "@Amer /绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: true},
		{input: "@Amer(QQ云湖互通机器人) /绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: true},

		// 4. AT text + command without slash
		{input: "@Amer 绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: true},
		{input: "@Amer 帮助", wantCmd: true, wantText: "/帮助", wantAtBot: true},

		// 5. CQ:at + command
		{input: "[CQ:at,qq=3218936228] /绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: true},
		{input: "[CQ:at,qq=3218936228,name=Amer] 绑定 yh 921307662", wantCmd: true, wantText: "/绑定 yh 921307662", wantAtBot: true},
		{input: "[CQ:at,qq=3218936228] 帮助", wantCmd: true, wantText: "/帮助", wantAtBot: true},

		// 6. Admin commands
		{input: "@Amer /amer-block-qq-123456", wantCmd: true, wantText: "/amer-block-qq-123456", wantAtBot: true},
		{input: "@Amer amer-block-qq-123456", wantCmd: true, wantText: "/amer-block-qq-123456", wantAtBot: true},

		// 7. AT bot without command
		{input: "@Amer", wantCmd: false, wantText: "", wantAtBot: true},
		{input: "@Amer 你好呀", wantCmd: false, wantText: "你好呀", wantAtBot: true},
		{input: "[CQ:at,qq=3218936228] 早上好", wantCmd: false, wantText: "早上好", wantAtBot: true},

		// 8. Normal message
		{input: "今天天气真好", wantCmd: false, wantText: "", wantAtBot: false},
		{input: "@其他用户 你好", wantCmd: false, wantText: "", wantAtBot: false},
		{input: "[CQ:at,qq=12345678] /绑定 yh 123", wantCmd: false, wantText: "", wantAtBot: false},
	}

	for _, tt := range tests {
		isCmd, cmdText, isAtBot := CheckAndExtractQQCommand(tt.input, botSelfID, botQQ, botName)
		if isCmd != tt.wantCmd || cmdText != tt.wantText || isAtBot != tt.wantAtBot {
			t.Errorf("CheckAndExtractQQCommand(%q) = (%v, %q, %v), want (%v, %q, %v)",
				tt.input, isCmd, cmdText, isAtBot, tt.wantCmd, tt.wantText, tt.wantAtBot)
		}
	}
}
