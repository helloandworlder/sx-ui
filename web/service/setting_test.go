package service

import (
	"encoding/json"
	"testing"
)

func readTemplateAPIPort(t *testing.T, template string) int {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(template), &payload); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	inbounds, ok := payload["inbounds"].([]any)
	if !ok {
		t.Fatalf("template missing inbounds")
	}
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		if inbound["tag"] == "api" {
			port, ok := inbound["port"].(float64)
			if !ok {
				t.Fatalf("api inbound missing numeric port")
			}
			return int(port)
		}
	}

	t.Fatalf("template missing api inbound")
	return 0
}

func readTemplateMetricsListen(t *testing.T, template string) string {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(template), &payload); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	metrics, ok := payload["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("template missing metrics")
	}
	listen, ok := metrics["listen"].(string)
	if !ok {
		t.Fatalf("template missing metrics.listen")
	}
	return listen
}

func TestInstanceScopedXrayTemplateUsesNonLegacyPorts(t *testing.T) {
	template := buildInstanceScopedXrayTemplateConfig("hk01")

	if port := readTemplateAPIPort(t, template); port == 62789 {
		t.Fatalf("api port should not use shared legacy default")
	}
	if listen := readTemplateMetricsListen(t, template); listen == "127.0.0.1:11111" {
		t.Fatalf("metrics listen should not use shared legacy default")
	}
}

func TestSetXrayInternalPortsOnTemplateUsesRequestedPorts(t *testing.T) {
	updated, err := setXrayInternalPortsOnTemplate(xrayTemplateConfig, 31111, 41111)
	if err != nil {
		t.Fatalf("setXrayInternalPortsOnTemplate: %v", err)
	}

	if port := readTemplateAPIPort(t, updated); port != 31111 {
		t.Fatalf("unexpected api port: %d", port)
	}
	if listen := readTemplateMetricsListen(t, updated); listen != "127.0.0.1:41111" {
		t.Fatalf("unexpected metrics listen: %s", listen)
	}
}
