package service

import (
	"encoding/json"
	"errors"
	"runtime"
	"sync"

	"github.com/helloandworlder/sx-ui/v2/logger"
	"github.com/helloandworlder/sx-ui/v2/xray"

	"go.uber.org/atomic"
)

var (
	p                 *xray.Process
	lock              sync.Mutex
	isNeedXrayRestart atomic.Bool // Indicates that restart was requested for Xray
	isManuallyStopped atomic.Bool // Indicates that Xray was stopped manually from the panel
	result            string
)

// XrayService provides business logic for Xray process management.
// It handles starting, stopping, restarting Xray, and managing its configuration.
type XrayService struct {
	inboundService      InboundService
	settingService      SettingService
	outboundCrudService OutboundCrudService
	routingCrudService  RoutingCrudService
	nodeMetaService     NodeMetaService
	xrayAPI             xray.XrayAPI
}

// IsXrayRunning checks if the Xray process is currently running.
func (s *XrayService) IsXrayRunning() bool {
	return p != nil && p.IsRunning()
}

// GetXrayErr returns the error from the Xray process, if any.
func (s *XrayService) GetXrayErr() error {
	if p == nil {
		return nil
	}

	err := p.GetErr()
	if err == nil {
		return nil
	}

	if runtime.GOOS == "windows" && err.Error() == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return nil
	}

	return err
}

// GetXrayResult returns the result string from the Xray process.
func (s *XrayService) GetXrayResult() string {
	if result != "" {
		return result
	}
	if s.IsXrayRunning() {
		return ""
	}
	if p == nil {
		return ""
	}

	result = p.GetResult()

	if runtime.GOOS == "windows" && result == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return ""
	}

	return result
}

// GetAPIPort returns the Xray gRPC API port for dynamic management.
func (s *XrayService) GetAPIPort() int {
	if p == nil {
		return 0
	}
	return p.GetAPIPort()
}

// GetXrayVersion returns the version of the running Xray process.
func (s *XrayService) GetXrayVersion() string {
	if p == nil {
		return "Unknown"
	}
	return p.GetVersion()
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig retrieves and builds the Xray configuration from settings and inbounds.
func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}
	xrayConfig.EnsureAPIServices("HandlerService", "LoggerService", "StatsService", "RoutingService", "RateLimitService", "ReverseService")

	s.inboundService.AddTraffic(nil, nil)

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		// get settings clients
		settings := map[string]any{}
		json.Unmarshal([]byte(inbound.Settings), &settings)
		if inbound.Protocol == "http" || inbound.Protocol == "socks" || inbound.Protocol == "mixed" {
			accounts, ok := settings["accounts"].([]any)
			if ok {
				clientStats := inbound.ClientStats
				enabledByEmail := make(map[string]bool, len(clientStats))
				for _, clientTraffic := range clientStats {
					enabledByEmail[clientTraffic.Email] = clientTraffic.Enable
				}

				finalAccounts := make([]any, 0, len(accounts))
				for _, account := range accounts {
					c, ok := account.(map[string]any)
					if !ok {
						continue
					}
					email, _ := c["email"].(string)
					if email == "" {
						email, _ = c["user"].(string)
					}
					if enabled, exists := enabledByEmail[email]; exists && !enabled {
						continue
					}
					if rawEnable, ok := c["enable"].(bool); ok && !rawEnable {
						continue
					}
					finalAccounts = append(finalAccounts, map[string]any{
						"user":  c["user"],
						"pass":  c["pass"],
						"email": email,
					})
				}

				settings["accounts"] = finalAccounts
				modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
				if err != nil {
					return nil, err
				}
				inbound.Settings = string(modifiedSettings)
			}
		}
		clients, ok := settings["clients"].([]any)
		if ok {
			// check users active or not
			clientStats := inbound.ClientStats
			for _, clientTraffic := range clientStats {
				indexDecrease := 0
				for index, client := range clients {
					c := client.(map[string]any)
					if c["email"] == clientTraffic.Email {
						if !clientTraffic.Enable {
							clients = RemoveIndex(clients, index-indexDecrease)
							indexDecrease++
							logger.Infof("Remove Inbound User %s due to expiration or traffic limit", c["email"])
						}
					}
				}
			}

			// clear client config for additional parameters
			var final_clients []any
			for _, client := range clients {
				c := client.(map[string]any)
				if c["enable"] != nil {
					if enable, ok := c["enable"].(bool); ok && !enable {
						continue
					}
				}
				for key := range c {
					if key != "email" && key != "id" && key != "password" && key != "flow" && key != "method" {
						delete(c, key)
					}
					if c["flow"] == "xtls-rprx-vision-udp443" {
						c["flow"] = "xtls-rprx-vision"
					}
				}
				final_clients = append(final_clients, any(c))
			}

			settings["clients"] = final_clients
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}

			inbound.Settings = string(modifiedSettings)
		}

		if len(inbound.StreamSettings) > 0 {
			// Unmarshal stream JSON
			var stream map[string]any
			json.Unmarshal([]byte(inbound.StreamSettings), &stream)

			// Remove the "settings" field under "tlsSettings" and "realitySettings"
			tlsSettings, ok1 := stream["tlsSettings"].(map[string]any)
			realitySettings, ok2 := stream["realitySettings"].(map[string]any)
			if ok1 || ok2 {
				if ok1 {
					delete(tlsSettings, "settings")
				} else if ok2 {
					delete(realitySettings, "settings")
				}
			}

			delete(stream, "externalProxy")

			newStream, err := json.MarshalIndent(stream, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.StreamSettings = string(newStream)
		}

		inboundConfig := inbound.GenXrayInboundConfig()
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
	}

	// sx-ui: Override outbounds from DB if any exist, otherwise keep template outbounds.
	dbOutbounds, _ := s.outboundCrudService.GetEnabled()
	if len(dbOutbounds) > 0 {
		// Merge: keep template outbounds that are not overridden by DB outbounds.
		// Parse existing template outbounds to check for duplicates.
		var templateOuts []map[string]any
		if len(xrayConfig.OutboundConfigs) > 0 {
			_ = json.Unmarshal(xrayConfig.OutboundConfigs, &templateOuts)
		}

		// Build map of DB outbound tags
		dbTagSet := make(map[string]bool)
		var allOuts []map[string]any
		for _, dbo := range dbOutbounds {
			dbTagSet[dbo.Tag] = true
			outObj := map[string]any{
				"tag":      dbo.Tag,
				"protocol": dbo.Protocol,
			}
			if dbo.Settings != "" {
				var settings any
				if err := json.Unmarshal([]byte(dbo.Settings), &settings); err == nil {
					outObj["settings"] = settings
				}
			}
			if dbo.SendThrough != "" {
				outObj["sendThrough"] = dbo.SendThrough
			}
			allOuts = append(allOuts, outObj)
		}

		// Add template outbounds not overridden by DB (e.g., "api", "direct", "blocked")
		for _, tplOut := range templateOuts {
			tag, _ := tplOut["tag"].(string)
			if tag != "" && !dbTagSet[tag] {
				allOuts = append(allOuts, tplOut)
			}
		}

		outsJson, _ := json.Marshal(allOuts)
		xrayConfig.OutboundConfigs = outsJson
	}

	// sx-ui: Override routing rules from DB if any exist.
	dbRules, _ := s.routingCrudService.GetEnabled()
	if len(dbRules) > 0 {
		// Parse existing routing config to preserve domainStrategy, balancers, etc.
		var routingObj map[string]any
		if len(xrayConfig.RouterConfig) > 0 {
			_ = json.Unmarshal(xrayConfig.RouterConfig, &routingObj)
		}
		if routingObj == nil {
			routingObj = map[string]any{"domainStrategy": "AsIs"}
		}

		// Always keep the API inbound route rule
		var allRules []any
		if existingRules, ok := routingObj["rules"].([]any); ok {
			for _, r := range existingRules {
				rMap, ok := r.(map[string]any)
				if ok {
					// Keep the API route rule from the template
					if inboundTag, ok := rMap["inboundTag"].([]any); ok {
						for _, t := range inboundTag {
							if t == "api" {
								allRules = append(allRules, r)
								break
							}
						}
					}
				}
			}
		}

		// Add DB rules in priority order
		for _, rule := range dbRules {
			var ruleObj any
			if err := json.Unmarshal([]byte(rule.RuleJson), &ruleObj); err == nil {
				allRules = append(allRules, ruleObj)
			}
		}

		// Check for geoip:cn block (auto-injected rule for residential nodes)
		geoipBlock, _ := s.nodeMetaService.Get("geoip_block_cn")
		if geoipBlock == "true" {
			allRules = append(allRules, map[string]any{
				"type":        "field",
				"source":      []string{"geoip:cn"},
				"outboundTag": "blocked",
			})
		}

		routingObj["rules"] = allRules
		routingJson, _ := json.Marshal(routingObj)
		xrayConfig.RouterConfig = routingJson
	}

	// sx-core: inject rate limits from DB into config JSON.
	// The Xray subprocess reads these on startup to initialize ratelimit.Manager.
	var rateLimitService RateLimitService
	allLimits, _ := rateLimitService.GetAll()
	if len(allLimits) > 0 {
		for _, rl := range allLimits {
			xrayConfig.RateLimits = append(xrayConfig.RateLimits, xray.RateLimitEntry{
				Email:                rl.Email,
				EgressBps:            rl.EgressBps,
				IngressBps:           rl.IngressBps,
				BurstEgressBps:       rl.BurstEgressBps,
				BurstIngressBps:      rl.BurstIngressBps,
				BurstDurationSeconds: rl.BurstDurationSeconds,
				BurstCooldownSeconds: rl.BurstCooldownSeconds,
			})
		}
	}

	return xrayConfig, nil
}

// GetXrayTraffic fetches the current traffic statistics from the running Xray process.
func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, []*xray.ClientTraffic, error) {
	if !s.IsXrayRunning() {
		err := errors.New("xray is not running")
		logger.Debug("Attempted to fetch Xray traffic, but Xray is not running:", err)
		return nil, nil, err
	}
	apiPort := p.GetAPIPort()
	s.xrayAPI.Init(apiPort)
	defer s.xrayAPI.Close()

	traffic, clientTraffic, err := s.xrayAPI.GetTraffic(true)
	if err != nil {
		logger.Debug("Failed to fetch Xray traffic:", err)
		return nil, nil, err
	}
	return traffic, clientTraffic, nil
}

// RestartXray restarts the Xray process, optionally forcing a restart even if config unchanged.
func (s *XrayService) RestartXray(isForce bool) error {
	lock.Lock()
	defer lock.Unlock()
	logger.Debug("restart Xray, force:", isForce)
	isManuallyStopped.Store(false)

	xrayConfig, err := s.GetXrayConfig()
	if err != nil {
		return err
	}

	if s.IsXrayRunning() {
		if !isForce && p.GetConfig().Equals(xrayConfig) && !isNeedXrayRestart.Load() {
			logger.Debug("It does not need to restart Xray")
			return nil
		}
		p.Stop()
	}

	p = xray.NewProcess(xrayConfig)
	result = ""
	err = p.Start()
	if err != nil {
		return err
	}

	return nil
}

// StopXray stops the running Xray process.
func (s *XrayService) StopXray() error {
	lock.Lock()
	defer lock.Unlock()
	isManuallyStopped.Store(true)
	logger.Debug("Attempting to stop Xray...")
	if s.IsXrayRunning() {
		return p.Stop()
	}
	return errors.New("xray is not running")
}

// SetToNeedRestart marks that Xray needs to be restarted.
func (s *XrayService) SetToNeedRestart() {
	isNeedXrayRestart.Store(true)
}

// IsNeedRestartAndSetFalse checks if restart is needed and resets the flag to false.
func (s *XrayService) IsNeedRestartAndSetFalse() bool {
	return isNeedXrayRestart.CompareAndSwap(true, false)
}

// DidXrayCrash checks if Xray crashed by verifying it's not running and wasn't manually stopped.
func (s *XrayService) DidXrayCrash() bool {
	return !s.IsXrayRunning() && !isManuallyStopped.Load()
}
