// Package httpapi 提供 REST 接口与终端 WebSocket。
//
// 这个服务按局域网自用设计：不鉴权、直接监听 0.0.0.0。不要暴露到公网。
package httpapi

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/hungtcs/baidupcs-console/internal/config"
	"github.com/hungtcs/baidupcs-console/internal/pcs"
	"github.com/hungtcs/baidupcs-console/internal/session"
)

// Server 聚合各接口需要的依赖。
type Server struct {
	cfg     config.Config
	manager *session.Manager
	cli     *pcs.Client
	web     fs.FS
	version string
}

// New 构造 HTTP 处理器。
func New(cfg config.Config, manager *session.Manager, web fs.FS, version string) *echo.Echo {
	s := &Server{
		cfg:     cfg,
		manager: manager,
		cli:     &pcs.Client{CLIPath: cfg.CLIPath},
		web:     web,
		version: version,
	}

	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Logger:           slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})

	api := e.Group("/api")
	{
		api.GET("/health", s.health)

		api.GET("/browse", s.browse)

		api.GET("/tasks", s.listTasks)
		api.POST("/tasks", s.createTask)
		api.GET("/tasks/:id", s.getTask)
		api.DELETE("/tasks/:id", s.removeTask)
		api.POST("/tasks/:id/kill", s.killTask)
		api.POST("/tasks/:id/restart", s.restartTask)
		api.GET("/tasks/:id/terminal", s.taskTerminal)
	}

	// 前端是单页应用，未命中的路径一律回落到 index.html。
	e.GET("/*", echo.WrapHandler(spaHandler(s.web)))

	return e
}

// errorHandler 把所有错误统一成 {"error": "..."}。
func errorHandler(c *echo.Context, err error) {
	code := echo.StatusCode(err)
	if code == 0 {
		code = http.StatusInternalServerError
	}

	msg := err.Error()
	var he *echo.HTTPError
	if errors.As(err, &he) {
		msg = he.Message
	}

	if code >= 500 {
		slog.Error("请求处理失败", "error", err, "path", c.Request().URL.Path)
	}

	if jerr := c.JSON(code, map[string]string{"error": msg}); jerr != nil {
		slog.Error("写出错误响应失败", "error", jerr)
	}
}

// spaHandler 优先返回静态文件，找不到就把 index.html 交出去。
func spaHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if rel == "" || rel == "." {
			rel = "index.html"
		}

		if f, err := fsys.Open(rel); err == nil {
			_ = f.Close()
			files.ServeHTTP(w, r)
			return
		}

		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "前端资源尚未构建：请先在 webapp/ 目录执行 pnpm build，再重新编译后端。", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
