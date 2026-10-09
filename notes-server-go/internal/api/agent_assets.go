package api

import (
	"github.com/gin-gonic/gin"
	"shiji/internal/agentassets"
	"strings"
)

func (a *App) RegisterAgentAssets(r *gin.Engine) {
	r.GET("/api/v1/agent-access", func(c *gin.Context) { c.JSON(200, agentassets.Metadata(strings.TrimRight(a.Settings.Origin, "/"))) })
	serve := func(c *gin.Context) {
		path := strings.TrimPrefix(c.Param("path"), "/")
		data, contentType, ok := agentassets.Read(path)
		if !ok {
			c.Status(404)
			return
		}
		if path == "shiji-notes.zip" {
			c.Header("Content-Disposition", `attachment; filename="shiji-notes.zip"`)
		}
		c.Data(200, contentType, data)
	}
	r.GET("/agent/*path", serve)
	r.HEAD("/agent/*path", serve)
}
