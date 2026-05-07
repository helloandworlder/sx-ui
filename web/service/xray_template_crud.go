package service

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/helloandworlder/sx-ui/v2/database/model"
)

// XrayTemplateConfigService treats settings.xrayTemplateConfig as the
// authoritative source for legacy 3x-ui outbounds and routing rules.
type XrayTemplateConfigService struct {
	SettingService   SettingService
	ConfigSeqService ConfigSeqService
}

func (s *XrayTemplateConfigService) loadTemplate() (map[string]any, error) {
	raw, err := s.SettingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(UnwrapXrayTemplateConfig(raw)), &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

func (s *XrayTemplateConfigService) saveTemplate(cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := s.SettingService.saveSetting("xrayTemplateConfig", string(data)); err != nil {
		return err
	}
	_, err = s.ConfigSeqService.BumpSeqAndHash()
	return err
}

func compactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func templateJSONSignature(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return string(raw)
	}
	delete(payload, "ruleTag")
	delete(payload, "rule_tag")
	return compactJSON(payload)
}

func templateOutboundFromJSON(index int, raw any) model.Outbound {
	item, _ := raw.(map[string]any)
	out := model.Outbound{Id: index + 1, Enabled: true}
	if tag, ok := item["tag"].(string); ok {
		out.Tag = tag
	}
	if protocol, ok := item["protocol"].(string); ok {
		out.Protocol = protocol
	}
	if settings, ok := item["settings"]; ok {
		out.Settings = compactJSON(settings)
	}
	if sendThrough, ok := item["sendThrough"].(string); ok {
		out.SendThrough = sendThrough
	}
	return out
}

func templateOutboundToJSON(out model.Outbound) (map[string]any, error) {
	if out.Tag == "" {
		return nil, fmt.Errorf("outbound tag is required")
	}
	if out.Protocol == "" {
		return nil, fmt.Errorf("outbound protocol is required")
	}
	item := map[string]any{
		"tag":      out.Tag,
		"protocol": out.Protocol,
	}
	if out.Settings != "" {
		var settings any
		if err := json.Unmarshal([]byte(out.Settings), &settings); err != nil {
			return nil, fmt.Errorf("invalid outbound settings: %w", err)
		}
		item["settings"] = settings
	}
	if out.SendThrough != "" {
		item["sendThrough"] = out.SendThrough
	}
	return item, nil
}

func templateOutbounds(cfg map[string]any) []any {
	outbounds, _ := cfg["outbounds"].([]any)
	if outbounds == nil {
		outbounds = []any{}
	}
	return outbounds
}

func (s *XrayTemplateConfigService) GetOutbounds() ([]model.Outbound, error) {
	cfg, err := s.loadTemplate()
	if err != nil {
		return nil, err
	}
	raw := templateOutbounds(cfg)
	outbounds := make([]model.Outbound, 0, len(raw))
	for i, item := range raw {
		outbounds = append(outbounds, templateOutboundFromJSON(i, item))
	}
	return outbounds, nil
}

func (s *XrayTemplateConfigService) GetOutboundByID(id int) (*model.Outbound, error) {
	outbounds, err := s.GetOutbounds()
	if err != nil {
		return nil, err
	}
	if id <= 0 || id > len(outbounds) {
		return nil, fmt.Errorf("outbound not found")
	}
	return &outbounds[id-1], nil
}

func (s *XrayTemplateConfigService) CreateOutbound(out *model.Outbound) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	outbounds := templateOutbounds(cfg)
	for index, existing := range outbounds {
		if templateOutboundFromJSON(0, existing).Tag == out.Tag {
			item, err := templateOutboundToJSON(*out)
			if err != nil {
				return err
			}
			outbounds[index] = item
			cfg["outbounds"] = outbounds
			out.Id = index + 1
			out.Enabled = true
			return s.saveTemplate(cfg)
		}
	}
	item, err := templateOutboundToJSON(*out)
	if err != nil {
		return err
	}
	outbounds = append(outbounds, item)
	cfg["outbounds"] = outbounds
	out.Id = len(outbounds)
	out.Enabled = true
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) UpdateOutbound(out *model.Outbound) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	outbounds := templateOutbounds(cfg)
	if out.Id <= 0 || out.Id > len(outbounds) {
		return fmt.Errorf("outbound not found")
	}
	item, err := templateOutboundToJSON(*out)
	if err != nil {
		return err
	}
	outbounds[out.Id-1] = item
	cfg["outbounds"] = outbounds
	out.Enabled = true
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) DeleteOutbound(id int) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	outbounds := templateOutbounds(cfg)
	if id <= 0 || id > len(outbounds) {
		return fmt.Errorf("outbound not found")
	}
	outbounds = append(outbounds[:id-1], outbounds[id:]...)
	cfg["outbounds"] = outbounds
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) ReplaceOutbounds(outbounds []model.Outbound) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	raw := make([]any, 0, len(outbounds))
	seen := map[string]bool{}
	for _, out := range outbounds {
		if seen[out.Tag] {
			return fmt.Errorf("duplicate outbound tag: %s", out.Tag)
		}
		seen[out.Tag] = true
		item, err := templateOutboundToJSON(out)
		if err != nil {
			return err
		}
		raw = append(raw, item)
	}
	cfg["outbounds"] = raw
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) GetOutboundsJSON() (string, error) {
	cfg, err := s.loadTemplate()
	if err != nil {
		return "", err
	}
	return compactJSON(templateOutbounds(cfg)), nil
}

func templateRouting(cfg map[string]any) map[string]any {
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{"domainStrategy": "AsIs"}
	}
	return routing
}

func templateRouteFromJSON(index int, raw any) model.RoutingRule {
	return model.RoutingRule{
		Id:       index + 1,
		Priority: index + 1,
		RuleJson: compactJSON(raw),
		Enabled:  true,
	}
}

func templateRouteToJSON(route model.RoutingRule) (any, error) {
	var raw any
	if err := json.Unmarshal([]byte(route.RuleJson), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *XrayTemplateConfigService) GetRoutes() ([]model.RoutingRule, error) {
	cfg, err := s.loadTemplate()
	if err != nil {
		return nil, err
	}
	routing := templateRouting(cfg)
	rawRules, _ := routing["rules"].([]any)
	routes := make([]model.RoutingRule, 0, len(rawRules))
	for i, rule := range rawRules {
		routes = append(routes, templateRouteFromJSON(i, rule))
	}
	return routes, nil
}

func (s *XrayTemplateConfigService) GetRouteByID(id int) (*model.RoutingRule, error) {
	routes, err := s.GetRoutes()
	if err != nil {
		return nil, err
	}
	if id <= 0 || id > len(routes) {
		return nil, fmt.Errorf("route not found")
	}
	return &routes[id-1], nil
}

func (s *XrayTemplateConfigService) CreateRoute(route *model.RoutingRule) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	routing := templateRouting(cfg)
	rawRules, _ := routing["rules"].([]any)
	raw, err := templateRouteToJSON(*route)
	if err != nil {
		return err
	}
	rawRules = append(rawRules, raw)
	routing["rules"] = rawRules
	cfg["routing"] = routing
	route.Id = len(rawRules)
	route.Priority = len(rawRules)
	route.Enabled = true
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) UpdateRoute(route *model.RoutingRule) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	routing := templateRouting(cfg)
	rawRules, _ := routing["rules"].([]any)
	if route.Id <= 0 || route.Id > len(rawRules) {
		return fmt.Errorf("route not found")
	}
	raw, err := templateRouteToJSON(*route)
	if err != nil {
		return err
	}
	rawRules[route.Id-1] = raw
	routing["rules"] = rawRules
	cfg["routing"] = routing
	route.Priority = route.Id
	route.Enabled = true
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) DeleteRoute(id int) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	routing := templateRouting(cfg)
	rawRules, _ := routing["rules"].([]any)
	if id <= 0 || id > len(rawRules) {
		return fmt.Errorf("route not found")
	}
	rawRules = append(rawRules[:id-1], rawRules[id:]...)
	routing["rules"] = rawRules
	cfg["routing"] = routing
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) ReorderRoutes(items []struct {
	ID       int `json:"id"`
	Priority int `json:"priority"`
}) error {
	routes, err := s.GetRoutes()
	if err != nil {
		return err
	}
	priorityByID := make(map[int]int, len(items))
	for _, item := range items {
		priorityByID[item.ID] = item.Priority
	}
	for i := range routes {
		if priority, ok := priorityByID[routes[i].Id]; ok {
			routes[i].Priority = priority
		}
	}
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Priority < routes[j].Priority
	})
	return s.ReplaceRoutes(routes)
}

func (s *XrayTemplateConfigService) ReplaceRoutes(routes []model.RoutingRule) error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}
	rawRules := make([]any, 0, len(routes))
	for _, route := range routes {
		raw, err := templateRouteToJSON(route)
		if err != nil {
			return err
		}
		rawRules = append(rawRules, raw)
	}
	routing := templateRouting(cfg)
	routing["rules"] = rawRules
	cfg["routing"] = routing
	return s.saveTemplate(cfg)
}

func (s *XrayTemplateConfigService) GetRoutingJSON() (string, error) {
	cfg, err := s.loadTemplate()
	if err != nil {
		return "", err
	}
	return compactJSON(templateRouting(cfg)), nil
}

// MigrateCrudRowsToTemplate is a one-way compatibility bridge from the
// short-lived sx-ui CRUD tables back into the 3x-ui-compatible Xray JSON blob.
// After this runs, xrayTemplateConfig remains the only runtime source of truth.
func (s *XrayTemplateConfigService) MigrateCrudRowsToTemplate() error {
	cfg, err := s.loadTemplate()
	if err != nil {
		return err
	}

	changed := false

	outbounds := templateOutbounds(cfg)
	outboundTags := make(map[string]bool, len(outbounds))
	for _, raw := range outbounds {
		outboundTags[templateOutboundFromJSON(0, raw).Tag] = true
	}
	outboundService := OutboundCrudService{}
	dbOutbounds, err := outboundService.GetEnabled()
	if err != nil {
		return err
	}
	for _, out := range dbOutbounds {
		if outboundTags[out.Tag] {
			continue
		}
		raw, err := templateOutboundToJSON(out)
		if err != nil {
			return err
		}
		outbounds = append(outbounds, raw)
		outboundTags[out.Tag] = true
		changed = true
	}
	if changed {
		cfg["outbounds"] = outbounds
	}

	routing := templateRouting(cfg)
	rawRules, _ := routing["rules"].([]any)
	ruleSignatures := make(map[string]bool, len(rawRules))
	for _, raw := range rawRules {
		if sig := templateJSONSignature(raw); sig != "" {
			ruleSignatures[sig] = true
		}
	}
	routingService := RoutingCrudService{}
	dbRoutes, err := routingService.GetEnabled()
	if err != nil {
		return err
	}
	for _, route := range dbRoutes {
		raw, err := templateRouteToJSON(route)
		if err != nil {
			return err
		}
		sig := templateJSONSignature(raw)
		if sig != "" && ruleSignatures[sig] {
			continue
		}
		rawRules = append(rawRules, raw)
		ruleSignatures[sig] = true
		changed = true
	}
	if changed {
		routing["rules"] = rawRules
		cfg["routing"] = routing
		return s.saveTemplate(cfg)
	}
	return nil
}
