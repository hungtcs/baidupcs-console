// BaiduPCS-Console 给 BaiduPCS-Go 套一个网页控制台。
//
// 后端本身很薄：它只负责把 BaiduPCS-Go 当成后台进程跑起来，把原始输出透传给
// 浏览器里的 xterm，再提供列目录、增删任务这几个接口。进度条怎么显示、\r 怎么
// 覆盖、颜色怎么解析，全部交给终端模拟器，后端不做任何输出解析。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/hungtcs/baidupcs-console/internal/config"
	"github.com/hungtcs/baidupcs-console/internal/httpapi"
	"github.com/hungtcs/baidupcs-console/internal/session"
)

// version 在构建时注入：-ldflags "-X main.version=v1.2.3"。
// 没注入时是 dev（直接 go build 的情况）。
var version = "dev"

func main() {
	envFile := flag.String("env", config.EnvFile, "dotenv 配置文件路径")
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	flag.Parse()

	if *showVersion {
		fmt.Printf("baidupcs-console %s\n", version)
		return
	}

	if err := run(*envFile); err != nil {
		slog.Error("启动失败", "error", err)
		os.Exit(1)
	}
}

func run(envFile string) error {
	cfg, err := config.Load(envFile)
	if err != nil {
		return err
	}

	if _, err := exec.LookPath(cfg.CLIPath); err != nil {
		return errors.New("找不到 BaiduPCS-Go 可执行文件: " + cfg.CLIPath +
			"（可用 BPCS_CLI_PATH 指定绝对路径）")
	}
	if !webBuilt() {
		slog.Warn("二进制里没有前端产物，请先在 webapp/ 目录执行 pnpm build 后重新编译")
	}

	manager, err := session.NewManager(cfg)
	if err != nil {
		return err
	}

	handler := httpapi.New(cfg, manager, webFS(), version)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Ctrl+C / SIGTERM 都走优雅退出。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("BaiduPCS-Console 已启动",
			"version", version,
			"listen", cfg.Listen,
			"cli", cfg.CLIPath,
			"saveRoot", cfg.SaveRoot,
			"dataDir", cfg.DataDir,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("收到退出信号，正在停止所有下载进程")
	}

	// 先把后台的 BaiduPCS-Go 都收掉，再关 HTTP 服务。
	manager.KillAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	slog.Info("已退出")
	return nil
}
