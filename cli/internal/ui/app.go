package ui

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"llamalens-cli/internal/model"
)

const renderInterval = 250 * time.Millisecond // 4fps 渲染

type tickMsg struct{}

// App bubbletea Model：管理终端尺寸、按键、渲染循环。
// 终端生命周期（raw 模式/备用屏幕/光标/缩放/退出恢复）全部由 bubbletea 负责，
// 本代码不手写任何 ANSI 转义序列。
type App struct {
	world      *model.World
	width      int
	height     int
	paused     bool
	showTrends bool
	showGPU    bool
	frozen     *model.Snapshot
	dumpFrame  string
	dumpWarned bool
}

func NewApp(world *model.World, dumpFrame string) *App {
	return &App{world: world, showTrends: true, showGPU: true, dumpFrame: dumpFrame}
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), tea.Every(renderInterval, func(t time.Time) tea.Msg { return tickMsg{} }))
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "p":
			a.paused = !a.paused
			if a.paused {
				a.frozen = a.world.Snapshot()
			} else {
				a.frozen = nil
			}
			return a, nil
		case "t":
			a.showTrends = !a.showTrends
			return a, nil
		case "g":
			a.showGPU = !a.showGPU
			return a, nil
		}
	case tickMsg:
		return a, tea.Every(renderInterval, func(t time.Time) tea.Msg { return tickMsg{} })
	}
	return a, nil
}

func (a *App) View() string {
	snap := a.world.Snapshot()
	if a.paused && a.frozen != nil {
		snap = a.frozen
	}
	if snap == nil {
		return stDim.Render("  加载中…（等待首次采集）")
	}
	view := Render(snap, a.world, a.width, a.height, options{
		showTrends: a.showTrends,
		showGPU:    a.showGPU,
		paused:     a.paused,
	})
	// --dump-frame：把本帧原始字节（含 ANSI）写入文件，每次渲染覆盖。
	// 用于排查"程序写出的字节"与"终端显示"不一致的问题：
	// 复现时按 p 暂停，文件即保留该帧，cat -v 查看原始字节。
	if a.dumpFrame != "" {
		if err := os.WriteFile(a.dumpFrame, []byte(view), 0o644); err != nil && !a.dumpWarned {
			a.dumpWarned = true
			fmt.Fprintf(os.Stderr, "警告：--dump-frame 写入失败：%v\n", err)
		}
	}
	return view
}
