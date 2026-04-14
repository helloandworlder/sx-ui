package service

import (
	"encoding/json"
	"fmt"
	"net"
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

func TestDefaultInstancePortsUseInstanceRanges(t *testing.T) {
	webPort, subPort, apiPort, metricsPort := defaultPortsForInstance("hk01")

	if webPort < 10000 || webPort > 19999 {
		t.Fatalf("unexpected web port range: %d", webPort)
	}
	if subPort < 20000 || subPort > 29999 {
		t.Fatalf("unexpected sub port range: %d", subPort)
	}
	if apiPort < 30000 || apiPort > 39999 {
		t.Fatalf("unexpected xray api port range: %d", apiPort)
	}
	if metricsPort < 40000 || metricsPort > 49999 {
		t.Fatalf("unexpected xray metrics port range: %d", metricsPort)
	}
	if webPort == 2053 {
		t.Fatalf("web port must not fall back to legacy default")
	}
	if subPort == 2096 {
		t.Fatalf("sub port must not fall back to legacy default")
	}
}

func TestDefaultInstancePortsRetreatFromBusyPorts(t *testing.T) {
	instance := "busy-hk01"
	preferredWebPort := preferredInstancePort(instance, webInstancePortBase, false)
	preferredSubPort := preferredInstancePort(instance, subInstancePortBase, false)
	preferredAPIPort := preferredInstancePort(instance, xrayInstanceAPIPortBase, true)
	preferredMetricsPort := preferredInstancePort(instance, xrayInstanceMetricsPortBase, true)

	listeners := []net.Listener{
		mustListen(t, fmt.Sprintf(":%d", preferredWebPort)),
		mustListen(t, fmt.Sprintf(":%d", preferredSubPort)),
		mustListen(t, fmt.Sprintf("127.0.0.1:%d", preferredAPIPort)),
		mustListen(t, fmt.Sprintf("127.0.0.1:%d", preferredMetricsPort)),
	}
	for _, listener := range listeners {
		defer listener.Close()
	}

	webPort, subPort, apiPort, metricsPort := defaultPortsForInstance(instance)
	if webPort == preferredWebPort {
		t.Fatalf("web port should retreat from busy preferred port %d", preferredWebPort)
	}
	if subPort == preferredSubPort {
		t.Fatalf("sub port should retreat from busy preferred port %d", preferredSubPort)
	}
	if apiPort == preferredAPIPort {
		t.Fatalf("api port should retreat from busy preferred port %d", preferredAPIPort)
	}
	if metricsPort == preferredMetricsPort {
		t.Fatalf("metrics port should retreat from busy preferred port %d", preferredMetricsPort)
	}
}

func mustListen(t *testing.T, address string) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen %s: %v", address, err)
	}
	return listener
}
