package api

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed static/dist
var staticFS embed.FS

// setupStaticFiles 设置静态文件服务
func (s *Server) setupStaticFiles() {
	// 获取 static/dist 子目录（Vue构建产物）
	subFS, err := fs.Sub(staticFS, "static/dist")
	if err != nil {
		// 如果 dist 目录不存在，尝试使用旧的 static 目录
		subFS, err = fs.Sub(staticFS, "static")
	if err != nil {
		return
		}
	}

	// 注册常见 MIME 类型
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".html", "text/html")
	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".ico", "image/x-icon")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")

	// 处理静态资源文件 /assets/*
	s.router.GET("/assets/*filepath", func(c *gin.Context) {
		filePath := "assets" + c.Param("filepath")
		serveEmbeddedFile(c, subFS, filePath)
	})

	// 处理根目录静态文件
	s.router.GET("/favicon.ico", func(c *gin.Context) {
		serveEmbeddedFile(c, subFS, "favicon.ico")
	})

	// SPA 路由：所有非 API 路由返回 index.html
	s.router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 如果是 API 路由，返回 404
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
			return
		}

		// 尝试作为静态文件服务
		filePath := strings.TrimPrefix(path, "/")
		if filePath != "" && !strings.Contains(filePath, "..") {
			if data, err := fs.ReadFile(subFS, filePath); err == nil {
				contentType := mime.TypeByExtension(filepath.Ext(filePath))
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				c.Data(http.StatusOK, contentType, data)
				return
			}
		}

		// 默认返回 index.html（SPA 路由）
		data, err := fs.ReadFile(subFS, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Not Found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

// serveEmbeddedFile 服务嵌入的文件
func serveEmbeddedFile(c *gin.Context, fsys fs.FS, filePath string) {
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// 根据扩展名设置 Content-Type
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Data(http.StatusOK, contentType, data)
}
