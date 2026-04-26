package service

import (
	"encoding/json"
	"testing"

	"github.com/helloandworlder/sx-ui/v2/database/model"
)

func TestBuildJSONForProtocolUsesNumericTelegramID(t *testing.T) {
	client_Id = "client-id"
	client_Flow = ""
	client_Email = "user@example.com"
	client_LimitIP = 1
	client_TotalGB = 1024
	client_ExpiryTime = 1234567890
	client_Enable = true
	client_TgID = "123456789"
	client_SubID = "subid123"
	client_Comment = "comment"
	client_Reset = 0
	client_Security = "auto"
	client_ShPassword = "shadow-pass"
	client_TrPassword = "trojan-pass"
	client_Method = "aes-256-gcm"

	tg := (&Tgbot{}).NewTgbot()
	protocols := []model.Protocol{model.VMESS, model.VLESS, model.Trojan, model.Shadowsocks}

	for _, protocol := range protocols {
		jsonString, err := tg.BuildJSONForProtocol(protocol)
		if err != nil {
			t.Fatalf("protocol %s build json: %v", protocol, err)
		}

		var payload struct {
			Clients []model.Client `json:"clients"`
		}
		if err := json.Unmarshal([]byte(jsonString), &payload); err != nil {
			t.Fatalf("protocol %s unmarshal: %v\njson=%s", protocol, err, jsonString)
		}
		if len(payload.Clients) != 1 {
			t.Fatalf("protocol %s expected exactly one client, got %d", protocol, len(payload.Clients))
		}
		if payload.Clients[0].TgID != 123456789 {
			t.Fatalf("protocol %s expected tgId 123456789, got %d", protocol, payload.Clients[0].TgID)
		}
	}
}

func TestBuildJSONForProtocolFallsBackToZeroTelegramID(t *testing.T) {
	client_Id = "client-id"
	client_Flow = ""
	client_Email = "user@example.com"
	client_LimitIP = 1
	client_TotalGB = 1024
	client_ExpiryTime = 1234567890
	client_Enable = true
	client_TgID = "  "
	client_SubID = "subid123"
	client_Comment = "comment"
	client_Reset = 0
	client_Security = "auto"
	client_ShPassword = "shadow-pass"
	client_TrPassword = "trojan-pass"
	client_Method = "aes-256-gcm"

	tg := (&Tgbot{}).NewTgbot()
	jsonString, err := tg.BuildJSONForProtocol(model.VMESS)
	if err != nil {
		t.Fatalf("build json: %v", err)
	}

	var payload struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(jsonString), &payload); err != nil {
		t.Fatalf("unmarshal: %v\njson=%s", err, jsonString)
	}
	if len(payload.Clients) != 1 {
		t.Fatalf("expected exactly one client, got %d", len(payload.Clients))
	}
	if payload.Clients[0].TgID != 0 {
		t.Fatalf("expected empty tgId to normalize to 0, got %d", payload.Clients[0].TgID)
	}
}
