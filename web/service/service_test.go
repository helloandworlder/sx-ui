package service

import (
	"os"
	"testing"

	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
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
