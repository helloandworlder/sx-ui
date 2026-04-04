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

	// Gather all state that defines this node's configuration
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

	// Build a deterministic hash input
	state := struct {
		Inbounds   []model.Inbound         `json:"i"`
		Outbounds  []model.Outbound        `json:"o"`
		Routes     []model.RoutingRule     `json:"r"`
		RateLimits []model.ClientRateLimit `json:"l"`
		NodeMeta   map[string]string       `json:"m"`
	}{inbounds, outbounds, routes, rateLimits, nodeMeta}

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
