package service

import (
	"time"

	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// RateLimitService manages per-client bandwidth rate limits.
// It persists limits in SQLite and (when sx-core is available) pushes them
// to the Xray gRPC RateLimitService.
type RateLimitService struct {
	ConfigSeqService ConfigSeqService
}

// Get returns the rate limit for the given email, or nil if none is set.
func (s *RateLimitService) Get(email string) (*model.ClientRateLimit, error) {
	db := database.GetDB()
	var rl model.ClientRateLimit
	err := db.Where("email = ?", email).First(&rl).Error
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &rl, nil
}

// Set creates or updates the rate limit for the given email.
func (s *RateLimitService) Set(email string, egressBps, ingressBps int64) (*model.ClientRateLimit, error) {
	return s.SetWithBurst(email, egressBps, ingressBps, 0, 0, 0, 0)
}

// SetWithBurst creates or updates the rate limit and burst window for one XrayCore email.
func (s *RateLimitService) SetWithBurst(
	email string,
	egressBps int64,
	ingressBps int64,
	burstEgressBps int64,
	burstIngressBps int64,
	burstDurationSeconds int64,
	burstCooldownSeconds int64,
) (*model.ClientRateLimit, error) {
	db := database.GetDB()
	now := time.Now().UnixMilli()

	var rl model.ClientRateLimit
	err := db.Where("email = ?", email).First(&rl).Error
	if err != nil {
		if database.IsNotFound(err) {
			rl = model.ClientRateLimit{
				Email:                email,
				EgressBps:            egressBps,
				IngressBps:           ingressBps,
				BurstEgressBps:       burstEgressBps,
				BurstIngressBps:      burstIngressBps,
				BurstDurationSeconds: burstDurationSeconds,
				BurstCooldownSeconds: burstCooldownSeconds,
				UpdatedAt:            now,
			}
			return &rl, db.Create(&rl).Error
		}
		return nil, err
	}

	rl.EgressBps = egressBps
	rl.IngressBps = ingressBps
	rl.BurstEgressBps = burstEgressBps
	rl.BurstIngressBps = burstIngressBps
	rl.BurstDurationSeconds = burstDurationSeconds
	rl.BurstCooldownSeconds = burstCooldownSeconds
	rl.UpdatedAt = now
	return &rl, db.Save(&rl).Error
}

// Remove deletes the rate limit for the given email.
func (s *RateLimitService) Remove(email string) error {
	db := database.GetDB()
	return db.Where("email = ?", email).Delete(&model.ClientRateLimit{}).Error
}

// GetAll returns all stored rate limits.
func (s *RateLimitService) GetAll() ([]model.ClientRateLimit, error) {
	db := database.GetDB()
	var rls []model.ClientRateLimit
	err := db.Find(&rls).Error
	return rls, err
}
