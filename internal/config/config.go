// Package config 负责加载 BaiduPCS-Console 的配置。
//
// 后端可配置项很少，全部走环境变量：优先读当前目录下的 .env（dotenv 格式），
// 再让进程环境变量覆盖。读取由 cleanenv 完成。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config 是全部可配置项。
type Config struct {
	// Listen 是 HTTP 监听地址。默认监听 0.0.0.0，供局域网访问。
	Listen string `env:"BPCS_LISTEN" env-default:"0.0.0.0:8080"`

	// CLIPath 是 BaiduPCS-Go 可执行文件路径，可以是绝对路径或 PATH 中的名字。
	CLIPath string `env:"BPCS_CLI_PATH" env-default:"BaiduPCS-Go"`

	// SaveRoot 是下载文件的落盘根目录。留空则取 ~/Downloads。
	SaveRoot string `env:"BPCS_SAVE_ROOT"`

	// DataDir 存放任务记录，一个任务一个 JSON 文件（<DataDir>/tasks/<id>.json）。
	// 服务重启后从这里恢复任务列表。
	DataDir string `env:"BPCS_DATA_DIR" env-default:"data"`

	// Mode 是下载模式：locate / pcs / stream。
	Mode string `env:"BPCS_MODE" env-default:"locate"`

	// MaxThreads 对应 CLI 的 -p 参数，0 表示用 CLI 自己的默认值。
	MaxThreads int `env:"BPCS_MAX_THREADS" env-default:"0"`

	// OutputBufferBytes 是每个任务在内存中保留的输出字节上限，超出后丢弃最旧部分。
	OutputBufferBytes int `env:"BPCS_OUTPUT_BUFFER_BYTES" env-default:"4194304"`
}

// EnvFile 是默认的 dotenv 文件名。
const EnvFile = ".env"

// Load 加载配置。envFile 不存在时只用进程环境变量，不报错。
func Load(envFile string) (Config, error) {
	var cfg Config

	if _, err := os.Stat(envFile); err == nil {
		// ReadConfig 会先读文件，再用进程环境变量覆盖同名项。
		if err := cleanenv.ReadConfig(envFile, &cfg); err != nil {
			return cfg, fmt.Errorf("读取 %s: %w", envFile, err)
		}
	} else {
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return cfg, fmt.Errorf("读取环境变量: %w", err)
		}
	}

	cfg.normalize()
	return cfg, nil
}

// normalize 修正缺失或非法的项，使其总是处于可用状态。
func (c *Config) normalize() {
	if c.Listen == "" {
		c.Listen = "0.0.0.0:8080"
	}
	if c.CLIPath == "" {
		c.CLIPath = "BaiduPCS-Go"
	}

	if strings.TrimSpace(c.DataDir) == "" {
		c.DataDir = "data"
	}

	c.SaveRoot = expandHome(c.SaveRoot)
	if c.SaveRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.SaveRoot = filepath.Join(home, "Downloads")
		} else {
			c.SaveRoot = "downloads"
		}
	}

	switch c.Mode {
	case "pcs", "stream", "locate":
	default:
		c.Mode = "locate"
	}
	if c.MaxThreads < 0 {
		c.MaxThreads = 0
	}
	if c.OutputBufferBytes < 64<<10 {
		c.OutputBufferBytes = 4 << 20
	}
}

func expandHome(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if len(p) == 1 {
		return home
	}
	if p[1] == '/' || p[1] == filepath.Separator {
		return filepath.Join(home, p[2:])
	}
	return p
}
