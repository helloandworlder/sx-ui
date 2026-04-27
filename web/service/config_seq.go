package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// ConfigSeqService manages the monotonically increasing configuration
// sequence number.  Every mutation to Inbound / Outbound / RoutingRule
// must call BumpSeq() so GoSea can detect changes.
type ConfigSeqService struct{}

// SeqInfo is the lightweight response for the polling endpoint.
type SeqInfo struct {
	Seq  int64  `json:"seq"`
	Hash string `json:"hash"`
}

type configHashInbound struct {
	Remark         string         `json:"remark"`
	Enable         bool           `json:"enable"`
	ExpiryTime     int64          `json:"expiryTime"`
	Listen         string         `json:"listen"`
	Port           int            `json:"port"`
	Protocol       model.Protocol `json:"protocol"`
	Settings       string         `json:"settings"`
	StreamSettings string         `json:"streamSettings"`
	Tag            string         `json:"tag"`
	Sniffing       string         `json:"sniffing"`
}

type configHashOutbound struct {
	Tag         string `json:"tag"`
	Protocol    string `json:"protocol"`
	Settings    string `json:"settings"`
	SendThrough string `json:"sendThrough"`
	Enabled     bool   `json:"enabled"`
}

type configHashRoute struct {
	Priority int    `json:"priority"`
	RuleJson string `json:"ruleJson"`
	Enabled  bool   `json:"enabled"`
}

type configHashRateLimit struct {
	Email                string `json:"email"`
	EgressBps            int64  `json:"egressBps"`
	IngressBps           int64  `json:"ingressBps"`
	BurstEgressBps       int64  `json:"burstEgressBps"`
	BurstIngressBps      int64  `json:"burstIngressBps"`
	BurstDurationSeconds int64  `json:"burstDurationSeconds"`
	BurstCooldownSeconds int64  `json:"burstCooldownSeconds"`
}

// GetSeqInfo returns {seq, hash} for the lightweight polling endpoint.
func (s *ConfigSeqService) GetSeqInfo() (*SeqInfo, error) {
	db := database.GetDB()
	var cs model.ConfigSequence
	err := db.First(&cs, 1).Error
	if err != nil {
		return nil, err
	}
	return &SeqInfo{Seq: cs.Seq, Hash: cs.Hash}, nil
}

// GetSeq returns the current configuration sequence number.
func (s *ConfigSeqService) GetSeq() (int64, error) {
	db := database.GetDB()
	var cs model.ConfigSequence
	err := db.First(&cs, 1).Error
	if err != nil {
		return 0, err
	}
	return cs.Seq, nil
}

// BumpSeq atomically increments the sequence number and returns the new value.
func (s *ConfigSeqService) BumpSeq() (int64, error) {
	db := database.GetDB()
	err := db.Model(&model.ConfigSequence{}).Where("id = 1").
		Update("seq", database.GetDB().Raw("seq + 1")).Error
	if err != nil {
		return 0, err
	}
	return s.GetSeq()
}

// UpdateHash recomputes the config state hash from all managed resources.
// Called after any mutation to keep the hash in sync with the seq.
func (s *ConfigSeqService) UpdateHash() error {
	db := database.GetDB()

	// Gather only stable configuration state. Dynamic traffic counters must not
	// participate in the hash, otherwise normal usage causes perpetual drift.
	var inbounds []model.Inbound
	db.Order("id").Find(&inbounds)

	var outbounds []model.Outbound
	db.Order("id").Find(&outbounds)

	var routes []model.RoutingRule
	db.Order("priority, id").Find(&routes)

	var rateLimits []model.ClientRateLimit
	db.Order("email").Find(&rateLimits)

	var nodeMetaRows []model.NodeMeta
	db.Where("key NOT IN ?", []string{"api_key", "public_ips"}).Order("key").Find(&nodeMetaRows)
	nodeMeta := make(map[string]string, len(nodeMetaRows))
	for _, row := range nodeMetaRows {
		nodeMeta[row.Key] = row.Value
	}

	hashInbounds := make([]configHashInbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		hashInbounds = append(hashInbounds, configHashInbound{
			Remark:         inbound.Remark,
			Enable:         inbound.Enable,
			ExpiryTime:     inbound.ExpiryTime,
			Listen:         inbound.Listen,
			Port:           inbound.Port,
			Protocol:       inbound.Protocol,
			Settings:       inbound.Settings,
			StreamSettings: inbound.StreamSettings,
			Tag:            inbound.Tag,
			Sniffing:       inbound.Sniffing,
		})
	}

	hashOutbounds := make([]configHashOutbound, 0, len(outbounds))
	for _, outbound := range outbounds {
		hashOutbounds = append(hashOutbounds, configHashOutbound{
			Tag:         outbound.Tag,
			Protocol:    outbound.Protocol,
			Settings:    outbound.Settings,
			SendThrough: outbound.SendThrough,
			Enabled:     outbound.Enabled,
		})
	}

	hashRoutes := make([]configHashRoute, 0, len(routes))
	for _, route := range routes {
		hashRoutes = append(hashRoutes, configHashRoute{
			Priority: route.Priority,
			RuleJson: route.RuleJson,
			Enabled:  route.Enabled,
		})
	}

	hashRateLimits := make([]configHashRateLimit, 0, len(rateLimits))
	for _, rateLimit := range rateLimits {
		hashRateLimits = append(hashRateLimits, configHashRateLimit{
			Email:                rateLimit.Email,
			EgressBps:            rateLimit.EgressBps,
			IngressBps:           rateLimit.IngressBps,
			BurstEgressBps:       rateLimit.BurstEgressBps,
			BurstIngressBps:      rateLimit.BurstIngressBps,
			BurstDurationSeconds: rateLimit.BurstDurationSeconds,
			BurstCooldownSeconds: rateLimit.BurstCooldownSeconds,
		})
	}

	// Build a deterministic hash input.
	state := struct {
		Inbounds   []configHashInbound   `json:"i"`
		Outbounds  []configHashOutbound  `json:"o"`
		Routes     []configHashRoute     `json:"r"`
		RateLimits []configHashRateLimit `json:"l"`
		NodeMeta   map[string]string     `json:"m"`
	}{hashInbounds, hashOutbounds, hashRoutes, hashRateLimits, nodeMeta}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return db.Model(&model.ConfigSequence{}).Where("id = 1").
		Update("hash", hash).Error
}

// BumpSeqAndHash increments the seq and recalculates the hash.
// This is the primary method to call after any config mutation.
func (s *ConfigSeqService) BumpSeqAndHash() (int64, error) {
	seq, err := s.BumpSeq()
	if err != nil {
		return 0, err
	}
	if err := s.UpdateHash(); err != nil {
		return seq, err
	}
	return seq, nil
}
