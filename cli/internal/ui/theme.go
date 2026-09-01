// Package ui 渲染 htop 风格 TUI（bubbletea + lipgloss）。
// 配色对齐前端 Terminal 主题（terminal.css）。
package ui

import "github.com/charmbracelet/lipgloss"

// 颜色（前端 terminal.css）
const (
	colCyan   = "#56b6c2"
	colGreen  = "#98c379"
	colAmber  = "#e5c07b"
	colRed    = "#e07d85" // 对齐 colBarR：#e06c75 在 ANSI256 终端被 termenv 映射到 232(#080808) 近黑不可见
	colText   = "#c8c8cc"
	colDim    = "#787880"
	colFaint  = "#48484e"
	colBorder = "#55555e" // 提亮：TUI 无卡片底色，#242428 在深色终端上几乎不可见
	colBarG   = "#a8c890"
	colBarA   = "#e8d090"
	colBarR   = "#e07d85"
)

var (
	stText   = lipgloss.NewStyle().Foreground(lipgloss.Color(colText))
	stDim    = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))
	stFaint  = lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint))
	stCyan   = lipgloss.NewStyle().Foreground(lipgloss.Color(colCyan))
	stGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen))
	stAmber  = lipgloss.NewStyle().Foreground(lipgloss.Color(colAmber))
	stRed    = lipgloss.NewStyle().Foreground(lipgloss.Color(colRed))
	stBold   = lipgloss.NewStyle().Foreground(lipgloss.Color(colText)).Bold(true)
	stCyanB  = lipgloss.NewStyle().Foreground(lipgloss.Color(colCyan)).Bold(true)
	stGreenB = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen)).Bold(true)
	stAmberB = lipgloss.NewStyle().Foreground(lipgloss.Color(colAmber)).Bold(true)
)

// levelStyle 按告警级别返回样式。
func levelStyle(level string) lipgloss.Style {
	switch level {
	case "danger":
		return stRed
	case "warn":
		return stAmber
	default:
		return stText
	}
}

// barColor 按百分比返回条颜色（绿/黄/红）。
func barColor(pct float64, warn, danger float64) lipgloss.Color {
	if pct >= danger {
		return lipgloss.Color(colBarR)
	}
	if pct >= warn {
		return lipgloss.Color(colBarA)
	}
	return lipgloss.Color(colBarG)
}
