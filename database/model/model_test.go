package model

import "testing"

func TestInboundGenXrayInboundConfigNormalizesLegacySocks5Protocol(t *testing.T) {
	inbound := &Inbound{
		Listen:   "",
		Port:     62004,
		Protocol: Protocol("socks5"),
		Settings: `{"auth":"noauth","udp":true}`,
		Tag:      "legacy-socks5",
		Sniffing: `{"enabled":true}`,
	}

	cfg := inbound.GenXrayInboundConfig()
	if cfg.Protocol != "socks" {
		t.Fatalf("expected legacy socks5 protocol to normalize to socks, got %q", cfg.Protocol)
	}
}
