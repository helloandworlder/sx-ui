package controller

import (
	"bytes"
	"encoding/json"
	"io"
	_ "net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/helloandworlder/sx-ui/v2/database"
	"github.com/helloandworlder/sx-ui/v2/database/model"
	sxlogger "github.com/helloandworlder/sx-ui/v2/logger"
	"github.com/helloandworlder/sx-ui/v2/web/service"
	logging "github.com/op/go-logging"
)

func setupTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	sxlogger.InitLogger(logging.DEBUG)

	// Create temp SQLite DB
	tmpFile, err := os.CreateTemp("", "sx-ui-api-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()

	if err := database.InitDB(dbPath); err != nil {
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Set API key in NodeMeta
	metaSvc := service.NodeMetaService{}
	metaSvc.Set("api_key", "test-api-key")
	metaSvc.Set("node_type", "dedicated")

	r := gin.New()
	NewRestAPIController(r.Group(""))
	return r, dbPath
}

func teardownRouter(dbPath string) {
	database.CloseDB()
	os.Remove(dbPath)
}

func doRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

type apiResp struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func parseResp(t *testing.T, w *httptest.ResponseRecorder) apiResp {
	t.Helper()
	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

// ── Tests ──────────────────────────────────────────────────────────────

func TestAPI_ConfigSeq(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	w := doRequest(router, "GET", "/api/v1/config/seq", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	if !resp.Success {
		t.Fatal("expected success")
	}

	var seqInfo struct {
		Seq  int64  `json:"seq"`
		Hash string `json:"hash"`
	}
	json.Unmarshal(resp.Obj, &seqInfo)
	if seqInfo.Seq != 0 {
		t.Errorf("expected initial seq 0, got %d", seqInfo.Seq)
	}
}

func TestAPI_AuthRequired(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Request with wrong API key should get 401
	// (Note: requests with no API key and no session will also get 401,
	//  but testing that requires Gin session middleware setup which is
	//  complex in test mode. We test the API key path only.)
	req := httptest.NewRequest("GET", "/api/v1/config/seq", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401 with wrong key, got %d", w.Code)
	}

	// Correct key should work
	w = doRequest(router, "GET", "/api/v1/config/seq", nil)
	if w.Code != 200 {
		t.Errorf("expected 200 with correct key, got %d", w.Code)
	}
}

func TestAPI_Outbound_CRUD(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Create
	w := doRequest(router, "POST", "/api/v1/outbounds", map[string]any{
		"tag": "direct", "protocol": "freedom", "settings": "{}", "enabled": true,
	})
	if w.Code != 201 {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	var created model.Outbound
	json.Unmarshal(resp.Obj, &created)
	if created.Tag != "direct" {
		t.Errorf("expected tag 'direct', got %q", created.Tag)
	}

	// List
	w = doRequest(router, "GET", "/api/v1/outbounds", nil)
	resp = parseResp(t, w)
	var outbounds []model.Outbound
	json.Unmarshal(resp.Obj, &outbounds)
	if len(outbounds) != 1 {
		t.Errorf("expected 1 outbound, got %d", len(outbounds))
	}

	// Update
	w = doRequest(router, "PUT", "/api/v1/outbounds/"+itoa(created.Id), map[string]any{
		"tag": "direct", "protocol": "freedom", "settings": `{"domainStrategy":"UseIP"}`, "enabled": true,
	})
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	// Delete
	w = doRequest(router, "DELETE", "/api/v1/outbounds/"+itoa(created.Id), nil)
	if w.Code != 200 {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// Verify empty
	w = doRequest(router, "GET", "/api/v1/outbounds", nil)
	resp = parseResp(t, w)
	json.Unmarshal(resp.Obj, &outbounds)
	if len(outbounds) != 0 {
		t.Errorf("expected 0 outbounds after delete, got %d", len(outbounds))
	}
}

func TestAPI_Routes_CRUD(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Create
	w := doRequest(router, "POST", "/api/v1/routes", map[string]any{
		"priority": 10,
		"ruleJson": `{"type":"field","outboundTag":"blocked","ip":["geoip:private"]}`,
		"enabled":  true,
	})
	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List
	w = doRequest(router, "GET", "/api/v1/routes", nil)
	resp := parseResp(t, w)
	var routes []model.RoutingRule
	json.Unmarshal(resp.Obj, &routes)
	if len(routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes))
	}
}

func TestAPI_RateLimit_SetGetRemove(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	email := "line-test-uuid"

	// Set
	w := doRequest(router, "PUT", "/api/v1/rate-limits/"+email, map[string]any{
		"egressBps": 12_500_000, "ingressBps": 6_250_000,
	})
	if w.Code != 200 {
		t.Fatalf("set: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get
	w = doRequest(router, "GET", "/api/v1/rate-limits/"+email, nil)
	if w.Code != 200 {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}
	resp := parseResp(t, w)
	var rl model.ClientRateLimit
	json.Unmarshal(resp.Obj, &rl)
	if rl.EgressBps != 12_500_000 {
		t.Errorf("expected egress 12.5M, got %d", rl.EgressBps)
	}

	// Remove
	w = doRequest(router, "DELETE", "/api/v1/rate-limits/"+email, nil)
	if w.Code != 200 {
		t.Fatalf("remove: expected 200, got %d", w.Code)
	}

	// Verify gone
	w = doRequest(router, "GET", "/api/v1/rate-limits/"+email, nil)
	if w.Code != 404 {
		t.Errorf("expected 404 after remove, got %d", w.Code)
	}
}

func TestAPI_NodeMeta(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Get initial meta (should have api_key and node_type from setup)
	w := doRequest(router, "GET", "/api/v1/node/meta", nil)
	resp := parseResp(t, w)
	var meta map[string]string
	json.Unmarshal(resp.Obj, &meta)
	if meta["node_type"] != "dedicated" {
		t.Errorf("expected node_type=dedicated, got %q", meta["node_type"])
	}

	// Set additional meta
	w = doRequest(router, "PUT", "/api/v1/node/meta", map[string]string{
		"geoip_block_cn": "true",
	})
	if w.Code != 200 {
		t.Fatalf("set meta: expected 200, got %d", w.Code)
	}

	// Verify
	w = doRequest(router, "GET", "/api/v1/node/meta", nil)
	resp = parseResp(t, w)
	json.Unmarshal(resp.Obj, &meta)
	if meta["geoip_block_cn"] != "true" {
		t.Error("geoip_block_cn not set")
	}
}

func TestAPI_SyncState(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Create some data first
	doRequest(router, "POST", "/api/v1/outbounds", map[string]any{
		"tag": "direct", "protocol": "freedom", "settings": "{}", "enabled": true,
	})
	doRequest(router, "POST", "/api/v1/routes", map[string]any{
		"priority": 10, "ruleJson": `{"type":"field","outboundTag":"blocked"}`, "enabled": true,
	})
	doRequest(router, "PUT", "/api/v1/rate-limits/user@test", map[string]any{
		"egressBps": 1000, "ingressBps": 2000,
	})

	// Get sync state
	w := doRequest(router, "GET", "/api/v1/sync/state", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := parseResp(t, w)
	var state struct {
		ConfigSeq  int64                  `json:"configSeq"`
		Outbounds  []model.Outbound       `json:"outbounds"`
		Routes     []model.RoutingRule     `json:"routes"`
		RateLimits []model.ClientRateLimit `json:"rateLimits"`
		NodeMeta   map[string]string       `json:"nodeMeta"`
	}
	json.Unmarshal(resp.Obj, &state)

	if state.ConfigSeq < 2 { // outbound create + route create = at least 2 bumps
		t.Errorf("expected configSeq >= 2, got %d", state.ConfigSeq)
	}
	if len(state.Outbounds) != 1 {
		t.Errorf("expected 1 outbound in state, got %d", len(state.Outbounds))
	}
	if len(state.Routes) != 1 {
		t.Errorf("expected 1 route in state, got %d", len(state.Routes))
	}
	if len(state.RateLimits) != 1 {
		t.Errorf("expected 1 rate limit in state, got %d", len(state.RateLimits))
	}
}

func TestAPI_ConfigSeq_IncrementsOnMutation(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	// Get initial seq
	w := doRequest(router, "GET", "/api/v1/config/seq", nil)
	resp := parseResp(t, w)
	var info1 struct {
		Seq  int64  `json:"seq"`
		Hash string `json:"hash"`
	}
	json.Unmarshal(resp.Obj, &info1)
	initialSeq := info1.Seq

	// Create an outbound (should bump seq)
	doRequest(router, "POST", "/api/v1/outbounds", map[string]any{
		"tag": "test", "protocol": "blackhole", "settings": "{}", "enabled": true,
	})

	// Check seq increased
	w = doRequest(router, "GET", "/api/v1/config/seq", nil)
	resp = parseResp(t, w)
	var info2 struct {
		Seq  int64  `json:"seq"`
		Hash string `json:"hash"`
	}
	json.Unmarshal(resp.Obj, &info2)
	if info2.Seq <= initialSeq {
		t.Errorf("seq should have increased after mutation: %d → %d", initialSeq, info2.Seq)
	}
	if info2.Hash == "" {
		t.Error("hash should be non-empty after mutation")
	}
	if info2.Hash == info1.Hash {
		t.Error("hash should have changed after mutation")
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
