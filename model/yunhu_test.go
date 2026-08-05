package model

import (
	"encoding/json"
	"testing"
)

func TestYunhuEventUnmarshal(t *testing.T) {
	rawJSON := `{"version":"1.0","header":{"eventId":"54e49872571f46d28fa10935e106c032","eventType":"message.receive.instruction","eventTime":1785686167990},"event":{"sender":{"senderId":"8516939","senderType":"user","senderUserLevel":"owner","senderNickname":"那狗吧","senderAvatarUrl":"https://chat-img.jwznb.com/defalut-avatars/Nellie%20Bly.png"},"chat":{"chatId":"857744874","chatType":"group"},"message":{"msgId":"1dad08d0b8ca4c8888863a651e7bd827","parentId":"","sendTime":1785686167954,"chatId":"857744874","chatType":"group","contentType":"text","content":{"text":"/帮助"},"instructionId":1746,"instructionName":"帮助","commandId":1746,"commandName":"帮助"}}}`

	var event YunhuEvent
	err := json.Unmarshal([]byte(rawJSON), &event)
	if err != nil {
		t.Fatalf("解析云湖指令消息失败: %v", err)
	}

	if event.Header.EventType.String() != "message.receive.instruction" {
		t.Errorf("期望 eventType 为 message.receive.instruction，实际为: %s", event.Header.EventType)
	}

	if event.Event.Message.InstructionID.String() != "1746" {
		t.Errorf("期望 instructionId 为 1746，实际为: %s", event.Event.Message.InstructionID)
	}

	if event.Event.GetChatID() != "857744874" {
		t.Errorf("期望 chatId 为 857744874，实际为: %s", event.Event.GetChatID())
	}

	if event.Event.Message.CommandName.String() != "帮助" {
		t.Errorf("期望 commandName 为 帮助，实际为: %s", event.Event.Message.CommandName)
	}
}
