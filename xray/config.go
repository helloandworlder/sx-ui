package xray

import (
	"bytes"
	"encoding/json"

	"github.com/helloandworlder/sx-ui/v2/util/json_util"
)

// Config represents the complete Xray configuration structure.
// It contains all sections of an Xray config file including inbounds, outbounds, routing, etc.
// RateLimitEntry is an sx-core extension: per-user bandwidth limit injected into Xray config.
type RateLimitEntry struct {
	Email                string `json:"email"`
	EgressBps            int64  `json:"egressBps"`
	IngressBps           int64  `json:"ingressBps"`
	BurstEgressBps       int64  `json:"burstEgressBps,omitempty"`
	BurstIngressBps      int64  `json:"burstIngressBps,omitempty"`
	BurstDurationSeconds int64  `json:"burstDurationSeconds,omitempty"`
	BurstCooldownSeconds int64  `json:"burstCooldownSeconds,omitempty"`
}

type Config struct {
	LogConfig        json_util.RawMessage `json:"log"`
	RouterConfig     json_util.RawMessage `json:"routing"`
	DNSConfig        json_util.RawMessage `json:"dns"`
	InboundConfigs   []InboundConfig      `json:"inbounds"`
	OutboundConfigs  json_util.RawMessage `json:"outbounds"`
	Transport        json_util.RawMessage `json:"transport"`
	Policy           json_util.RawMessage `json:"policy"`
	API              json_util.RawMessage `json:"api"`
	Stats            json_util.RawMessage `json:"stats"`
	Reverse          json_util.RawMessage `json:"reverse"`
	FakeDNS          json_util.RawMessage `json:"fakedns"`
	Observatory      json_util.RawMessage `json:"observatory"`
	BurstObservatory json_util.RawMessage `json:"burstObservatory"`
	Metrics          json_util.RawMessage `json:"metrics"`
	// sx-core extension: rate limits loaded by Xray on startup
	RateLimits []RateLimitEntry `json:"rateLimits,omitempty"`
}

// Equals compares two Config instances for deep equality.
func (c *Config) Equals(other *Config) bool {
	if len(c.InboundConfigs) != len(other.InboundConfigs) {
		return false
	}
	for i, inbound := range c.InboundConfigs {
		if !inbound.Equals(&other.InboundConfigs[i]) {
			return false
		}
	}
	if !bytes.Equal(c.LogConfig, other.LogConfig) {
		return false
	}
	if !bytes.Equal(c.RouterConfig, other.RouterConfig) {
		return false
	}
	if !bytes.Equal(c.DNSConfig, other.DNSConfig) {
		return false
	}
	if !bytes.Equal(c.OutboundConfigs, other.OutboundConfigs) {
		return false
	}
	if !bytes.Equal(c.Transport, other.Transport) {
		return false
	}
	if !bytes.Equal(c.Policy, other.Policy) {
		return false
	}
	if !bytes.Equal(c.API, other.API) {
		return false
	}
	if !bytes.Equal(c.Stats, other.Stats) {
		return false
	}
	if !bytes.Equal(c.Reverse, other.Reverse) {
		return false
	}
	if !bytes.Equal(c.FakeDNS, other.FakeDNS) {
		return false
	}
	if !bytes.Equal(c.Metrics, other.Metrics) {
		return false
	}
	return true
}

func (c *Config) EnsureAPIServices(required ...string) {
	if len(c.API) == 0 || len(required) == 0 {
		return
	}

	var api map[string]any
	if err := json.Unmarshal(c.API, &api); err != nil || api == nil {
		return
	}

	rawServices, _ := api["services"].([]any)
	existing := make(map[string]bool, len(rawServices))
	for _, service := range rawServices {
		if name, ok := service.(string); ok {
			existing[name] = true
		}
	}

	changed := false
	for _, service := range required {
		if !existing[service] {
			rawServices = append(rawServices, service)
			changed = true
		}
	}
	if !changed {
		return
	}

	api["services"] = rawServices
	if data, err := json.Marshal(api); err == nil {
		c.API = data
	}
}
