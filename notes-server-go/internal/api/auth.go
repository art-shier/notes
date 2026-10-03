package api

import (
	"crypto/subtle"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"regexp"
	"shiji/internal/security"
	"shiji/internal/store"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const Cookie = "shiji_session"

var validEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var Scopes = []string{"notes:read", "notes:create", "notes:update", "notes:trash", "folders:read", "folders:write", "attachments:read", "attachments:write", "tags:read", "tags:write"}
var dummyHash, _ = security.HashPassword("dummy-verification-password")

func NormalizeEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 254 || !validEmail.MatchString(s) {
		return "", Fail(422, "validation_error", "请输入有效邮箱。")
	}
	return s, nil
}
func (a *App) Authenticate(c *gin.Context, scope string, webOnly bool) (Principal, error) {
	var p Principal
	auth := c.GetHeader("Authorization")
	now := store.Now()
	if auth != "" {
		if !strings.HasPrefix(auth, "Bearer ") {
			return p, Fail(401, "unauthorized", "访问凭证无效。")
		}
		var t store.Token
		if e := a.DB.Where("secret_hash = ? AND revoked_at IS NULL AND expires_at > ?", security.Digest(strings.TrimPrefix(auth, "Bearer ")), now).First(&t).Error; e != nil {
			return p, Fail(401, "unauthorized", "请登录，或检查访问凭证。")
		}
		p.Token = &t
		if e := a.DB.First(&p.User, "id = ?", t.UserID).Error; e != nil {
			return p, Fail(401, "unauthorized", "请登录，或检查访问凭证。")
		}
	} else {
		raw, _ := c.Cookie(Cookie)
		var s store.Session
		if raw == "" || a.DB.Where("secret_hash = ? AND expires_at > ?", security.Digest(raw), now).First(&s).Error != nil {
			return p, Fail(401, "unauthorized", "请登录，或检查访问凭证。")
		}
		p.Session = &s
		p.Raw = raw
		if e := a.DB.First(&p.User, "id = ?", s.UserID).Error; e != nil {
			return p, Fail(401, "unauthorized", "请登录，或检查访问凭证。")
		}
	}
	if p.User.Disabled {
		return p, Fail(401, "unauthorized", "请登录，或检查访问凭证。")
	}
	if p.Token != nil {
		allowed := scope == ""
		for _, s := range p.Token.Scopes {
			if s == scope {
				allowed = true
			}
		}
		if webOnly || !allowed {
			return p, Fail(403, "scope_denied", "访问凭证没有这项权限。")
		}
	} else if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
		if c.GetHeader("Origin") != a.Settings.Origin {
			return p, Fail(403, "origin_denied", "请求来源不被允许。")
		}
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(security.CSRF(p.Raw))) != 1 {
			return p, Fail(403, "csrf_denied", "登录状态校验失败，请刷新后重试。")
		}
	}
	return p, nil
}
func UserInfo(u store.User, csrf string) map[string]any {
	return map[string]any{"id": u.ID, "email": u.Email, "display_name": u.DisplayName, "role": u.Role, "quota_bytes": u.QuotaBytes, "used_bytes": u.UsedBytes, "csrf_token": csrf}
}
func newSession(tx *gorm.DB, u store.User) (string, error) {
	raw := security.Secret()
	s := store.Session{ID: store.ID(), UserID: u.ID, SecretHash: security.Digest(raw), ExpiresAt: store.Time{Time: time.Now().UTC().Add(7 * 24 * time.Hour)}}
	return raw, tx.Create(&s).Error
}
func (a *App) setSession(c *gin.Context, u store.User, raw string, status int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(Cookie, raw, 7*86400, "/api", "", a.Settings.CookieSecure, true)
	c.JSON(status, UserInfo(u, security.CSRF(raw)))
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) RegisterAuth(r *gin.Engine) {
	r.POST("/api/v1/auth/register", func(c *gin.Context) {
		if e := a.register(c); e != nil {
			RespondError(c, e)
		}
	})
	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		if e := a.login(c); e != nil {
			RespondError(c, e)
		}
	})
	r.GET("/api/v1/me", a.Handle("", true, func(c *gin.Context, p Principal) error {
		c.JSON(200, UserInfo(p.User, security.CSRF(p.Raw)))
		return nil
	}))
	r.POST("/api/v1/auth/logout", a.Handle("", true, func(c *gin.Context, p Principal) error {
		if e := a.DB.Delete(p.Session).Error; e != nil {
			return e
		}
		c.SetCookie(Cookie, "", -1, "/api", "", a.Settings.CookieSecure, true)
		c.Status(204)
		return nil
	}))
	r.GET("/api/v1/tokens", a.Handle("", true, func(c *gin.Context, p Principal) error {
		var ts []store.Token
		if e := a.DB.Where("user_id = ?", p.User.ID).Order("created_at DESC").Find(&ts).Error; e != nil {
			return e
		}
		items := []any{}
		for _, t := range ts {
			items = append(items, tokenView(t))
		}
		c.JSON(200, gin.H{"items": items})
		return nil
	}))
	r.POST("/api/v1/tokens", a.Handle("", true, a.createToken))
	r.DELETE("/api/v1/tokens/:token_id", a.Handle("", true, func(c *gin.Context, p Principal) error {
		var t store.Token
		if a.DB.First(&t, "id = ? AND user_id = ?", c.Param("token_id"), p.User.ID).Error != nil {
			return Fail(404, "not_found", "凭证不存在。")
		}
		now := store.Now()
		if e := a.DB.Model(&t).Update("revoked_at", now).Error; e != nil {
			return e
		}
		c.Status(204)
		return nil
	}))
}
func (a *App) register(c *gin.Context) error {
	if c.GetHeader("Origin") != a.Settings.Origin {
		return Fail(403, "origin_denied", "请求来源不被允许。")
	}
	var b struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Invitation  string `json:"invitation"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	email, e := NormalizeEmail(b.Email)
	if e != nil {
		return e
	}
	name := strings.TrimSpace(b.DisplayName)
	if utf8.RuneCountInString(b.Password) < 12 || utf8.RuneCountInString(b.Password) > 256 || name == "" || utf8.RuneCountInString(name) > 60 || len(b.Invitation) < 10 || len(b.Invitation) > 100 {
		return Fail(422, "validation_error", "请求参数不正确。")
	}
	hash, e := security.HashPassword(b.Password)
	if e != nil {
		return e
	}
	var u store.User
	var raw string
	e = a.Write("", func(tx *gorm.DB) error {
		var inv store.Invitation
		if tx.First(&inv, "secret_hash = ? AND email = ?", security.Digest(b.Invitation), email).Error != nil {
			return Fail(422, "invalid_invitation", "邀请无效、已使用或已过期。")
		}
		now := store.Now()
		res := tx.Model(&store.Invitation{}).Where("id = ? AND used_at IS NULL AND expires_at > ?", inv.ID, now).Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return Fail(422, "invalid_invitation", "邀请无效、已使用或已过期。")
		}
		var n int64
		tx.Model(&store.User{}).Where("email = ?", email).Count(&n)
		if n > 0 {
			return Fail(409, "email_exists", "这个邮箱已经注册。")
		}
		u = store.User{ID: store.ID(), Email: email, DisplayName: name, PasswordHash: hash, Role: inv.Role, QuotaBytes: a.Settings.QuotaBytes, CreatedAt: now}
		if e := tx.Create(&u).Error; e != nil {
			return e
		}
		f := store.Folder{ID: store.ID(), UserID: u.ID, ParentKey: "root", Name: "收件箱", IsInbox: true}
		if e := tx.Create(&f).Error; e != nil {
			return e
		}
		var e error
		raw, e = newSession(tx, u)
		return e
	})
	if e != nil {
		return e
	}
	a.setSession(c, u, raw, 201)
	return nil
}
func (a *App) login(c *gin.Context) error {
	if c.GetHeader("Origin") != a.Settings.Origin {
		return Fail(403, "origin_denied", "请求来源不被允许。")
	}
	var b loginBody
	if e := Decode(c, &b); e != nil {
		return e
	}
	email, e := NormalizeEmail(b.Email)
	if e != nil {
		return e
	}
	if b.Password == "" || utf8.RuneCountInString(b.Password) > 256 {
		return Fail(422, "validation_error", "请求参数不正确。")
	}
	var u store.User
	found := a.DB.First(&u, "email = ?", email).Error == nil
	hash := dummyHash
	if found {
		hash = u.PasswordHash
	}
	valid := security.VerifyPassword(hash, b.Password)
	if !valid || !found || u.Disabled {
		return Fail(401, "invalid_credentials", "邮箱或密码不正确。")
	}
	var raw string
	e = a.Write(u.ID, func(tx *gorm.DB) error { var e error; raw, e = newSession(tx, u); return e })
	if e != nil {
		return e
	}
	a.setSession(c, u, raw, 200)
	return nil
}
func tokenView(t store.Token) map[string]any {
	return map[string]any{"id": t.ID, "name": t.Name, "prefix": t.Prefix, "scopes": t.Scopes, "expires_at": t.ExpiresAt, "revoked": t.RevokedAt != nil, "created_at": t.CreatedAt}
}
func (a *App) createToken(c *gin.Context, p Principal) error {
	var b struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
		Days   *int     `json:"expires_days"`
	}
	if e := Decode(c, &b); e != nil {
		return e
	}
	b.Name = strings.TrimSpace(b.Name)
	days := 30
	if b.Days != nil {
		days = *b.Days
	}
	if b.Name == "" || utf8.RuneCountInString(b.Name) > 60 || len(b.Scopes) < 1 || len(b.Scopes) > 10 || days < 1 || days > 365 {
		return Fail(422, "validation_error", "请求参数不正确。")
	}
	set := map[string]bool{}
	for _, s := range b.Scopes {
		ok := false
		for _, x := range Scopes {
			if s == x {
				ok = true
			}
		}
		if !ok {
			return Fail(422, "validation_error", "权限范围无效。")
		}
		set[s] = true
	}
	ss := store.List{}
	for s := range set {
		ss = append(ss, s)
	}
	sort.Strings(ss)
	raw := "sj_" + security.Secret()
	t := store.Token{ID: store.ID(), UserID: p.User.ID, Name: b.Name, Prefix: raw[:10], SecretHash: security.Digest(raw), Scopes: ss, ExpiresAt: store.Time{Time: time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)}, CreatedAt: store.Now()}
	if e := a.DB.Create(&t).Error; e != nil {
		return e
	}
	v := tokenView(t)
	v["secret"] = raw
	c.JSON(201, v)
	return nil
}
func IssueInvitation(db *gorm.DB, email, adminEmail string, bootstrap bool) (string, error) {
	email, e := NormalizeEmail(email)
	if e != nil {
		return "", e
	}
	raw := security.Secret()
	e = store.Write(db, "", func(tx *gorm.DB) error {
		if db.Dialector.Name() == "postgres" {
			if e := tx.Exec("SELECT pg_advisory_xact_lock(84721093)").Error; e != nil {
				return e
			}
		}
		var n int64
		if bootstrap {
			if e := tx.Model(&store.User{}).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return Fail(409, "bootstrap_denied", "管理员或有效初始邀请已存在。")
			}
			if e := tx.Model(&store.Invitation{}).Where("role = ? AND used_at IS NULL AND expires_at > ?", "admin", store.Now()).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return Fail(409, "bootstrap_denied", "管理员或有效初始邀请已存在。")
			}
		} else {
			emailAdmin, e := NormalizeEmail(adminEmail)
			if e != nil {
				return e
			}
			if e = tx.Model(&store.User{}).Where("email = ? AND role = ? AND disabled = ?", emailAdmin, "admin", false).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return Fail(403, "admin_required", "需要有效管理员账号。")
			}
		}
		role := "member"
		if bootstrap {
			role = "admin"
		}
		inv := store.Invitation{ID: store.ID(), Email: email, Role: role, SecretHash: security.Digest(raw), ExpiresAt: store.Time{Time: time.Now().UTC().Add(24 * time.Hour)}}
		return tx.Create(&inv).Error
	})
	return raw, e
}
