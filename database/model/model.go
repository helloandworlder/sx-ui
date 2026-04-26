// Package model defines the database models and data structures used by the 3x-ui panel.
package model

import (
	"fmt"

	"github.com/helloandworlder/sx-ui/v2/util/json_util"
	"github.com/helloandworlder/sx-ui/v2/xray"
)

// Protocol represents the protocol type for Xray inbounds.
type Protocol string

// Protocol constants for different Xray inbound protocols
const (
	VMESS       Protocol = "vmess"
	VLESS       Protocol = "vless"
	Tunnel      Protocol = "tunnel"
	HTTP        Protocol = "http"
	Socks       Protocol = "socks"
	Trojan      Protocol = "trojan"
	Shadowsocks Protocol = "shadowsocks"
	Mixed       Protocol = "mixed"
	Hysteria    Protocol = "hysteria"
	WireGuard   Protocol = "wireguard"
)

func (p Protocol) Normalize() Protocol {
	switch p {
	case "socks5":
		return Socks
	default:
		return p
	}
}

// User represents a user account in the 3x-ui panel.
type User struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Inbound represents an Xray inbound configuration with traffic statistics and settings.
type Inbound struct {
	Id                   int                  `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`                                                    // Unique identifier
	UserId               int                  `json:"-"`                                                                                               // Associated user ID
	Up                   int64                `json:"up" form:"up"`                                                                                    // Upload traffic in bytes
	Down                 int64                `json:"down" form:"down"`                                                                                // Download traffic in bytes
	Total                int64                `json:"total" form:"total"`                                                                              // Total traffic limit in bytes
	AllTime              int64                `json:"allTime" form:"allTime" gorm:"default:0"`                                                         // All-time traffic usage
	Remark               string               `json:"remark" form:"remark"`                                                                            // Human-readable remark
	Enable               bool                 `json:"enable" form:"enable" gorm:"index:idx_enable_traffic_reset,priority:1"`                           // Whether the inbound is enabled
	ExpiryTime           int64                `json:"expiryTime" form:"expiryTime"`                                                                    // Expiration timestamp
	TrafficReset         string               `json:"trafficReset" form:"trafficReset" gorm:"default:never;index:idx_enable_traffic_reset,priority:2"` // Traffic reset schedule
	LastTrafficResetTime int64                `json:"lastTrafficResetTime" form:"lastTrafficResetTime" gorm:"default:0"`                               // Last traffic reset timestamp
	ClientStats          []xray.ClientTraffic `gorm:"foreignKey:InboundId;references:Id" json:"clientStats" form:"clientStats"`                        // Client traffic statistics

	// Xray configuration fields
	Listen         string   `json:"listen" form:"listen"`
	Port           int      `json:"port" form:"port"`
	Protocol       Protocol `json:"protocol" form:"protocol"`
	Settings       string   `json:"settings" form:"settings"`
	StreamSettings string   `json:"streamSettings" form:"streamSettings"`
	Tag            string   `json:"tag" form:"tag" gorm:"unique"`
	Sniffing       string   `json:"sniffing" form:"sniffing"`
}

// OutboundTraffics tracks traffic statistics for Xray outbound connections.
type OutboundTraffics struct {
	Id    int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Tag   string `json:"tag" form:"tag" gorm:"unique"`
	Up    int64  `json:"up" form:"up" gorm:"default:0"`
	Down  int64  `json:"down" form:"down" gorm:"default:0"`
	Total int64  `json:"total" form:"total" gorm:"default:0"`
}

// InboundClientIps stores IP addresses associated with inbound clients for access control.
type InboundClientIps struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientEmail string `json:"clientEmail" form:"clientEmail" gorm:"unique"`
	Ips         string `json:"ips" form:"ips"`
}

// HistoryOfSeeders tracks which database seeders have been executed to prevent re-running.
type HistoryOfSeeders struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	SeederName string `json:"seederName"`
}

// GenXrayInboundConfig generates an Xray inbound configuration from the Inbound model.
func (i *Inbound) GenXrayInboundConfig() *xray.InboundConfig {
	listen := i.Listen
	// Default to 0.0.0.0 (all interfaces) when listen is empty
	// This ensures proper dual-stack IPv4/IPv6 binding in systems where bindv6only=0
	if listen == "" {
		listen = "0.0.0.0"
	}
	listen = fmt.Sprintf("\"%v\"", listen)
	return &xray.InboundConfig{
		Listen:         json_util.RawMessage(listen),
		Port:           i.Port,
		Protocol:       string(i.Protocol.Normalize()),
		Settings:       json_util.RawMessage(i.Settings),
		StreamSettings: json_util.RawMessage(i.StreamSettings),
		Tag:            i.Tag,
		Sniffing:       json_util.RawMessage(i.Sniffing),
	}
}

// Setting stores key-value configuration settings for the 3x-ui panel.
type Setting struct {
	Id    int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}

// CustomGeoResource registers an externally-fetched geo data file
// (geosite/geoip) so admins can hot-reload custom routing data.
type CustomGeoResource struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Type          string `json:"type" gorm:"not null;uniqueIndex:idx_custom_geo_type_alias;column:geo_type"`
	Alias         string `json:"alias" gorm:"not null;uniqueIndex:idx_custom_geo_type_alias"`
	Url           string `json:"url" gorm:"not null"`
	LocalPath     string `json:"localPath" gorm:"column:local_path"`
	LastUpdatedAt int64  `json:"lastUpdatedAt" gorm:"default:0;column:last_updated_at"`
	LastModified  string `json:"lastModified" gorm:"column:last_modified"`
	CreatedAt     int64  `json:"createdAt" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt     int64  `json:"updatedAt" gorm:"autoUpdateTime;column:updated_at"`
}

// ---------------------------------------------------------------------------
// sx-ui extensions: models added for GoSea-managed node orchestration
// ---------------------------------------------------------------------------

// ConfigSequence tracks a monotonically increasing counter that is bumped on
// every Inbound / Outbound / RoutingRule mutation.  GoSea compares its own
// last-known seq against the node's current seq to decide whether an
// incremental or full sync is required.
type ConfigSequence struct {
	Id   int    `json:"id" gorm:"primaryKey"`
	Seq  int64  `json:"seq" gorm:"default:0"`
	Hash string `json:"hash" gorm:"default:''"` // SHA256 of current config state
}

// ClientRateLimit stores per-client bandwidth limits (bytes/sec).
// The email field maps 1-to-1 with the Xray client email.
type ClientRateLimit struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Email      string `json:"email" gorm:"uniqueIndex"`
	EgressBps  int64  `json:"egressBps"`  // max egress bytes per second
	IngressBps int64  `json:"ingressBps"` // max ingress bytes per second
	UpdatedAt  int64  `json:"updatedAt"`
}

// Outbound represents an Xray outbound configuration.
// In stock 3x-ui outbounds live inside the xrayTemplateConfig JSON blob;
// sx-ui promotes them to first-class DB rows so GoSea can CRUD them via API.
type Outbound struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Tag         string `json:"tag" gorm:"uniqueIndex"`
	Protocol    string `json:"protocol"`    // freedom, socks, blackhole, ...
	Settings    string `json:"settings"`    // raw JSON (protocol-specific)
	SendThrough string `json:"sendThrough"` // bind to specific local IP (residential)
	Enabled     bool   `json:"enabled" gorm:"default:true"`
	Seq         int64  `json:"seq"` // configSeq at time of create/update
}

// RoutingRule represents a single Xray routing rule.
// Like Outbound, these are promoted from the JSON template to individual rows.
type RoutingRule struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Priority int    `json:"priority" gorm:"index"` // lower = matched first
	RuleJson string `json:"ruleJson"`              // full Xray rule JSON object
	Enabled  bool   `json:"enabled" gorm:"default:true"`
	Seq      int64  `json:"seq"` // configSeq at time of create/update
}

// NodeMeta is a generic key-value store for node-level metadata.
// Well-known keys:
//   - api_key          – bearer token for the REST /api/v1 endpoints
//   - node_type        – "residential" or "dedicated"
//   - public_ips       – JSON array of detected egress IPs
//   - geoip_block_cn   – "true" / "false"
type NodeMeta struct {
	Id    int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" gorm:"uniqueIndex"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------

// Client represents a client configuration for Xray inbounds with traffic limits and settings.
type Client struct {
	ID         string `json:"id"`                           // Unique client identifier
	Security   string `json:"security"`                     // Security method (e.g., "auto", "aes-128-gcm")
	Method     string `json:"method,omitempty"`             // Cipher / method for multi-user shadowsocks
	Auth       string `json:"auth,omitempty"`               // Auth secret for hysteria2
	Password   string `json:"password"`                     // Client password
	Flow       string `json:"flow"`                         // Flow control (XTLS)
	Email      string `json:"email"`                        // Client email identifier
	EgressBps  int64  `json:"egressBps,omitempty"`          // Upload limit in bytes per second
	IngressBps int64  `json:"ingressBps,omitempty"`         // Download limit in bytes per second
	LimitIP    int    `json:"limitIp"`                      // IP limit for this client
	TotalGB    int64  `json:"totalGB" form:"totalGB"`       // Total traffic limit in GB
	ExpiryTime int64  `json:"expiryTime" form:"expiryTime"` // Expiration timestamp
	Enable     bool   `json:"enable" form:"enable"`         // Whether the client is enabled
	TgID       int64  `json:"tgId" form:"tgId"`             // Telegram user ID for notifications
	SubID      string `json:"subId" form:"subId"`           // Subscription identifier
	Comment    string `json:"comment" form:"comment"`       // Client comment
	Reset      int    `json:"reset" form:"reset"`           // Reset period in days
	CreatedAt  int64  `json:"created_at,omitempty"`         // Creation timestamp
	UpdatedAt  int64  `json:"updated_at,omitempty"`         // Last update timestamp
}
