package httpapi

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/hungtcs/baidupcs-console/internal/pcs"
	"github.com/hungtcs/baidupcs-console/internal/session"
)

// health 返回后端的基本信息，前端用它显示当前配置。
func (s *Server) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"cliPath":  s.cfg.CLIPath,
		"saveRoot": s.cfg.SaveRoot,
		"mode":     s.cfg.Mode,
	})
}

// browse 列出网盘目录。
func (s *Server) browse(c *echo.Context) error {
	remotePath := c.QueryParam("path")

	entries, err := s.cli.Ls(c.Request().Context(), remotePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if entries == nil {
		entries = []pcs.Entry{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"path":    pcs.NormalizeRemote(remotePath),
		"parent":  pcs.ParentRemote(remotePath),
		"entries": entries,
	})
}

// listTasks 返回全部任务，最新的在前。
func (s *Server) listTasks(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"tasks": s.manager.List()})
}

// getTask 返回单个任务。
func (s *Server) getTask(c *echo.Context) error {
	sess, err := s.manager.Get(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, sess.Task())
}

// createTaskReq 是新建下载任务的请求体。
type createTaskReq struct {
	RemotePath string `json:"remotePath"`
}

// createTask 新建并立刻启动一个下载任务。
func (s *Server) createTask(c *echo.Context) error {
	var req createTaskReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求体解析失败: "+err.Error())
	}

	// 校验远端路径要调 CLI，耗时可能超过客户端耐心；即使请求被取消，
	// 也不该把已经启动的进程一起带走，所以脱离请求上下文。
	ctx := context.WithoutCancel(c.Request().Context())

	task, err := s.manager.StartDownload(ctx, req.RemotePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, task)
}

// killTask 停止任务进程，保留任务记录与已下载的分片。
func (s *Server) killTask(c *echo.Context) error {
	task, err := s.manager.Kill(c.Param("id"))
	if err != nil {
		return taskError(err)
	}
	return c.JSON(http.StatusOK, task)
}

// restartTask 用同样的命令重新起一个进程。BaiduPCS-Go 会从断点续传。
func (s *Server) restartTask(c *echo.Context) error {
	task, err := s.manager.Restart(c.Param("id"))
	if err != nil {
		return taskError(err)
	}
	return c.JSON(http.StatusOK, task)
}

// removeTask 停止任务并从列表里移除，磁盘上已下载的文件不动。
func (s *Server) removeTask(c *echo.Context) error {
	if err := s.manager.Remove(c.Param("id")); err != nil {
		return taskError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// taskError 把管理器返回的错误映射成合适的状态码。
func taskError(err error) error {
	if err == session.ErrNotFound {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}
