package api

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"shiji/internal/config"
	"shiji/internal/store"
	"strings"
	"sync"
)

type App struct {
	DB                       *gorm.DB
	Settings                 config.Settings
	ExportSlots, UploadSlots chan struct{}
	agentRateMu              sync.Mutex
	agentRates               map[string]agentRate
}
type Principal struct {
	User    store.User
	Session *store.Session
	Token   *store.Token
	Raw     string
}

func (p Principal) ActorKey() string {
	if p.Token != nil {
		return p.Token.ID
	}
	return p.Session.ID
}
func (p Principal) Actor() string {
	if p.Token != nil {
		return "agent"
	}
	return "web"
}

type Error struct {
	Status        int
	Code, Message string
	Details       any
}

func (e *Error) Error() string { return e.Message }
func Fail(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}
func RespondError(c *gin.Context, e error) {
	var a *Error
	if !errors.As(e, &a) {
		a = Fail(500, "internal_error", "服务暂时不可用。")
		if strings.Contains(e.Error(), "UNIQUE constraint") || strings.Contains(e.Error(), "duplicate key") {
			a = Fail(409, "conflict", "数据已存在或关联无效。")
		}
	}
	c.AbortWithStatusJSON(a.Status, gin.H{"error": gin.H{"code": a.Code, "message": a.Message, "details": a.Details}, "request_id": c.GetString("request_id")})
}
func Decode(c *gin.Context, out any) error {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20))
	dec.DisallowUnknownFields()
	check := func(e error) error {
		var limit *http.MaxBytesError
		if errors.As(e, &limit) {
			return Fail(413, "body_size", "请求内容过大。")
		}
		return Fail(422, "validation_error", "请求参数不正确。")
	}
	if e := dec.Decode(out); e != nil {
		return check(e)
	}
	var extra any
	if e := dec.Decode(&extra); e != io.EOF {
		return check(e)
	}
	return nil
}

type Handler func(*gin.Context, Principal) error

func (a *App) Handle(scope string, webOnly bool, fn Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, e := a.Authenticate(c, scope, webOnly)
		if e == nil {
			e = fn(c, p)
		}
		if e != nil {
			RespondError(c, e)
		}
	}
}
func (a *App) Write(userID string, fn func(*gorm.DB) error) error {
	return store.Write(a.DB, userID, func(tx *gorm.DB) error {
		return fn(tx.Set("history_limit", a.Settings.HistoryLimit).Session(&gorm.Session{}))
	})
}
func New(db *gorm.DB, s config.Settings) *App {
	return &App{DB: db, Settings: s, ExportSlots: make(chan struct{}, s.ExportConcurrency), UploadSlots: make(chan struct{}, 2)}
}
func (a *App) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		id := store.ID()
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				RespondError(c, Fail(500, "internal_error", "服务暂时不可用。"))
			}
		}()
		c.Next()
	})
	r.MaxMultipartMemory = 1 << 20
	r.GET("/api/v1/health/live", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/v1/health/ready", func(c *gin.Context) {
		if e := store.Check(a.DB); e != nil {
			RespondError(c, e)
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})
	r.GET("/openapi.json", func(c *gin.Context) { c.Data(200, "application/json; charset=utf-8", openAPI) })
	a.RegisterAuth(r)
	a.RegisterAgentAssets(r)
	a.RegisterAttachments(r)
	a.RegisterNotes(r)
	a.RegisterExports(r)
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			RespondError(c, Fail(404, "not_found", "接口不存在。"))
			return
		}
		if a.Settings.WebDir != "" {
			rel := strings.TrimPrefix(c.Request.URL.Path, "/")
			if rel != "" && !filepath.IsLocal(rel) {
				c.Status(404)
				return
			}
			p := filepath.Join(a.Settings.WebDir, rel)
			if st, e := os.Stat(p); e == nil && !st.IsDir() {
				c.File(p)
				return
			}
			c.File(filepath.Join(a.Settings.WebDir, "index.html"))
			return
		}
		c.Status(http.StatusNotFound)
	})
	return r
}
