package ui

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// sparkBlocks 用 ASCII 刻度（每字符 1 字节）。
// 不能用多字节块字符（▁▂▃…）：sparkline 按 sparkBlocks[level] 字节索引取字符，
// 多字节 rune 会被拆成单个字节拼出非法 UTF-8（终端显示 ?/乱码）；
// ASCII 同时保证任何终端字体都能渲染。
const sparkBlocks = " .:-=+*#"

// bar 渲染水平条：██████░░░░（按 warn/danger 着色）。
func bar(pct float64, width int, warn, danger float64) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	color := barColor(pct, warn, danger)
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)) +
		stFaint.Render(strings.Repeat("░", width-filled))
}

// barText 条 + 百分比文本。
func barText(pct float64, width int, warn, danger float64) string {
	return bar(pct, width, warn, danger) + " " + levelStyle(levelOf(pct, warn, danger)).Render(pctStr(pct))
}

func levelOf(pct, warn, danger float64) string {
	if pct >= danger {
		return "danger"
	}
	if pct >= warn {
		return "warn"
	}
	return "normal"
}

func pctStr(pct float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(pct, 'f', 1, 64), "0"), ".") + "%"
}

// sparkline 单行 Unicode 块 sparkline（值序列，nil 为断点）。
func sparkline(values []*float64, width int) string {
	var vals []float64
	min, max := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v == nil {
			continue
		}
		vals = append(vals, *v)
		if *v < min {
			min = *v
		}
		if *v > max {
			max = *v
		}
	}
	if len(vals) == 0 {
		return stFaint.Render(strings.Repeat(" ", width))
	}
	if max-min < 1e-9 {
		max = min + 1
	}
	var out []byte
	for i := 0; i < width; i++ {
		idx := 0
		if width > 1 && len(vals) > 1 {
			idx = int(float64(i) / float64(width-1) * float64(len(vals)-1))
		}
		if idx >= len(vals) {
			idx = len(vals) - 1
		}
		norm := (vals[idx] - min) / (max - min)
		level := int(norm * 7.999)
		if level < 0 {
			level = 0
		}
		if level > 7 {
			level = 7
		}
		out = append(out, sparkBlocks[level])
	}
	return stCyan.Render(string(out))
}

// sparkRange 返回序列 min/max（nil 为断点，跳过）；无有效值时 ok=false。
func sparkRange(values []*float64) (min, max float64, ok bool) {
	for _, v := range values {
		if v == nil {
			continue
		}
		if !ok {
			min, max, ok = *v, *v, true
			continue
		}
		if *v < min {
			min = *v
		}
		if *v > max {
			max = *v
		}
	}
	return
}

// gauge 简单仪表（MTP 接受率等）：条 + 大数字。
func gauge(pct *float64, width int) string {
	if pct == nil {
		return stFaint.Render(strings.Repeat(" ", width)) + stFaint.Render("  —")
	}
	// MTP 为 inverted：低=坏。用 65/80 阈值着色
	v := *pct
	color := colGreen
	if v < 65 {
		color = colRed
	} else if v < 80 {
		color = colAmber
	}
	filled := int(v / 100 * float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	b := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", filled)) +
		stFaint.Render(strings.Repeat("░", width-filled))
	return b + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(pctStr(v))
}

// panel 带边框面板（标题为框内首行）。width/height 为外尺寸（含边框）。
// 注意：lipgloss 的 Width/Height 是内尺寸（不含边框），需先减去边框。
func panel(title, content string, width, height int) string {
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}
	innerW := width - 2
	innerH := height - 2
	titleLine := stDim.Render(trunc(title, innerW))
	body := titleLine + "\n" + fitBlock(content, innerW, innerH-1)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(colBorder)).
		Width(innerW).
		Height(innerH).
		Render(body)
}

// panelNoTitle 带边框无标题的面板（用于 KPI 卡）。width/height 为外尺寸（含边框）。
func panelNoTitle(content string, width, height int) string {
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}
	innerW := width - 2
	innerH := height - 2
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(colBorder)).
		Width(innerW).
		Height(innerH).
		Padding(0, 1).
		Render(fitBlock(content, innerW-2, innerH))
}

// fitBlock 把多行内容裁剪到 width×height。
func fitBlock(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, l := range lines {
		lines[i] = trunc(l, width)
	}
	// 补齐高度
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// trunc 按显示宽度截断：ANSI 转义序列占 0 列且绝不在序列中间截断，CJK 占 2 列。
func trunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var out []byte
	w := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			end := ansiSeqEnd(s, i)
			out = append(out, s[i:end]...)
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runeWidth(r)
		if w+rw > width {
			break
		}
		out = append(out, s[i:i+size]...)
		w += rw
		i += size
	}
	return string(out)
}

func runeWidth(r rune) int {
	if r >= 0x1100 && (
		r <= 0x115F || r == 0x2329 || r == 0x232A ||
			(0x2E80 <= r && r <= 0x303E) ||
			(0x3041 <= r && r <= 0x33FF) ||
			(0x3400 <= r && r <= 0x4DBF) ||
			(0x4E00 <= r && r <= 0x9FFF) ||
			(0xA000 <= r && r <= 0xA4CF) ||
			(0xA960 <= r && r <= 0xA97F) ||
			(0xAC00 <= r && r <= 0xD7A3) ||
			(0xF900 <= r && r <= 0xFAFF) ||
			(0xFE30 <= r && r <= 0xFE4F) ||
			(0xFF00 <= r && r <= 0xFF60) ||
			(0xFFE0 <= r && r <= 0xFFE6) ||
			(0x1F300 <= r && r <= 0x1F64F) ||
			(0x20000 <= r && r <= 0x3FFFD)) {
		return 2
	}
	return 1
}

// padRight 右填充到显示宽度。
func padRight(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return trunc(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// ansiSeqEnd 返回 s 中从 i 开始的 ANSI 转义序列的结束位置（不含）；
// 若 i 不是转义序列起点则原样返回 i。正确区分 CSI 引导符 '[' 与终止字节。
func ansiSeqEnd(s string, i int) int {
	if i >= len(s) || s[i] != 0x1b {
		return i
	}
	i++
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[': // CSI：ESC [ <params/intermediates> <final 0x40-0x7E>
		i++
		for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e) {
			i++
		}
		if i < len(s) {
			i++
		}
	case ']': // OSC：ESC ] ... BEL（或 ESC \ 终止）
		i++
		for i < len(s) && s[i] != 0x07 {
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				i += 2
				break
			}
			i++
		}
		if i < len(s) {
			i++
		}
	default: // 两字节序列
		i++
	}
	return i
}

// displayWidth 返回显示宽度：ANSI 转义序列占 0 列，CJK 占 2 列。
func displayWidth(s string) int {
	w := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			i = ansiSeqEnd(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w += runeWidth(r)
		i += size
	}
	return w
}
