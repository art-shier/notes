package api

import (
	"crypto/rand"
	"encoding/base32"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"regexp"
	"shiji/internal/security"
	"shiji/internal/store"
	"strings"
	"time"
	"unicode/utf8"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var prefixPattern = regexp.MustCompile(`^sj_[A-Za-z0-9_-]{7}$`)
var codePattern = regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}$`)

type agentRate struct {
	until time.Time
	count int
}

// Trust socket addresses, not user-supplied forwarding headers. Bound retained entries.
func (a *App) agentLimit(c *gin.Context, kind string, limit int) error {
	a.agentRateMu.Lock()
	defer a.agentRateMu.Unlock()
	now := time.Now()
	if a.agentRates == nil {
		a.agentRates = map[string]agentRate{}
	}
	for k, v := range a.agentRates {
		if !now.Before(v.until) {
			delete(a.agentRates, k)
		}
	}
	key := kind + ":" + c.RemoteIP()
	v, ok := a.agentRates[key]
	if !ok {
		if len(a.agentRates) >= 4096 {
			return Fail(429, "rate_limited", "连接请求过多，请稍后再试。")
		}
		v.until = now.Add(time.Minute)
	}
	v.count++
	a.agentRates[key] = v
	if v.count > limit {
		c.Header("Retry-After", "60")
		return Fail(429, "rate_limited", "连接请求过多，请稍后再试。")
	}
	return nil
}
func (a *App) agentPublic(fn func(*gin.Context) error, kind string, limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		e := a.agentLimit(c, kind, limit)
		if e == nil && c.GetHeader("Origin") != "" && c.GetHeader("Origin") != a.Settings.Origin {
			e = Fail(403, "origin_denied", "请求来源不被允许。")
		}
		if e == nil {
			e = fn(c)
		}
		if e != nil {
			RespondError(c, e)
		}
	}
}
func (a *App) RegisterAgentAuth(r *gin.Engine) {
	r.POST("/api/v1/auth/agent/request", a.agentPublic(a.agentRequest, "request", 5))
	r.POST("/api/v1/auth/agent/poll", a.agentPublic(a.agentPoll, "poll", 90))
	r.POST("/api/v1/auth/agent/cancel", a.agentPublic(a.agentCancel, "cancel", 20))
	r.GET("/api/v1/auth/agent/requests/:code", a.Handle("", true, func(c *gin.Context, p Principal) error {
		if e := a.agentLimit(c, "details", 30); e != nil {
			return e
		}
		code := strings.ToUpper(c.Param("code"))
		if !codePattern.MatchString(code) {
			return Fail(404, "not_found", "授权请求不存在。")
		}
		var g store.AgentGrant
		if e := a.DB.First(&g, "user_code = ?", code).Error; e != nil {
			return Fail(404, "not_found", "授权请求不存在。")
		}
		c.JSON(200, gin.H{"user_code": g.UserCode, "name": g.Name, "status": grantStatus(g), "expires_at": g.ExpiresAt})
		return nil
	}))
	r.POST("/api/v1/auth/agent/requests/:code", a.Handle("", true, a.agentDecision))
	r.GET("/api/v1/auth/agent/me", a.Handle("", false, func(c *gin.Context, p Principal) error {
		if p.Token == nil {
			return Fail(403, "bearer_required", "请使用 Agent 凭据。")
		}
		c.JSON(200, gin.H{"account": gin.H{"id": p.User.ID, "email": p.User.Email, "display_name": p.User.DisplayName}, "token": tokenView(*p.Token)})
		return nil
	}))
	r.POST("/api/v1/auth/agent/logout", a.Handle("", false, func(c *gin.Context, p Principal) error {
		if p.Token == nil {
			return Fail(403, "bearer_required", "请使用 Agent 凭据。")
		}
		if e := a.DB.Model(p.Token).Update("revoked_at", store.Now()).Error; e != nil {
			return e
		}
		c.Status(204)
		return nil
	}))
}
func grantStatus(g store.AgentGrant) string {
	if !g.ExpiresAt.After(time.Now()) {
		return "expired"
	}
	return g.Status
}
func (a *App) agentRequest(c *gin.Context) error {
	var b struct {
		Name   string `json:"name"`
		Hash   string `json:"token_hash"`
		Prefix string `json:"token_prefix"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || utf8.RuneCountInString(b.Name) > 60 || !hashPattern.MatchString(b.Hash) || !prefixPattern.MatchString(b.Prefix) {
		return Fail(422, "validation_error", "请求参数不正确。")
	}
	var random [5]byte
	if _, e := rand.Read(random[:]); e != nil {
		return e
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random[:])
	code = code[:4] + "-" + code[4:]
	device := security.Secret()
	g := store.AgentGrant{ID: store.ID(), DeviceHash: security.Digest(device), UserCode: code, Name: b.Name, TokenHash: b.Hash, TokenPrefix: b.Prefix, Status: "pending", CreatedAt: store.Now(), ExpiresAt: store.Time{Time: time.Now().UTC().Add(10 * time.Minute)}}
	if e := a.Write("", func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if e := tx.Exec("SELECT pg_advisory_xact_lock(84721095)").Error; e != nil {
				return e
			}
		}
		if e := tx.Where("expires_at < ?", store.Time{Time: time.Now().Add(-time.Hour)}).Delete(&store.AgentGrant{}).Error; e != nil {
			return e
		}
		var n int64
		if e := tx.Model(&store.AgentGrant{}).Where("expires_at > ?", store.Now()).Count(&n).Error; e != nil {
			return e
		}
		if n >= 512 {
			return Fail(429, "rate_limited", "连接请求过多，请稍后再试。")
		}
		return tx.Create(&g).Error
	}); e != nil {
		return e
	}
	c.JSON(201, gin.H{"device_code": device, "user_code": code, "verification_uri": strings.TrimRight(a.Settings.Origin, "/") + "/#agent-authorize?code=" + code, "expires_in": 600, "interval": 2})
	return nil
}
func deviceGrant(c *gin.Context, tx *gorm.DB, lock bool) (store.AgentGrant, error) {
	var g store.AgentGrant
	var b struct {
		Device string `json:"device_code"`
	}
	if e := Decode(c, &b); e != nil {
		return g, e
	}
	if len(b.Device) != 43 {
		return g, Fail(422, "validation_error", "请求参数不正确。")
	}
	if lock && tx.Dialector.Name() == "postgres" {
		tx = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if tx.First(&g, "device_hash = ?", security.Digest(b.Device)).Error != nil {
		return g, Fail(404, "not_found", "授权请求不存在。")
	}
	return g, nil
}
func (a *App) agentPoll(c *gin.Context) error {
	g, e := deviceGrant(c, a.DB, false)
	if e != nil {
		return e
	}
	status := grantStatus(g)
	v := gin.H{"status": status}
	if status == "approved" && g.TokenID != nil {
		var t store.Token
		if a.DB.First(&t, "id = ? AND revoked_at IS NULL AND expires_at > ?", *g.TokenID, store.Now()).Error != nil {
			v["status"] = "canceled"
		} else {
			v["token"] = tokenView(t)
		}
	}
	c.JSON(200, v)
	return nil
}
func (a *App) agentDecision(c *gin.Context, p Principal) error {
	if e := a.agentLimit(c, "decision", 20); e != nil {
		return e
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var b struct {
		Approve bool   `json:"approve"`
		Access  string `json:"access"`
		Trash   bool   `json:"allow_trash"`
		Days    int    `json:"expires_days"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	if b.Approve && (b.Access != "read" && b.Access != "write" || b.Days < 1 || b.Days > 365 || b.Trash && b.Access != "write") {
		return Fail(422, "validation_error", "请选择有效的权限和有效期。")
	}
	code := strings.ToUpper(c.Param("code"))
	if !codePattern.MatchString(code) {
		return Fail(404, "not_found", "授权请求不存在。")
	}
	e := a.Write(p.User.ID, func(tx *gorm.DB) error {
		var g store.AgentGrant
		query := tx
		if tx.Dialector.Name() == "postgres" {
			query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if query.First(&g, "user_code = ?", code).Error != nil {
			return Fail(404, "not_found", "授权请求不存在。")
		}
		if grantStatus(g) != "pending" {
			return Fail(409, "grant_closed", "授权请求已处理或已过期，请重新连接。")
		}
		status := "denied"
		if b.Approve {
			status = "approved"
		}
		res := tx.Model(&store.AgentGrant{}).Where("id = ? AND status = ? AND expires_at > ?", g.ID, "pending", store.Now()).Update("status", status)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return Fail(409, "grant_closed", "授权请求已处理或已过期。")
		}
		if !b.Approve {
			return nil
		}
		scopes := store.List{"notes:read", "folders:read", "attachments:read", "tags:read"}
		if b.Access == "write" {
			scopes = append(scopes, "notes:create", "notes:update", "folders:write", "attachments:write", "tags:write")
		}
		if b.Trash {
			scopes = append(scopes, "notes:trash")
		}
		t := store.Token{ID: store.ID(), UserID: p.User.ID, Name: g.Name, Prefix: g.TokenPrefix, SecretHash: g.TokenHash, Scopes: scopes, CreatedAt: store.Now(), ExpiresAt: store.Time{Time: time.Now().UTC().Add(time.Duration(b.Days) * 24 * time.Hour)}}
		if e := tx.Create(&t).Error; e != nil {
			return e
		}
		return tx.Model(&g).Update("token_id", t.ID).Error
	})
	if e != nil {
		return e
	}
	c.JSON(200, gin.H{"status": map[bool]string{true: "approved", false: "denied"}[b.Approve]})
	return nil
}
func (a *App) agentCancel(c *gin.Context) error {
	e := a.Write("", func(tx *gorm.DB) error {
		g, e := deviceGrant(c, tx, true)
		if e != nil {
			return e
		}
		if grantStatus(g) == "expired" {
			return Fail(409, "grant_closed", "授权已过期，请通过凭据管理撤销。")
		}
		if g.Status == "denied" || g.Status == "canceled" {
			return nil
		}
		if g.TokenID != nil {
			if e := tx.Model(&store.Token{}).Where("id = ?", *g.TokenID).Update("revoked_at", store.Now()).Error; e != nil {
				return e
			}
		}
		return tx.Model(&g).Update("status", "canceled").Error
	})
	if e != nil {
		return e
	}
	c.JSON(200, gin.H{"status": "canceled"})
	return nil
}
