package controller

import (
	"bytes"
	"encoding/json"
	"io"
	_ "net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
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

func doFormRequest(router *gin.Engine, method, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

func TestAPI_RateLimit_SetGetBurstWindow(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	email := "D20668679"

	w := doRequest(router, "PUT", "/api/v1/rate-limits/"+email, map[string]any{
		"egressBps":            1_000_000,
		"ingressBps":           1_000_000,
		"burstEgressBps":       2_000_000,
		"burstIngressBps":      2_000_000,
		"burstDurationSeconds": 30,
		"burstCooldownSeconds": 300,
	})
	if w.Code != 200 {
		t.Fatalf("set: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = doRequest(router, "GET", "/api/v1/rate-limits/"+email, nil)
	if w.Code != 200 {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}
	resp := parseResp(t, w)
	var rl model.ClientRateLimit
	json.Unmarshal(resp.Obj, &rl)

	if rl.Email != email {
		t.Fatalf("expected email %s, got %s", email, rl.Email)
	}
	if rl.BurstEgressBps != 2_000_000 || rl.BurstIngressBps != 2_000_000 {
		t.Fatalf("expected burst bps 2000000/2000000, got %d/%d", rl.BurstEgressBps, rl.BurstIngressBps)
	}
	if rl.BurstDurationSeconds != 30 || rl.BurstCooldownSeconds != 300 {
		t.Fatalf("expected burst window 30/300, got %d/%d", rl.BurstDurationSeconds, rl.BurstCooldownSeconds)
	}
}

func TestAPI_MixedAccountUpdatePreservesAndClearsEntitlements(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20084,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","accounts":[{"user":"u","pass":"p","email":"line@example.com","enable":true,"comment":"line","limitIp":3,"totalGB":107374182400,"expiryTime":1893456000000,"reset":30,"egressBps":125000,"ingressBps":125000}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	w := doRequest(router, "PUT", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/line@example.com", map[string]any{
		"email":      "line@example.com",
		"totalGB":    0,
		"expiryTime": 0,
		"reset":      0,
		"limitIp":    0,
	})
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Accounts) != 1 {
		t.Fatalf("expected one account, got %d", len(settings.Accounts))
	}
	account := settings.Accounts[0]
	if account["user"] != "u" || account["pass"] != "p" {
		t.Fatalf("expected partial update to preserve credentials, got %#v", account)
	}
	if account["totalGB"].(float64) != 0 || account["expiryTime"].(float64) != 0 ||
		account["reset"].(float64) != 0 || account["limitIp"].(float64) != 0 {
		t.Fatalf("expected explicit zero entitlement fields, got %#v", account)
	}
}

func TestAPI_MixedAccountUpdateAcceptsFormPayload(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20086,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","accounts":[{"user":"old-user","pass":"old-pass","email":"vo4qcnxo","enable":true,"comment":"","limitIp":0,"totalGB":0,"expiryTime":0,"reset":0,"egressBps":0,"ingressBps":0,"subId":"old-sub"}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed-form",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("user", "sLeHVAG47c")
	form.Set("pass", "XxUgDcEwVg")
	form.Set("email", "vo4qcnxo")
	form.Set("enable", "true")
	form.Set("comment", "")
	form.Set("egressBps", "0")
	form.Set("ingressBps", "0")
	form.Set("subId", "j312rxe3bpjkz02k")

	w := doFormRequest(router, "PUT", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/vo4qcnxo", form)
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Accounts) != 1 {
		t.Fatalf("expected one account, got %d", len(settings.Accounts))
	}
	account := settings.Accounts[0]
	if account["user"] != "sLeHVAG47c" || account["pass"] != "XxUgDcEwVg" {
		t.Fatalf("expected form credentials to update, got %#v", account)
	}
	if account["subId"] != "j312rxe3bpjkz02k" {
		t.Fatalf("expected form subId to update, got %#v", account)
	}
}

func TestAPI_MixedAccountUpdateMatchesOriginalIdentifierWhenEmailChanges(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20087,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","accounts":[{"user":"old-user","pass":"old-pass","email":"vmvnleo0111","enable":true,"comment":"","limitIp":0,"totalGB":0,"expiryTime":0,"reset":0,"egressBps":0,"ingressBps":0,"subId":"old-sub"}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed-email-change",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("user", "TQdfuMaWPS111")
	form.Set("pass", "enD1RABi4c")
	form.Set("email", "npo5a")
	form.Set("enable", "true")
	form.Set("comment", "")
	form.Set("egressBps", "0")
	form.Set("ingressBps", "0")
	form.Set("subId", "fccap8wq7g701zj0")
	form.Set("created_at", "1777590634275")
	form.Set("updated_at", "1777590640388")

	w := doFormRequest(router, "PUT", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/vmvnleo0111", form)
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Accounts) != 1 {
		t.Fatalf("expected one account, got %d", len(settings.Accounts))
	}
	account := settings.Accounts[0]
	if account["email"] != "npo5a" || account["user"] != "TQdfuMaWPS111" || account["pass"] != "enD1RABi4c" {
		t.Fatalf("expected account to update by original identifier, got %#v", account)
	}
}

func TestAPI_MixedAccountUpdateAcceptsLegacyClientsSettings(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed legacy clients",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20088,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","clients":[{"user":"legacy-user","pass":"legacy-pass","email":"legacy-email","enable":true,"comment":"","egressBps":0,"ingressBps":0,"subId":"old-sub"}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed-legacy-clients",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("user", "new-user")
	form.Set("pass", "new-pass")
	form.Set("email", "legacy-email")
	form.Set("enable", "true")
	form.Set("comment", "")
	form.Set("egressBps", "0")
	form.Set("ingressBps", "0")
	form.Set("subId", "new-sub")

	w := doFormRequest(router, "PUT", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/legacy-email", form)
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 1 {
		t.Fatalf("expected one legacy client, got %d", len(settings.Clients))
	}
	client := settings.Clients[0]
	if client["user"] != "new-user" || client["pass"] != "new-pass" || client["subId"] != "new-sub" {
		t.Fatalf("expected legacy clients settings to update in place, got %#v", client)
	}
}

func TestAPI_MixedAccountListAndDeleteAcceptLegacyClientsSettings(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Mixed legacy clients",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20089,
		Protocol:       model.Mixed,
		Settings:       `{"auth":"password","clients":[{"id":"account-uuid","user":"legacy-user","pass":"legacy-pass","email":"legacy-email","enable":true,"comment":"","egressBps":0,"ingressBps":0,"subId":"old-sub"}]}`,
		StreamSettings: `{}`,
		Tag:            "in-mixed-legacy-list-delete",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	w := doRequest(router, "GET", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients", nil)
	if w.Code != 200 {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := parseResp(t, w)
	var listed struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal(resp.Obj, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Clients) != 1 || listed.Clients[0]["email"] != "legacy-email" {
		t.Fatalf("expected legacy clients to list as clients, got %#v", listed.Clients)
	}

	w = doRequest(router, "DELETE", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/account-uuid", nil)
	if w.Code != 200 {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 0 {
		t.Fatalf("expected legacy clients settings to delete in place, got %#v", settings.Clients)
	}
}

func TestAPI_UpdateClientAcceptsStringTelegramID(t *testing.T) {
	router, dbPath := setupTestRouter(t)
	defer teardownRouter(dbPath)

	db := database.GetDB()
	inbound := &model.Inbound{
		Remark:         "Shadowsocks",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           20085,
		Protocol:       model.Shadowsocks,
		Settings:       `{"method":"aes-256-gcm","clients":[{"method":"aes-256-gcm","password":"old-pass","email":"dl-a81aa960-ce3f-4e01-8452-33aa2ba7ad17","enable":true,"tgId":0,"subId":"old","limitIp":0,"totalGB":0,"expiryTime":0,"reset":0}]}`,
		StreamSettings: `{}`,
		Tag:            "in-shadowsocks",
		Sniffing:       `{"enabled":false}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	w := doRequest(router, "PUT", "/api/v1/inbounds/"+itoa(inbound.Id)+"/clients/dl-a81aa960-ce3f-4e01-8452-33aa2ba7ad17", map[string]any{
		"method":     "aes-256-gcm",
		"password":   "new-pass",
		"email":      "dl-a81aa960-ce3f-4e01-8452-33aa2ba7ad17",
		"enable":     true,
		"tgId":       "0",
		"subId":      "392aada560b141fe",
		"limitIp":    0,
		"totalGB":    107374182400,
		"expiryTime": 1778885848570,
		"reset":      0,
	})
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.Inbound
	if err := db.First(&updated, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(updated.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 1 {
		t.Fatalf("expected one client, got %d", len(settings.Clients))
	}
	if settings.Clients[0].TgID != 0 {
		t.Fatalf("expected tgId 0, got %d", settings.Clients[0].TgID)
	}
	if settings.Clients[0].Password != "new-pass" {
		t.Fatalf("expected updated password, got %q", settings.Clients[0].Password)
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
		ConfigSeq  int64                   `json:"configSeq"`
		Outbounds  []model.Outbound        `json:"outbounds"`
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
