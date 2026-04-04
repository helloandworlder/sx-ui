package service

import (
	"encoding/json"
	"fmt"

	"github.com/helloandworlder/sx-ui/v2/database/model"
	"github.com/helloandworlder/sx-ui/v2/logger"
	"github.com/helloandworlder/sx-ui/v2/xray"
)

// XrayDynamicService provides gRPC-based dynamic management of Xray resources
// (Inbound/Outbound/Route/User) without requiring a full restart.
// Falls back to SetToNeedRestart() if gRPC call fails.
type XrayDynamicService struct {
	XrayService XrayService
}

func (s *XrayDynamicService) getAPI() (*xray.XrayAPI, error) {
	if !s.XrayService.IsXrayRunning() {
		return nil, fmt.Errorf("xray not running")
	}
	api := &xray.XrayAPI{}
	if err := api.Init(s.XrayService.GetAPIPort()); err != nil {
		return nil, err
	}
	return api, nil
}

// DynamicAddOutbound adds an outbound via gRPC. Falls back to restart on failure.
func (s *XrayDynamicService) DynamicAddOutbound(out *model.Outbound) {
	api, err := s.getAPI()
	if err != nil {
		logger.Debug("gRPC unavailable for outbound add, will restart:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	outJSON := buildOutboundJSON(out)
	if err := api.AddOutbound(outJSON); err != nil {
		logger.Warning("gRPC AddOutbound failed, falling back to restart:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: added outbound %s", out.Tag)
}

// DynamicDelOutbound removes an outbound via gRPC.
func (s *XrayDynamicService) DynamicDelOutbound(tag string) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.DelOutbound(tag); err != nil {
		logger.Warning("gRPC DelOutbound failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: removed outbound %s", tag)
}

// DynamicAddUser adds a user to an inbound via gRPC (for VMess/VLESS/Trojan/SS).
// If egressBps/ingressBps > 0, also sets the rate limit in sx-core.
func (s *XrayDynamicService) DynamicAddUser(proto string, inboundTag string, user map[string]any, egressBps, ingressBps int64) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.AddUser(proto, inboundTag, user); err != nil {
		logger.Warning("gRPC AddUser failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	email, _ := user["email"].(string)
	logger.Infof("gRPC: added user %s to %s", email, inboundTag)

	if egressBps > 0 || ingressBps > 0 {
		if err := api.SetUserRateLimit(email, egressBps, ingressBps); err != nil {
			logger.Warning("gRPC SetUserRateLimit failed:", err)
			s.XrayService.SetToNeedRestart()
			return
		}
		logger.Infof("gRPC: set ratelimit %s egress=%d bps ingress=%d bps", email, egressBps, ingressBps)
	}
}

// DynamicRemoveUser removes a user from an inbound via gRPC.
// Also removes the rate limit from sx-core.
func (s *XrayDynamicService) DynamicRemoveUser(inboundTag, email string) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.RemoveUser(inboundTag, email); err != nil {
		logger.Warning("gRPC RemoveUser failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: removed user %s from %s", email, inboundTag)

	if err := api.RemoveUserRateLimit(email); err != nil {
		logger.Warning("gRPC RemoveUserRateLimit failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: removed ratelimit %s", email)
}

// DynamicSetRateLimit sets or updates the rate limit for a user.
// This is called independently when only the rate limit changes (not the user).
// egressBps and ingressBps are in bytes/sec (both directions).
func (s *XrayDynamicService) DynamicSetRateLimit(email string, egressBps, ingressBps int64) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.SetUserRateLimit(email, egressBps, ingressBps); err != nil {
		logger.Warning("gRPC SetUserRateLimit failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: set ratelimit %s egress=%d bps ingress=%d bps", email, egressBps, ingressBps)
}

// DynamicRemoveRateLimit removes the rate limit for a user.
func (s *XrayDynamicService) DynamicRemoveRateLimit(email string) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.RemoveUserRateLimit(email); err != nil {
		logger.Warning("gRPC RemoveUserRateLimit failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: removed ratelimit %s", email)
}

// DynamicGetUserSpeed returns real-time speed for a user (bytes/sec).
func (s *XrayDynamicService) DynamicGetUserSpeed(email string) (egressBps, ingressBps int64) {
	api, err := s.getAPI()
	if err != nil {
		logger.Debug("gRPC unavailable for GetUserSpeed:", err)
		return 0, 0
	}
	defer api.Close()

	egressBps, ingressBps, err = api.GetUserSpeed(email)
	if err != nil {
		logger.Debug("gRPC GetUserSpeed failed:", err)
		return 0, 0
	}
	return egressBps, ingressBps
}

// DynamicAddRoute adds a routing rule via gRPC.
func (s *XrayDynamicService) DynamicAddRoute(ruleJSON string) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.AddRoutingRule([]byte(ruleJSON), true); err != nil {
		logger.Warning("gRPC AddRule failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Info("gRPC: added routing rule")
}

// DynamicDelRoute removes a routing rule via gRPC.
func (s *XrayDynamicService) DynamicDelRoute(ruleTag string) {
	api, err := s.getAPI()
	if err != nil {
		s.XrayService.SetToNeedRestart()
		return
	}
	defer api.Close()

	if err := api.DelRoutingRule(ruleTag); err != nil {
		logger.Warning("gRPC DelRule failed:", err)
		s.XrayService.SetToNeedRestart()
		return
	}
	logger.Infof("gRPC: removed routing rule %s", ruleTag)
}

func buildOutboundJSON(out *model.Outbound) []byte {
	obj := map[string]any{
		"tag":      out.Tag,
		"protocol": out.Protocol,
	}
	if out.Settings != "" {
		var settings any
		json.Unmarshal([]byte(out.Settings), &settings)
		obj["settings"] = settings
	}
	if out.SendThrough != "" {
		obj["sendThrough"] = out.SendThrough
	}
	data, _ := json.Marshal(obj)
	return data
}
