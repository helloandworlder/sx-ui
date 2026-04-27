package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
	"github.com/helloandworlder/sx-ui/v2/xray"
)

type outboundModel = model.Outbound
type routingRuleModel = model.RoutingRule

// testDBPath creates a temporary SQLite database for testing.
// Caller must defer os.Remove(path) after use.
func setupTestDB(t *testing.T) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "sx-ui-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	path := tmpFile.Name()
	tmpFile.Close()

	if err := database.InitDB(path); err != nil {
		os.Remove(path)
		t.Fatalf("failed to init db: %v", err)
	}
	return path
}

func teardownTestDB(path string) {
	database.CloseDB()
	os.Remove(path)
}

// ── ConfigSeqService Tests ─────────────────────────────────────────────

func TestConfigSeq_GetAndBump(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := ConfigSeqService{}

	// Initial seq should be 0
	seq, err := svc.GetSeq()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 0 {
		t.Errorf("expected initial seq 0, got %d", seq)
	}

	// Bump should increment
	newSeq, err := svc.BumpSeq()
	if err != nil {
		t.Fatal(err)
	}
	if newSeq != 1 {
		t.Errorf("expected seq 1 after bump, got %d", newSeq)
	}

	// Bump again
	newSeq, err = svc.BumpSeq()
	if err != nil {
		t.Fatal(err)
	}
	if newSeq != 2 {
		t.Errorf("expected seq 2, got %d", newSeq)
	}
}

func TestConfigSeq_BumpSeqAndHash(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := ConfigSeqService{}

	seq, err := svc.BumpSeqAndHash()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Errorf("expected seq 1, got %d", seq)
	}

	info, err := svc.GetSeqInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.Seq != 1 {
		t.Errorf("expected info.Seq 1, got %d", info.Seq)
	}
	if info.Hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestConfigSeq_HashIgnoresDynamicTrafficCounters(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	db := database.GetDB()
	if err := db.Create(&model.Inbound{
		Remark:         "Socks",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20084,
		Protocol:       model.Socks,
		Settings:       `{"accounts":[{"user":"u","pass":"p","email":"e@example.com"}]}`,
		StreamSettings: `{}`,
		Tag:            "in-socks",
		Sniffing:       `{"enabled":false}`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := ConfigSeqService{}
	if _, err := svc.BumpSeqAndHash(); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.GetSeqInfo()
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&model.Inbound{}).
		Where("tag = ?", "in-socks").
		Updates(map[string]any{"up": int64(1024), "down": int64(2048), "all_time": int64(3072)}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateHash(); err != nil {
		t.Fatal(err)
	}
	afterTraffic, err := svc.GetSeqInfo()
	if err != nil {
		t.Fatal(err)
	}

	if initial.Hash != afterTraffic.Hash {
		t.Fatalf("expected config hash to ignore traffic counters, got %s -> %s", initial.Hash, afterTraffic.Hash)
	}
}

func TestConfigSeq_HashIgnoresRateLimitUpdatedAt(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	db := database.GetDB()
	if err := db.Create(&model.ClientRateLimit{
		Email:      "line@example.com",
		EgressBps:  125000,
		IngressBps: 125000,
		UpdatedAt:  1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := ConfigSeqService{}
	if _, err := svc.BumpSeqAndHash(); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.GetSeqInfo()
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&model.ClientRateLimit{}).
		Where("email = ?", "line@example.com").
		Update("updated_at", int64(2)).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateHash(); err != nil {
		t.Fatal(err)
	}
	afterUpdate, err := svc.GetSeqInfo()
	if err != nil {
		t.Fatal(err)
	}

	if initial.Hash != afterUpdate.Hash {
		t.Fatalf("expected config hash to ignore rate limit timestamps, got %s -> %s", initial.Hash, afterUpdate.Hash)
	}
}

func TestInboundService_GetClientByEmailSupportsAccountProtocols(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Socks",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20084,
		Protocol:       model.Socks,
		Settings:       `{"accounts":[{"user":"u","pass":"p","email":"line@example.com","enable":true,"comment":"test","totalGB":107374182400,"expiryTime":1893456000000,"reset":30,"egressBps":125000,"ingressBps":125000}]}`,
		StreamSettings: `{}`,
		Tag:            "in-socks",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{
		InboundId: inbound.Id,
		Email:     "line@example.com",
		Enable:    true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := InboundService{}
	traffic, client, err := svc.GetClientByEmail("line@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if traffic == nil || client == nil {
		t.Fatal("expected traffic and client to be resolved")
	}
	if client.Email != "line@example.com" {
		t.Fatalf("unexpected client email: %s", client.Email)
	}
	if client.Password != "p" {
		t.Fatalf("unexpected client password: %s", client.Password)
	}
	if client.EgressBps != 125000 || client.IngressBps != 125000 {
		t.Fatalf("unexpected account rate limits: %d/%d", client.EgressBps, client.IngressBps)
	}
	if client.TotalGB != 107374182400 || client.ExpiryTime != 1893456000000 || client.Reset != 30 {
		t.Fatalf("unexpected account entitlement: total=%d expiry=%d reset=%d", client.TotalGB, client.ExpiryTime, client.Reset)
	}
}

func TestXrayService_GetXrayConfigFiltersDisabledAccountProtocols(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20084,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","accounts":[{"user":"active","pass":"p1","email":"active@example.com","enable":true,"totalGB":107374182400,"expiryTime":1893456000000},{"user":"expired","pass":"p2","email":"expired@example.com","enable":true,"totalGB":107374182400,"expiryTime":1}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{
		InboundId: inbound.Id,
		Email:     "active@example.com",
		Enable:    true,
		Total:     107374182400,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{
		InboundId:  inbound.Id,
		Email:      "expired@example.com",
		Enable:     false,
		Total:      107374182400,
		ExpiryTime: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	xraySvc := XrayService{}
	cfg, err := xraySvc.GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.InboundConfigs) == 0 {
		t.Fatal("expected generated inbounds")
	}
	var settings struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(cfg.InboundConfigs[len(cfg.InboundConfigs)-1].Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Accounts) != 1 {
		t.Fatalf("expected only one active account, got %d: %#v", len(settings.Accounts), settings.Accounts)
	}
	if settings.Accounts[0]["email"] != "active@example.com" {
		t.Fatalf("unexpected account left in config: %#v", settings.Accounts[0])
	}
	if _, ok := settings.Accounts[0]["totalGB"]; ok {
		t.Fatalf("runtime account should not include panel-only entitlement fields: %#v", settings.Accounts[0])
	}
}

// ── NodeMetaService Tests ──────────────────────────────────────────────

func TestNodeMeta_CRUD(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := NodeMetaService{}

	// Get non-existent
	val, err := svc.Get("api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty for non-existent key, got %q", val)
	}

	// Set
	if err := svc.Set("api_key", "test-key-123"); err != nil {
		t.Fatal(err)
	}

	// Get
	val, err = svc.Get("api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "test-key-123" {
		t.Errorf("expected 'test-key-123', got %q", val)
	}

	// Update (upsert)
	if err := svc.Set("api_key", "updated-key"); err != nil {
		t.Fatal(err)
	}
	val, _ = svc.Get("api_key")
	if val != "updated-key" {
		t.Errorf("expected 'updated-key', got %q", val)
	}

	// GetAll
	svc.Set("node_type", "residential")
	all, err := svc.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}
	if all["api_key"] != "updated-key" || all["node_type"] != "residential" {
		t.Error("GetAll values mismatch")
	}

	// Delete
	if err := svc.Delete("api_key"); err != nil {
		t.Fatal(err)
	}
	val, _ = svc.Get("api_key")
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

// ── RateLimitService Tests ─────────────────────────────────────────────

func TestRateLimit_CRUD(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := RateLimitService{}

	// Get non-existent
	rl, err := svc.Get("user@test")
	if err != nil {
		t.Fatal(err)
	}
	if rl != nil {
		t.Error("expected nil for non-existent")
	}

	// Set
	rl, err = svc.Set("user@test", 12_500_000, 6_250_000)
	if err != nil {
		t.Fatal(err)
	}
	if rl.Email != "user@test" {
		t.Errorf("expected email user@test, got %s", rl.Email)
	}
	if rl.EgressBps != 12_500_000 {
		t.Errorf("expected egress 12500000, got %d", rl.EgressBps)
	}

	// Get
	rl, _ = svc.Get("user@test")
	if rl == nil || rl.IngressBps != 6_250_000 {
		t.Error("Get after Set mismatch")
	}

	// Update
	rl, _ = svc.Set("user@test", 25_000_000, 25_000_000)
	if rl.EgressBps != 25_000_000 {
		t.Errorf("expected updated egress 25M, got %d", rl.EgressBps)
	}

	// GetAll
	svc.Set("user2@test", 1000, 2000)
	all, err := svc.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 rate limits, got %d", len(all))
	}

	// Remove
	if err := svc.Remove("user@test"); err != nil {
		t.Fatal(err)
	}
	rl, _ = svc.Get("user@test")
	if rl != nil {
		t.Error("expected nil after remove")
	}
}

// ── OutboundCrudService Tests ──────────────────────────────────────────

func TestOutboundCrud_CRUD(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := OutboundCrudService{}

	// Create
	out := &outboundModel{
		Tag:      "direct",
		Protocol: "freedom",
		Settings: "{}",
		Enabled:  true,
	}
	if err := svc.Create(out); err != nil {
		t.Fatal(err)
	}
	if out.Id == 0 {
		t.Error("expected auto-generated ID")
	}
	if out.Seq == 0 {
		t.Error("expected non-zero seq after create")
	}

	// GetAll
	all, err := svc.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(all))
	}

	// GetByTag
	got, err := svc.GetByTag("direct")
	if err != nil {
		t.Fatal(err)
	}
	if got.Protocol != "freedom" {
		t.Errorf("expected protocol freedom, got %s", got.Protocol)
	}

	// Update
	got.Settings = `{"domainStrategy": "UseIP"}`
	if err := svc.Update(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := svc.GetById(got.Id)
	if got2.Settings != `{"domainStrategy": "UseIP"}` {
		t.Error("update didn't persist")
	}

	// Delete
	if err := svc.Delete(got.Id); err != nil {
		t.Fatal(err)
	}
	all, _ = svc.GetAll()
	if len(all) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(all))
	}
}

// ── RoutingCrudService Tests ───────────────────────────────────────────

func TestRoutingCrud_CRUDAndReorder(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(dbPath)

	svc := RoutingCrudService{}

	// Create two rules
	r1 := &routingRuleModel{Priority: 10, RuleJson: `{"type":"field","outboundTag":"direct"}`, Enabled: true}
	r2 := &routingRuleModel{Priority: 20, RuleJson: `{"type":"field","outboundTag":"blocked"}`, Enabled: true}
	svc.Create(r1)
	svc.Create(r2)

	// GetAll should return sorted by priority
	all, _ := svc.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(all))
	}
	if all[0].Priority != 10 || all[1].Priority != 20 {
		t.Error("rules not sorted by priority")
	}

	// Reorder: swap priorities
	err := svc.Reorder([]struct {
		Id       int `json:"id"`
		Priority int `json:"priority"`
	}{
		{r1.Id, 20},
		{r2.Id, 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	all, _ = svc.GetAll()
	if all[0].Id != r2.Id {
		t.Error("reorder didn't work — r2 should be first now")
	}

	// Delete
	svc.Delete(r1.Id)
	svc.Delete(r2.Id)
	all, _ = svc.GetAll()
	if len(all) != 0 {
		t.Error("expected 0 after delete all")
	}
}
