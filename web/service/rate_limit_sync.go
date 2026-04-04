package service

import (
	"github.com/helloandworlder/sx-ui/v2/logger"
)

// RateLimitSyncService pushes all stored rate limits to the running Xray process
// over the sx-core RateLimitService gRPC API. It must be called after restarts
// because the in-memory limiter state lives inside the Xray subprocess.
type RateLimitSyncService struct {
	RateLimitService   RateLimitService
	XrayDynamicService XrayDynamicService
}

// SyncAllToXray reads all rate limits from SQLite and pushes them to the running Xray process over gRPC.
func (s *RateLimitSyncService) SyncAllToXray() {
	limits, err := s.RateLimitService.GetAll()
	if err != nil {
		logger.Warning("Failed to load rate limits for sync:", err)
		return
	}

	if len(limits) == 0 {
		return
	}

	for _, rl := range limits {
		s.XrayDynamicService.DynamicSetRateLimit(rl.Email, rl.EgressBps, rl.IngressBps)
	}

	logger.Infof("Synced %d rate limits to running Xray over gRPC", len(limits))
}

// PushSingle sets a single rate limit in the running Xray process.
func (s *RateLimitSyncService) PushSingle(email string, egressBps, ingressBps int64) {
	s.XrayDynamicService.DynamicSetRateLimit(email, egressBps, ingressBps)
}

// RemoveSingle removes a single rate limit from the running Xray process.
func (s *RateLimitSyncService) RemoveSingle(email string) {
	s.XrayDynamicService.DynamicRemoveRateLimit(email)
}
