package collect

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"llamalens-cli/internal/cfg"
	"llamalens-cli/internal/logparse"
	"llamalens-cli/internal/model"
)

// JournalCollector 流式跟随 llama-server 日志（journalctl -u <unit> -f 或 tail -F <path>），
// 解析行喂给 logparse.Poller，并周期性把状态推给 World。
// 断线后 --since 补拉 catchup 秒防丢行；PID 变化（服务重启）→ 重拉 boot 块。
type JournalCollector struct {
	cfg    *cfg.Config
	world  *model.World
	poller *logparse.Poller
	events *model.EventDetector
}

func NewJournalCollector(c *cfg.Config, w *model.World, p *logparse.Poller, ev *model.EventDetector) *JournalCollector {
	return &JournalCollector{cfg: c, world: w, poller: p, events: ev}
}

// Run 启动日志采集（阻塞）。
func (j *JournalCollector) Run(ctx context.Context) {
	// 状态推送 ticker（500ms）：Poller.State() 返回副本，必须周期推送给 World，
	// 否则 UI 读到的 logst 停留在流打开时的初始状态（任务/预填充/MTP 卡片全空）
	pushTicker := time.NewTicker(500 * time.Millisecond)
	defer pushTicker.Stop()
	push := func() { j.world.SetLogState(j.poller.State()) }
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pushTicker.C:
				push()
			}
		}
	}()

	// 首次拉 boot 块（后台）：journalctl -n 50000 需 9-19s，若阻塞在补拉之前，
	// 冷启动期间日志通道全盲——短任务（4-12s）整个跑完都看不到"解码中"。
	// boot 块只含启动信息（模型/n_ctx_slot 等，HTTP 通道亦有），不阻塞任务状态补拉。
	go j.fetchBootBlock(ctx)
	push()

	// 跟随循环（进程退出则重连补拉）
	// 首次即补拉最近历史（而非只跟新行）：否则二进制启动晚于上一个任务结束时，
	// 读不到该任务的 release/汇总行 → LastTask 为空 → 预填充等卡片显示 "—"。
	catchup := true
	for ctx.Err() == nil {
		// 检查是否需要补拉 boot 块（PID 变化）
		if pid := j.poller.BootNeeded(); pid != nil {
			j.fetchBootBlock(ctx)
			j.poller.MarkBootFetched(*pid)
			push()
		}

		cmd := j.followCmd(catchup)
		catchup = false
		j.followOnce(ctx, cmd, push)
		if ctx.Err() != nil {
			return
		}
		// 进程退出 → 重连补拉
		j.poller.SetAvailable(false)
		push()
		catchup = true
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (j *JournalCollector) followCmd(catchup bool) *exec.Cmd {
	if j.cfg.LogSource == "file" {
		path := j.cfg.LogPath
		if catchup {
			return exec.Command("tail", "-n", "200", "-F", path)
		}
		return exec.Command("tail", "-n", "0", "-F", path)
	}
	unit := j.cfg.Unit
	if catchup {
		since := int(time.Now().Unix()) - 300
		return exec.Command("journalctl", "-u", unit, "-o", "short-iso", "--no-pager",
			"--since", fmt.Sprintf("@%d", since), "-f")
	}
	return exec.Command("journalctl", "-u", unit, "-o", "short-iso", "--no-pager", "-f", "-n", "0")
}

// followOnce 运行跟随命令并逐行处理，直到进程退出或 ctx 取消。
// reader goroutine 读行推 channel，主循环 select 行/ctx/done，避免阻塞。
func (j *JournalCollector) followOnce(ctx context.Context, cmd *exec.Cmd, push func()) bool {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	j.poller.SetAvailable(true)
	j.poller.SetStreamOpen()
	push()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	lines := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	cleanup := func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-done
		j.poller.SetAvailable(false)
	}
	for {
		select {
		case <-ctx.Done():
			cleanup()
			return false
		case err := <-done:
			_ = err
			j.poller.SetAvailable(false)
			return false
		case line, ok := <-lines:
			if !ok {
				cleanup()
				return false
			}
			j.poller.HandleLine(line)
		}
	}
}

func (j *JournalCollector) fetchBootBlock(ctx context.Context) {
	var cmd *exec.Cmd
	if j.cfg.LogSource == "file" {
		path := j.cfg.LogPath
		cmd = exec.CommandContext(ctx, "bash", "-c",
			fmt.Sprintf("tail -n 50000 %s 2>/dev/null | grep -B 200 'listening on' | tail -201", path))
	} else {
		unit := j.cfg.Unit
		cmd = exec.CommandContext(ctx, "bash", "-c",
			fmt.Sprintf("journalctl -u %s -o short-iso --no-pager -n 50000 2>/dev/null | grep -B 200 'listening on' | tail -201", unit))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			j.poller.ParseBootLine(line)
		}
	}
}
