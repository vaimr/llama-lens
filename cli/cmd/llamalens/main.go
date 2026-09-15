// llamalens：在 llama-server 所在主机本地运行的 htop 风格监控 TUI。
// 零运行时依赖（静态二进制）：/proc 直读 + nvidia-smi + journalctl + 本机 HTTP API。
// 展示内容与 Web 面板（llama灵境）单主机详情页同源。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"llamalens-cli/internal/cfg"
	"llamalens-cli/internal/collect"
	"llamalens-cli/internal/logparse"
	"llamalens-cli/internal/model"
	"llamalens-cli/internal/ui"
)

func main() {
	c := cfg.Parse(os.Args[1:])

	events := model.NewEventDetector(200)
	diff := model.NewDiffEngine()
	ringL := model.NewRingBuffer(3600) // llama 序列 @1s（1h）
	ringH := model.NewRingBuffer(1800) // host 序列 @2s（1h）
	world := model.NewWorld(c, events, diff, ringL, ringH)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 信号 → 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// 采集器
	llama := collect.NewLlamaCollector(c, world, events)
	host := collect.NewHostCollector(c, world, diff, events)
	poller := logparse.NewPoller(events, c.LogSource)
	journal := collect.NewJournalCollector(c, world, poller, events)

	go llama.Run(ctx)
	go host.Run(ctx)
	go journal.Run(ctx)

	// tick @1s：速度优先级、写 llama 序列、构建快照、评估告警
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				world.Tick()
			}
		}
	}()

	if c.Once {
		runOnce(world)
		return
	}

	// --no-color：纯 ASCII 渲染（排查终端颜色显示问题；lipgloss 渲染时读全局 profile）
	if c.NoColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	app := ui.NewApp(world, c.DumpFrame)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI 启动失败:", err)
		fmt.Fprintln(os.Stderr, "提示：需要交互式终端（TTY）。可用 --once 输出单次文本快照。")
		os.Exit(1)
	}
}

// runOnce 单次快照模式：等待首次采集后打印文本快照。
func runOnce(world *model.World) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := world.Snapshot(); s != nil {
			printSnapshot(s)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("（5s 内未获得快照，llama-server 可能未启动）")
}

func printSnapshot(s *model.Snapshot) {
	fmt.Println("== llama灵境 本地快照 ==")
	fmt.Printf("主机: %s   时间: %s\n", s.HostName, time.Unix(int64(s.TS), 0).Format("2006-01-02 15:04:05"))
	online := "离线"
	if s.Llama.Online {
		online = "在线"
	}
	fmt.Printf("llama: %s   模型: %s   生成速度: %.1f tok/s   预填充: %.1f tok/s\n",
		online, orDash(s.Llama.Model.Name), s.Llama.GenTPS, s.Llama.PromptTPS)
	if ctx := s.Log.Context; ctx.Used != nil && ctx.Total != nil {
		fmt.Printf("上下文: %d/%d (%.1f%%)\n", *ctx.Used, *ctx.Total, ctxPct(ctx))
	}
	for _, g := range s.Host.GPUs {
		temp := "—"
		if g.TempC != nil {
			temp = fmt.Sprintf("%.0f°C", *g.TempC)
		}
		fmt.Printf("GPU%d %s  利用率 %.0f%%  显存 %d/%d MB  温度 %s  功耗 %s\n",
			g.Index, g.Name, g.UtilPct, g.MemUsedMB, g.MemTotalMB, temp, powerStr(g))
	}
	if cpu := s.Host.CPU.UsagePct; cpu != nil {
		fmt.Printf("CPU %.1f%%  负载 %.2f %.2f %.2f  内存 %d/%d MB\n",
			*cpu, s.Host.CPU.Load[0], s.Host.CPU.Load[1], s.Host.CPU.Load[2], s.Host.Mem.UsedMB, s.Host.Mem.TotalMB)
	}
	if s.Host.Process != nil && len(s.Host.Process) > 0 {
		for _, p := range s.Host.Process {
			fmt.Printf("进程: PID %d  运行 %s\n", p.PID, orDash(p.Elapsed))
		}
	}
	if len(s.Events) > 0 {
		fmt.Println("最近事件:")
		n := len(s.Events)
		if n > 5 {
			n = 5
		}
		for _, e := range s.Events[len(s.Events)-n:] {
			fmt.Printf("  %s %s\n", time.Unix(int64(e.TS), 0).Format("15:04:05"), e.Msg)
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func ctxPct(ctx model.Ctx) float64 {
	if ctx.Pct != nil {
		return *ctx.Pct
	}
	return 0
}

func powerStr(g model.GPU) string {
	if g.PowerW == nil {
		return "—"
	}
	if g.PowerLimitW != nil {
		return fmt.Sprintf("%.0f/%.0fW", *g.PowerW, *g.PowerLimitW)
	}
	return fmt.Sprintf("%.0fW", *g.PowerW)
}
