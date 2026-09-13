// Package cfg 定义 llamalens 的配置与命令行参数。
// 默认值对齐 ai.lan 主机的 hosts.yaml（llama 127.0.0.1:8080、进程 llama-server、
// journal unit llama-server、挂载 / 与 /share）。
package cfg

import (
	"flag"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Name            string   // 显示名（默认取 hostname）
	LlamaHost       string   // llama-server 地址
	LlamaPort       int      // llama-server 端口
	LlamaPath       string   // llama-server URL 前缀（可选，如 "v1/llama"）
	LlamaInterval   float64  // /slots 轮询间隔（秒）
	LlamaSlowInterval float64 // /props + /v1/models 轮询间隔（秒）
	LlamaTimeout    float64  // HTTP 超时（秒）
	SysInterval     float64  // 主机指标采集间隔（秒）
	ProcessName     string   // llama-server 进程名（/proc comm 精确匹配）
	Unit            string   // systemd unit 名（journalctl -u / systemctl show）
	LogSource       string   // journal | file
	LogPath         string   // 日志文件路径（LogSource=file 时必填）
	Mounts          []string // df 监控的挂载点
	Once            bool     // 单次快照（非 TUI，打印文本后退出）
	DumpFrame       string   // 每次渲染帧的原始字节写入该文件（排查终端显示问题）
	NoColor         bool     // 禁用所有颜色（纯 ASCII 渲染，排查终端颜色显示问题用）
}

func Default() *Config {
	name, _ := os.Hostname()
	return &Config{
		Name:              name,
		LlamaHost:         "127.0.0.1",
		LlamaPort:         8080,
		LlamaInterval:     1.0,
		LlamaSlowInterval: 30.0,
		LlamaTimeout:      3.0,
		SysInterval:       2.0,
		ProcessName:       "llama-server",
		Unit:              "llama-server",
		LogSource:         "journal",
		Mounts:            []string{"/"},
	}
}

// Parse 解析命令行参数。
func Parse(args []string) *Config {
	c := Default()
	fs := flag.NewFlagSet("llamalens", flag.ContinueOnError)
	fs.StringVar(&c.LlamaHost, "llama-host", c.LlamaHost, "llama-server 地址")
	fs.IntVar(&c.LlamaPort, "llama-port", c.LlamaPort, "llama-server 端口")
	fs.StringVar(&c.LlamaPath, "llama-path", c.LlamaPath, "llama-server URL 前缀（可选，如 v1/llama；http://host:port/<path>/v1/models）")
	fs.Float64Var(&c.LlamaInterval, "llama-interval", c.LlamaInterval, "/slots 轮询间隔（秒）")
	fs.Float64Var(&c.LlamaSlowInterval, "llama-slow-interval", c.LlamaSlowInterval, "/props 轮询间隔（秒）")
	fs.Float64Var(&c.LlamaTimeout, "llama-timeout", c.LlamaTimeout, "HTTP 超时（秒）")
	fs.Float64Var(&c.SysInterval, "sys-interval", c.SysInterval, "主机指标采集间隔（秒）")
	fs.StringVar(&c.ProcessName, "process", c.ProcessName, "llama-server 进程名")
	fs.StringVar(&c.Unit, "unit", c.Unit, "systemd unit 名")
	fs.StringVar(&c.LogSource, "log", c.LogSource, "日志源：journal | file")
	fs.StringVar(&c.LogPath, "log-path", c.LogPath, "日志文件路径（--log file 时必填）")
	fs.StringVar(&c.Name, "name", c.Name, "显示名")
	var mounts string
	fs.StringVar(&mounts, "mounts", strings.Join(c.Mounts, ","), "df 监控挂载点（逗号分隔）")
	fs.BoolVar(&c.Once, "once", false, "单次快照模式（打印文本后退出，非 TUI）")
	fs.StringVar(&c.DumpFrame, "dump-frame", "", "每次渲染帧的原始字节写入该文件（每次覆盖；排查终端显示问题时配合 p 暂停使用）")
	fs.BoolVar(&c.NoColor, "no-color", false, "禁用所有颜色（纯文本渲染；排查终端颜色显示问题用）")
	fs.Parse(args)

	if mounts != "" {
		c.Mounts = nil
		for _, m := range strings.Split(mounts, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				c.Mounts = append(c.Mounts, m)
			}
		}
	}
	if len(c.Mounts) == 0 {
		c.Mounts = []string{"/"}
	}
	if c.LogSource != "journal" && c.LogSource != "file" {
		c.LogSource = "journal"
	}
	if c.LogSource == "file" && c.LogPath == "" {
		// file 模式必须指定路径（与面板一致：快速失败）
		fs.Usage()
		os.Exit(2)
	}
	return c
}

// LlamaURL 返回 llama-server 基础 URL（不含尾部斜杠）。
// 若 LlamaPath 为空，返回 "http://host:port"；
// 否则返回 "http://host:port/<path>"（path 两端斜杠已修剪）。
func (c *Config) LlamaURL() string {
	base := "http://" + c.LlamaHost + ":" + strconv.Itoa(c.LlamaPort)
	if c.LlamaPath == "" {
		return base
	}
	p := strings.Trim(strings.TrimSpace(c.LlamaPath), "/")
	if p == "" {
		return base
	}
	return base + "/" + p
}
