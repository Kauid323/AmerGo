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

func TestYunhuFormInstructionUnmarshal(t *testing.T) {
	rawJSON := `{"version":"1.0","header":{"eventId":"99df80704b9f410cb083ed482cf0a8ff","eventType":"message.receive.instruction","eventTime":1788705034360},"event":{"sender":{"senderId":"8516939","senderType":"user","senderUserLevel":"owner","senderNickname":"那狗吧","senderAvatarUrl":"https://chat-img.jwznb.com/defalut-avatars/Nellie%20Bly.png"},"chat":{"chatId":"857744874","chatType":"group"},"message":{"msgId":"ef2900dde38e43e5826a0111ffa0b236","parentId":"","sendTime":1788705034317,"chatId":"857744874","chatType":"group","contentType":"form","content":{"formJson":{"uhorxv":{"label":"平台","selectIndex":0,"selectValue":"QQ","id":"uhorxv","type":"radio"},"zsvovb":{"id":"zsvovb","type":"input","label":"QQ群号","value":"767676465"}}},"instructionId":1749,"instructionName":"绑定","commandId":1749,"commandName":"绑定"}}}`

	var event YunhuEvent
	err := json.Unmarshal([]byte(rawJSON), &event)
	if err != nil {
		t.Fatalf("解析云湖表单指令失败: %v", err)
	}

	cmdName := event.Event.Message.CommandName.String()
	if cmdName != "绑定" {
		t.Errorf("期望 commandName 为 绑定，实际为: %s", cmdName)
	}

	qqGroup := event.Event.Message.Content.GetFormFieldString("QQ群号")
	if qqGroup != "767676465" {
		t.Errorf("期望 GetFormFieldString('QQ群号') 为 767676465，实际为: %s", qqGroup)
	}

	fuzzyGroup := event.Event.Message.Content.GetFormFieldByFuzzyLabel("群号")
	if fuzzyGroup != "767676465" {
		t.Errorf("期望 GetFormFieldByFuzzyLabel('群号') 为 767676465，实际为: %s", fuzzyGroup)
	}

	firstInput := event.Event.Message.Content.GetFirstInputFieldValue()
	if firstInput != "767676465" {
		t.Errorf("期望 GetFirstInputFieldValue() 为 767676465，实际为: %s", firstInput)
	}
}

