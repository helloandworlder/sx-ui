package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/helloandworlder/sx-ui/v2/database/model"
	"github.com/helloandworlder/sx-ui/v2/logger"
	"github.com/helloandworlder/sx-ui/v2/web/service"
	"github.com/helloandworlder/sx-ui/v2/web/session"

	"github.com/gin-gonic/gin"
)

// RestAPIController exposes a RESTful /api/v1 surface for GoSea management.
type RestAPIController struct {
	inboundService   service.InboundService
	outboundService  service.OutboundCrudService
	routingService   service.RoutingCrudService
	rateLimitService service.RateLimitService
	configSeqService service.ConfigSeqService
	nodeMetaService  service.NodeMetaService
	xrayService      service.XrayService
	xrayDynamic      service.XrayDynamicService
	ipScannerService service.IpScannerService
}

func NewRestAPIController(g *gin.RouterGroup) *RestAPIController {
	a := &RestAPIController{}
	a.initRouter(g)
	return a
}

// apiKeyOrSession authenticates via X-API-Key header first, then falls back
// to session cookie auth. Returns 401 on failure.
func (a *RestAPIController) apiKeyOrSession(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" {
		stored, err := a.nodeMetaService.Get("api_key")
		if err == nil && stored != "" && apiKey == stored {
			c.Next()
			return
		}
		// API key was provided but invalid — reject immediately
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "invalid api key"})
		return
	}
	// No API key header: fallback to session (only when session middleware is available)
	if session.IsLogin(c) {
		c.Next()
		return
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
}

func (a *RestAPIController) initRouter(g *gin.RouterGroup) {
	v1 := g.Group("/api/v1")
	v1.Use(a.apiKeyOrSession)

	// Config sequence
	v1.GET("/config/seq", a.getConfigSeq)

	// Inbounds
	v1.GET("/inbounds", a.listInbounds)
	v1.POST("/inbounds", a.createInbound)
	v1.GET("/inbounds/:id", a.getInbound)
	v1.PUT("/inbounds/:id", a.updateInbound)
	v1.DELETE("/inbounds/:id", a.deleteInbound)

	// Clients (nested under inbound)
	v1.GET("/inbounds/:id/clients", a.listClients)
	v1.POST("/inbounds/:id/clients", a.addClient)
	v1.PUT("/inbounds/:id/clients/:email", a.updateClient)
	v1.DELETE("/inbounds/:id/clients/:email", a.deleteClient)

	// Outbounds
	v1.GET("/outbounds", a.listOutbounds)
	v1.POST("/outbounds", a.createOutbound)
	v1.GET("/outbounds/:id", a.getOutbound)
	v1.PUT("/outbounds/:id", a.updateOutbound)
	v1.DELETE("/outbounds/:id", a.deleteOutbound)

	// Routes
	v1.GET("/routes", a.listRoutes)
	v1.POST("/routes", a.createRoute)
	v1.PUT("/routes/:id", a.updateRoute)
	v1.DELETE("/routes/:id", a.deleteRoute)
	v1.POST("/routes/reorder", a.reorderRoutes)

	// Rate limits
	v1.GET("/rate-limits/:email", a.getRateLimit)
	v1.PUT("/rate-limits/:email", a.setRateLimit)
	v1.DELETE("/rate-limits/:email", a.removeRateLimit)

	// Xray control
	v1.POST("/xray/restart", a.restartXray)

	// Client traffic & speed
	v1.GET("/clients/:email/traffic", a.getClientTraffic)
	v1.GET("/clients/:email/speed", a.getClientSpeed)
	v1.GET("/clients/:email/ips", a.getClientIps)

	// Node
	v1.GET("/node/meta", a.getNodeMeta)
	v1.PUT("/node/meta", a.setNodeMeta)
	v1.GET("/node/status", a.getNodeStatus)
	v1.GET("/node/public-ips", a.getPublicIps)
	v1.POST("/node/scan-ips", a.scanIps)

	// Sync (bulk)
	v1.GET("/sync/state", a.getSyncState)
	v1.POST("/sync/full", a.fullSync)
}

// --- helpers ---

func (a *RestAPIController) ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": data})
}

func (a *RestAPIController) created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "obj": data})
}

func (a *RestAPIController) fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "msg": msg})
}

func (a *RestAPIController) idParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		a.fail(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// --- Config Seq ---

func (a *RestAPIController) getConfigSeq(c *gin.Context) {
	info, err := a.configSeqService.GetSeqInfo()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, info)
}

// --- Inbounds ---

func (a *RestAPIController) listInbounds(c *gin.Context) {
	inbounds, err := a.inboundService.GetAllInbounds()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, inbounds)
}

func (a *RestAPIController) createInbound(c *gin.Context) {
	var inbound model.Inbound
	if err := c.ShouldBindJSON(&inbound); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// bump config seq
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Allow creating inbound without clients (empty settings)
	if inbound.Settings == "" {
		if isAccountInboundProtocol(inbound.Protocol) {
			inbound.Settings = `{"accounts":[]}`
		} else {
			inbound.Settings = `{"clients":[]}`
		}
	}
	result, needRestart, err := a.inboundService.AddInbound(&inbound)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.created(c, result)
}

func (a *RestAPIController) getInbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "inbound not found")
		return
	}
	a.ok(c, inbound)
}

func (a *RestAPIController) updateInbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	var inbound model.Inbound
	if err := c.ShouldBindJSON(&inbound); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	inbound.Id = id
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	result, needRestart, err := a.inboundService.UpdateInbound(&inbound)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.ok(c, result)
}

func (a *RestAPIController) deleteInbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	needRestart, err := a.inboundService.DelInbound(id)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.ok(c, nil)
}

// --- Clients ---

func (a *RestAPIController) listClients(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "inbound not found")
		return
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if isAccountInboundProtocol(inbound.Protocol) {
		a.ok(c, gin.H{"clients": settings["accounts"]})
		return
	}
	a.ok(c, gin.H{"clients": settings["clients"]})
}

// AddClientRequest supports both VMess/VLESS/Trojan style clients and HTTP/Socks5 accounts.
// For HTTP/Socks/Mixed: GoSea sends {user, pass, email} — the email is the UUIDv7 internal key,
// user/pass are the short random credentials the end-user sees.
type AddClientRequest struct {
	// VMess/VLESS/Trojan fields
	Clients []model.Client `json:"clients"`
	// HTTP/Socks5/Mixed fields
	Accounts []struct {
		User       string `json:"user"`
		Pass       string `json:"pass"`
		Email      string `json:"email"` // UUIDv7 internal identifier
		Enable     *bool  `json:"enable"`
		Comment    string `json:"comment"`
		EgressBps  int64  `json:"egressBps"`
		IngressBps int64  `json:"ingressBps"`
	} `json:"accounts"`
}

type inboundAccountPayload struct {
	User       string `json:"user"`
	Pass       string `json:"pass"`
	Email      string `json:"email"`
	Enable     *bool  `json:"enable"`
	Comment    string `json:"comment"`
	EgressBps  int64  `json:"egressBps"`
	IngressBps int64  `json:"ingressBps"`
	CreatedAt  int64  `json:"created_at,omitempty"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

func (a *RestAPIController) syncAccountRateLimit(email string, egressBps, ingressBps int64) error {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	if egressBps <= 0 && ingressBps <= 0 {
		return a.rateLimitService.Remove(email)
	}
	_, err := a.rateLimitService.Set(email, egressBps, ingressBps)
	return err
}

func boolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func isAccountInboundProtocol(protocol model.Protocol) bool {
	switch protocol {
	case model.HTTP, model.Socks, model.Mixed:
		return true
	default:
		return false
	}
}

func mergeShadowsocksClients(
	inbound *model.Inbound,
	clients []model.Client,
) error {
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return err
	}

	method, _ := settings["method"].(string)
	existing, _ := settings["clients"].([]any)
	if len(existing) == 0 {
		email, _ := settings["email"].(string)
		password, _ := settings["password"].(string)
		if strings.TrimSpace(email) != "" && strings.TrimSpace(password) != "" {
			nowTs := time.Now().UnixMilli()
			existing = append(existing, map[string]any{
				"email":      email,
				"password":   password,
				"method":     method,
				"enable":     true,
				"created_at": nowTs,
				"updated_at": nowTs,
			})
		}
	}

	nowTs := time.Now().UnixMilli()
	for _, client := range clients {
		existing = append(existing, map[string]any{
			"email":      client.Email,
			"password":   client.Password,
			"method":     method,
			"enable":     client.Enable,
			"comment":    client.Comment,
			"limitIp":    client.LimitIP,
			"totalGB":    client.TotalGB,
			"expiryTime": client.ExpiryTime,
			"subId":      client.SubID,
			"tgId":       client.TgID,
			"reset":      client.Reset,
			"created_at": nowTs,
			"updated_at": nowTs,
		})
	}

	settings["clients"] = existing
	newSettings, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	inbound.Settings = string(newSettings)
	return nil
}

func (a *RestAPIController) addClient(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}

	var req AddClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Determine protocol to decide settings format
	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "inbound not found")
		return
	}

	switch inbound.Protocol {
	case model.HTTP, model.Socks, model.Mixed:
		// HTTP/Socks/Mixed: accounts format [{"user":"x","pass":"y","email":"line-uuid"}]
		if len(req.Accounts) == 0 && len(req.Clients) > 0 {
			// Convert from clients format: use email as both user and pass
			for _, cl := range req.Clients {
				req.Accounts = append(req.Accounts, struct {
					User       string `json:"user"`
					Pass       string `json:"pass"`
					Email      string `json:"email"`
					Enable     *bool  `json:"enable"`
					Comment    string `json:"comment"`
					EgressBps  int64  `json:"egressBps"`
					IngressBps int64  `json:"ingressBps"`
				}{User: cl.Email, Pass: cl.Email, Email: cl.Email})
			}
		}
		// Merge new accounts into existing inbound settings
		var existSettings map[string]any
		json.Unmarshal([]byte(inbound.Settings), &existSettings)
		existAccounts, _ := existSettings["accounts"].([]any)
		for _, acc := range req.Accounts {
			nowTs := time.Now().UnixMilli()
			enable := boolOrDefault(acc.Enable, true)
			existAccounts = append(existAccounts, map[string]any{
				"user": acc.User, "pass": acc.Pass, "email": acc.Email,
				"enable": enable, "comment": acc.Comment,
				"egressBps": acc.EgressBps, "ingressBps": acc.IngressBps,
				"created_at": nowTs, "updated_at": nowTs,
			})
			if err := a.syncAccountRateLimit(acc.Email, acc.EgressBps, acc.IngressBps); err != nil {
				a.fail(c, http.StatusInternalServerError, err.Error())
				return
			}
		}
		existSettings["accounts"] = existAccounts
		newSettings, _ := json.Marshal(existSettings)
		inbound.Settings = string(newSettings)
		_, _, err := a.inboundService.UpdateInbound(inbound)
		if err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		a.xrayService.SetToNeedRestart()
		a.created(c, req.Accounts)

	case model.Shadowsocks:
		if len(req.Clients) == 0 {
			a.fail(c, http.StatusBadRequest, "no clients provided")
			return
		}
		if err := mergeShadowsocksClients(inbound, req.Clients); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		_, needRestart, err := a.inboundService.UpdateInbound(inbound)
		if err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		a.created(c, req.Clients)

	default:
		// VMess/VLESS/Trojan/Shadowsocks: clients format
		if len(req.Clients) == 0 {
			a.fail(c, http.StatusBadRequest, "no clients provided")
			return
		}
		clientsJson, _ := json.Marshal(req.Clients)
		settings := `{"clients":` + string(clientsJson) + `}`
		needRestart, err := a.inboundService.AddInboundClient(&model.Inbound{
			Id:       id,
			Settings: settings,
		})
		if err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		a.created(c, req.Clients)
	}
}

func (a *RestAPIController) updateClient(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	email := c.Param("email")

	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "inbound not found")
		return
	}

	if isAccountInboundProtocol(inbound.Protocol) {
		var account inboundAccountPayload
		if err := c.ShouldBindJSON(&account); err != nil {
			a.fail(c, http.StatusBadRequest, err.Error())
			return
		}
		account.Email = email
		if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		accounts, _ := settings["accounts"].([]any)
		found := false
		for idx, raw := range accounts {
			item, _ := raw.(map[string]any)
			accEmail, _ := item["email"].(string)
			accUser, _ := item["user"].(string)
			if accEmail != email && accUser != email {
				continue
			}
			if v, ok := item["created_at"].(float64); ok {
				account.CreatedAt = int64(v)
			}
			if account.CreatedAt == 0 {
				account.CreatedAt = time.Now().UnixMilli()
			}
			account.UpdatedAt = time.Now().UnixMilli()
			enable := true
			if rawEnable, ok := item["enable"].(bool); ok {
				enable = rawEnable
			}
			enable = boolOrDefault(account.Enable, enable)
			accounts[idx] = map[string]any{
				"user": account.User, "pass": account.Pass, "email": account.Email,
				"enable": enable, "comment": account.Comment,
				"egressBps": account.EgressBps, "ingressBps": account.IngressBps,
				"created_at": account.CreatedAt, "updated_at": account.UpdatedAt,
			}
			found = true
			break
		}
		if !found {
			a.fail(c, http.StatusNotFound, "client not found")
			return
		}
		settings["accounts"] = accounts
		newSettings, _ := json.Marshal(settings)
		inbound.Settings = string(newSettings)
		if _, _, err := a.inboundService.UpdateInbound(inbound); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := a.syncAccountRateLimit(account.Email, account.EgressBps, account.IngressBps); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		a.xrayService.SetToNeedRestart()
		a.ok(c, account)
		return
	}

	var client model.Client
	if err := c.ShouldBindJSON(&client); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	client.Email = email

	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	clientJson, _ := json.Marshal(client)
	settings := `{"clients":[` + string(clientJson) + `]}`

	// UpdateInboundClient expects clientId (the UUID), but in our REST API
	// we use email as the identifier. Pass email as clientId — the service
	// will look up the client by scanning the settings JSON.
	needRestart, err := a.inboundService.UpdateInboundClient(&model.Inbound{
		Id:       id,
		Settings: settings,
	}, client.ID)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.ok(c, client)
}

func (a *RestAPIController) deleteClient(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	email := c.Param("email")

	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "inbound not found")
		return
	}

	switch inbound.Protocol {
	case model.HTTP, model.Socks, model.Mixed:
		// Remove account by email (or username) from accounts array
		var settings map[string]any
		json.Unmarshal([]byte(inbound.Settings), &settings)
		accounts, _ := settings["accounts"].([]any)
		var filtered []any
		for _, acc := range accounts {
			m, _ := acc.(map[string]any)
			accEmail, _ := m["email"].(string)
			accUser, _ := m["user"].(string)
			if accEmail != email && accUser != email {
				filtered = append(filtered, acc)
			}
		}
		settings["accounts"] = filtered
		newSettings, _ := json.Marshal(settings)
		inbound.Settings = string(newSettings)
		_, _, err := a.inboundService.UpdateInbound(inbound)
		if err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		_ = a.rateLimitService.Remove(email)
		a.xrayService.SetToNeedRestart()

	default:
		needRestart, err := a.inboundService.DelInboundClientByEmail(id, email)
		if err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
	}
	a.ok(c, nil)
}

// --- Outbounds ---

func (a *RestAPIController) listOutbounds(c *gin.Context) {
	outs, err := a.outboundService.GetAll()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, outs)
}

func (a *RestAPIController) createOutbound(c *gin.Context) {
	var out model.Outbound
	if err := c.ShouldBindJSON(&out); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.outboundService.Create(&out); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// gRPC dynamic add (falls back to restart on failure)
	a.xrayDynamic.DynamicAddOutbound(&out)
	a.created(c, out)
}

func (a *RestAPIController) getOutbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	out, err := a.outboundService.GetById(id)
	if err != nil {
		a.fail(c, http.StatusNotFound, "outbound not found")
		return
	}
	a.ok(c, out)
}

func (a *RestAPIController) updateOutbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	var out model.Outbound
	if err := c.ShouldBindJSON(&out); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	out.Id = id
	if err := a.outboundService.Update(&out); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayService.SetToNeedRestart()
	a.ok(c, out)
}

func (a *RestAPIController) deleteOutbound(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	// Get tag before delete for gRPC removal
	existing, _ := a.outboundService.GetById(id)
	if err := a.outboundService.Delete(id); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		a.xrayDynamic.DynamicDelOutbound(existing.Tag)
	}
	a.ok(c, nil)
}

// --- Routes ---

func (a *RestAPIController) listRoutes(c *gin.Context) {
	rules, err := a.routingService.GetAll()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, rules)
}

func (a *RestAPIController) createRoute(c *gin.Context) {
	var rule model.RoutingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.routingService.Create(&rule); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayDynamic.DynamicAddRoute(rule.RuleJson)
	a.created(c, rule)
}

func (a *RestAPIController) updateRoute(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	var rule model.RoutingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	rule.Id = id
	if err := a.routingService.Update(&rule); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayService.SetToNeedRestart()
	a.ok(c, rule)
}

func (a *RestAPIController) deleteRoute(c *gin.Context) {
	id, ok := a.idParam(c)
	if !ok {
		return
	}
	if err := a.routingService.Delete(id); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayService.SetToNeedRestart()
	a.ok(c, nil)
}

func (a *RestAPIController) reorderRoutes(c *gin.Context) {
	var items []struct {
		Id       int `json:"id"`
		Priority int `json:"priority"`
	}
	if err := c.ShouldBindJSON(&items); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.routingService.Reorder(items); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayService.SetToNeedRestart()
	a.ok(c, nil)
}

// --- Rate Limits ---

func (a *RestAPIController) getRateLimit(c *gin.Context) {
	email := c.Param("email")
	rl, err := a.rateLimitService.Get(email)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rl == nil {
		a.fail(c, http.StatusNotFound, "no rate limit set")
		return
	}
	a.ok(c, rl)
}

func (a *RestAPIController) setRateLimit(c *gin.Context) {
	email := c.Param("email")
	var body struct {
		EgressBps  int64 `json:"egressBps"`
		IngressBps int64 `json:"ingressBps"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	rl, err := a.rateLimitService.Set(email, body.EgressBps, body.IngressBps)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Push to the running Xray subprocess over gRPC immediately.
	a.xrayDynamic.DynamicSetRateLimit(email, body.EgressBps, body.IngressBps)
	a.ok(c, rl)
}

func (a *RestAPIController) removeRateLimit(c *gin.Context) {
	email := c.Param("email")
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.rateLimitService.Remove(email); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.xrayDynamic.DynamicRemoveRateLimit(email)
	a.ok(c, nil)
}

// --- Client Traffic & Speed ---

func (a *RestAPIController) getClientTraffic(c *gin.Context) {
	email := c.Param("email")
	traffics, err := a.inboundService.GetClientTrafficByEmail(email)
	if err != nil {
		a.fail(c, http.StatusNotFound, "client not found")
		return
	}
	a.ok(c, traffics)
}

func (a *RestAPIController) getClientSpeed(c *gin.Context) {
	email := c.Param("email")
	eBps, iBps := a.xrayDynamic.DynamicGetUserSpeed(email)
	a.ok(c, gin.H{
		"email":      email,
		"egressBps":  eBps,
		"ingressBps": iBps,
	})
}

func (a *RestAPIController) getClientIps(c *gin.Context) {
	email := c.Param("email")
	ips, err := a.inboundService.GetInboundClientIps(email)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, ips)
}

// --- Node Meta ---

func (a *RestAPIController) getNodeMeta(c *gin.Context) {
	meta, err := a.nodeMetaService.GetAll()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, meta)
}

func (a *RestAPIController) setNodeMeta(c *gin.Context) {
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := a.configSeqService.BumpSeqAndHash(); err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	for k, v := range body {
		if err := a.nodeMetaService.Set(k, v); err != nil {
			a.fail(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	a.ok(c, body)
}

func (a *RestAPIController) restartXray(c *gin.Context) {
	err := a.xrayService.RestartXray(true)
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, gin.H{"restarted": true})
}

func (a *RestAPIController) getNodeStatus(c *gin.Context) {
	seq, _ := a.configSeqService.GetSeq()
	a.ok(c, gin.H{
		"xrayRunning": a.xrayService.IsXrayRunning(),
		"xrayVersion": a.xrayService.GetXrayVersion(),
		"configSeq":   seq,
	})
}

func (a *RestAPIController) getPublicIps(c *gin.Context) {
	ips, err := a.ipScannerService.GetPublicIps()
	if err != nil {
		a.fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.ok(c, ips)
}

func (a *RestAPIController) scanIps(c *gin.Context) {
	go func() {
		if err := a.ipScannerService.ScanPublicIps(); err != nil {
			logger.Warning("IP scan failed:", err)
		}
	}()
	a.ok(c, gin.H{"msg": "scan started"})
}

// --- Sync ---

// SyncState represents the full current state of a node for GoSea comparison.
type SyncState struct {
	ConfigSeq  int64                   `json:"configSeq"`
	Inbounds   []*model.Inbound        `json:"inbounds"`
	Outbounds  []model.Outbound        `json:"outbounds"`
	Routes     []model.RoutingRule     `json:"routes"`
	RateLimits []model.ClientRateLimit `json:"rateLimits"`
	NodeMeta   map[string]string       `json:"nodeMeta"`
}

func (a *RestAPIController) getSyncState(c *gin.Context) {
	seq, _ := a.configSeqService.GetSeq()
	inbounds, _ := a.inboundService.GetAllInbounds()
	outbounds, _ := a.outboundService.GetAll()
	routes, _ := a.routingService.GetAll()
	rateLimits, _ := a.rateLimitService.GetAll()
	meta, _ := a.nodeMetaService.GetAll()

	state := SyncState{
		ConfigSeq:  seq,
		Inbounds:   inbounds,
		Outbounds:  outbounds,
		Routes:     routes,
		RateLimits: rateLimits,
		NodeMeta:   meta,
	}
	a.ok(c, state)
}

// FullSyncRequest is the desired state that GoSea pushes to this node.
type FullSyncRequest struct {
	Inbounds   []model.Inbound         `json:"inbounds"`
	Outbounds  []model.Outbound        `json:"outbounds"`
	Routes     []model.RoutingRule     `json:"routes"`
	RateLimits []model.ClientRateLimit `json:"rateLimits"`
	NodeMeta   map[string]string       `json:"nodeMeta"`
}

func (a *RestAPIController) fullSync(c *gin.Context) {
	var req FullSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var errs []string

	// Sync outbounds: delete all, recreate from desired state
	if req.Outbounds != nil {
		existingOuts, _ := a.outboundService.GetAll()
		existingMap := make(map[string]bool)
		for _, o := range existingOuts {
			existingMap[o.Tag] = true
		}
		desiredMap := make(map[string]bool)
		for i := range req.Outbounds {
			desiredMap[req.Outbounds[i].Tag] = true
			existing, _ := a.outboundService.GetByTag(req.Outbounds[i].Tag)
			if existing != nil {
				req.Outbounds[i].Id = existing.Id
				if err := a.outboundService.Update(&req.Outbounds[i]); err != nil {
					errs = append(errs, "outbound update "+req.Outbounds[i].Tag+": "+err.Error())
				}
			} else {
				if err := a.outboundService.Create(&req.Outbounds[i]); err != nil {
					errs = append(errs, "outbound create "+req.Outbounds[i].Tag+": "+err.Error())
				}
			}
		}
		// delete outbounds not in desired state
		for _, o := range existingOuts {
			if !desiredMap[o.Tag] {
				if err := a.outboundService.Delete(o.Id); err != nil {
					errs = append(errs, "outbound delete "+o.Tag+": "+err.Error())
				}
			}
		}
	}

	// Sync routing rules: replace all
	if req.Routes != nil {
		existingRules, _ := a.routingService.GetAll()
		for _, r := range existingRules {
			_ = a.routingService.Delete(r.Id)
		}
		for i := range req.Routes {
			req.Routes[i].Id = 0 // reset ID for creation
			if err := a.routingService.Create(&req.Routes[i]); err != nil {
				errs = append(errs, "route create: "+err.Error())
			}
		}
	}

	// Sync rate limits
	if req.RateLimits != nil {
		existingRLs, _ := a.rateLimitService.GetAll()
		existingRLMap := make(map[string]bool)
		for _, rl := range existingRLs {
			existingRLMap[rl.Email] = true
		}
		desiredRLMap := make(map[string]bool)
		for _, rl := range req.RateLimits {
			desiredRLMap[rl.Email] = true
			if _, err := a.rateLimitService.Set(rl.Email, rl.EgressBps, rl.IngressBps); err != nil {
				errs = append(errs, "rate-limit set "+rl.Email+": "+err.Error())
			}
		}
		for _, rl := range existingRLs {
			if !desiredRLMap[rl.Email] {
				_ = a.rateLimitService.Remove(rl.Email)
			}
		}
	}

	// Sync node meta
	if req.NodeMeta != nil {
		for k, v := range req.NodeMeta {
			if k == "api_key" {
				continue // never overwrite api_key from remote
			}
			_ = a.nodeMetaService.Set(k, v)
		}
	}

	a.xrayService.SetToNeedRestart()

	if len(errs) > 0 {
		a.ok(c, gin.H{"synced": true, "errors": errs})
	} else {
		a.ok(c, gin.H{"synced": true})
	}
}
