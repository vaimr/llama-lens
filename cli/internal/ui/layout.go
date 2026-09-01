package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"llamalens-cli/internal/model"
)

type options struct {
	showTrends bool
	showGPU    bool
	paused     bool
}

// Render 把快照渲染成全屏 TUI（宽度 w、高度 h）。
func Render(snap *model.Snapshot, world *model.World, w, h int, opts options) string {
	if snap == nil {
		return stDim.Render("加载中…")
	}
	if w < 80 || h < 20 {
		return stAmber.Render(fmt.Sprintf("终端过小（%dx%d），请调整到至少 80x20", w, h))
	}

	usable := w - 2
	var b strings.Builder

	// 高度预算（均为外尺寸，含边框）
	// 固定行 = topBar + 分区标题 + KPI 卡 + 系统资源卡 + [历史趋势]
	topBarH := 1
	kpiH := 5 // KPI 卡：3 行内容（标题/数值/条或 sparkline）+ 2 行边框
	sysH := 6 // 系统资源卡：4 行内容（标题/条/数值/每核条）+ 2 行边框
	trendsH := 1 // 历史趋势为单行 sparkline
	nTitles := 5 // 实时总览 / GPU / 实时生成任务 / 系统资源 / 模型与 Slot·进程
	if !opts.showGPU {
		nTitles = 4
	}
	fixed := topBarH + nTitles + kpiH + sysH
	if opts.showTrends {
		fixed += 1 + trendsH
	}
	rest := h - fixed
	if rest < 12 {
		rest = 12
	}
	var gpuH, taskH, modelH int
	if opts.showGPU {
		// GPU 是 llama 主资源，权重最高；任务/模型次之
		gpuH = rest * 40 / 100
		taskH = rest * 35 / 100
		modelH = rest - gpuH - taskH
	} else {
		taskH = rest / 2
		modelH = rest - taskH
	}
	if gpuH < 4 {
		gpuH = 4
	}
	if taskH < 4 {
		taskH = 4
	}
	if modelH < 4 {
		modelH = 4
	}

	// TopBar
	b.WriteString(renderTopBar(snap, w, opts))
	b.WriteString("\n")

	// 实时总览
	b.WriteString(sectionTitle("实时总览", usable))
	b.WriteString("\n")
	b.WriteString(renderKPI(snap, world, usable, kpiH))
	b.WriteString("\n")

	// GPU
	if opts.showGPU {
		b.WriteString(sectionTitle("GPU（按卡聚合）", usable))
		b.WriteString("\n")
		b.WriteString(renderGPU(snap, usable, gpuH))
		b.WriteString("\n")
	}

	// 实时生成任务 + 事件流
	b.WriteString(sectionTitle("实时生成任务", usable))
	b.WriteString("\n")
	b.WriteString(renderTask(snap, usable, taskH))
	b.WriteString("\n")

	// 系统资源
	b.WriteString(sectionTitle("系统资源", usable))
	b.WriteString("\n")
	b.WriteString(renderSys(snap, usable, sysH))
	b.WriteString("\n")

	// 模型与 Slot + 进程
	b.WriteString(sectionTitle("模型与 Slot · 进程", usable))
	b.WriteString("\n")
	b.WriteString(renderModelProc(snap, usable, modelH))
	b.WriteString("\n")

	// 历史趋势（标题行右侧图例：解释 sparkline 字符刻度）
	if opts.showTrends {
		b.WriteString(sectionTitleLegend("历史趋势", "趋势 .:-=+*# 低→高", usable))
		b.WriteString("\n")
		b.WriteString(renderTrends(snap, world, usable))
		b.WriteString("\n")
	}

	out := b.String()
	// 整体裁剪到终端高度（防止超出）
	lines := strings.Split(out, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	// 每行宽度控制在 w-1 内：写满最后一列会触发终端自动换行，导致后续行整体错位
	for i, l := range lines {
		lines[i] = trunc(l, w-1)
	}
	return strings.Join(lines, "\n")
}

// ---------- TopBar ----------

func renderTopBar(snap *model.Snapshot, w int, opts options) string {
	name := snap.HostName
	modelName := snap.Llama.Model.Name
	// 与 Web TopBar 一致：alias 为完整路径时（定制 build 的 model_alias）只显示文件名
	if modelName != "" {
		modelName = filepath.Base(modelName)
	}
	if modelName == "" {
		modelName = "—"
	}
	online := "● 在线"
	onlineStyle := stGreen
	if !snap.Llama.Online {
		online = "● 离线"
		onlineStyle = stRed
	}
	now := time.Now().Format("15:04:05")
	right := stFaint.Render("q 退出  p 暂停  t 趋势  g GPU")
	if opts.paused {
		right = stAmberB.Render("已暂停") + "  " + stFaint.Render("q 退出  p 恢复  t 趋势  g GPU")
	}
	// 为右侧时间+按键预留宽度；整行控制在 w-2 内（与分区标题右缘对齐；不写满最后一列，避免终端自动换行错位）
	reserve := displayWidth(now) + 1 + displayWidth(right)
	leftW := (w - 2) - reserve
	if leftW < 10 {
		leftW = 10
	}
	// 左侧分段：品牌 · 主机 · 模型  在线状态。空间不足时先截模型名、再截主机名，
	// 保证在线状态始终可见（整段 left 截断会先砍掉末尾的在线状态）
	brand := stBold.Render("llama灵境")
	sep := stDim.Render(" · ")
	onlinePart := "  " + onlineStyle.Render(online)
	namePart := stText.Render(name)
	fixedW := displayWidth(brand) + 2*displayWidth(sep) + displayWidth(namePart) + displayWidth(onlinePart)
	modelW := leftW - fixedW
	if modelW < 2 {
		nameW := leftW - (fixedW - displayWidth(namePart)) - 2
		if nameW < 0 {
			nameW = 0
		}
		namePart = stText.Render(trunc(name, nameW))
		fixedW = displayWidth(brand) + 2*displayWidth(sep) + displayWidth(namePart) + displayWidth(onlinePart)
		modelW = leftW - fixedW
	}
	if modelW < 0 {
		modelW = 0
	}
	left := brand + sep + namePart + sep + stCyan.Render(trunc(modelName, modelW)) + onlinePart
	return padRight(left, leftW) + stDim.Render(now) + " " + right
}

// ---------- 分区标题 ----------

func sectionTitle(title string, width int) string {
	t := stDim.Render(" " + title + " ")
	dash := width - displayWidth(t)
	if dash < 0 {
		dash = 0
	}
	return t + stFaint.Render(strings.Repeat("─", dash))
}

// sectionTitleLegend 分区标题 + 右侧图例（趋势行用：解释 sparkline 字符刻度）。
func sectionTitleLegend(title, legend string, width int) string {
	t := stDim.Render(" " + title + " ")
	leg := stFaint.Render(" " + legend)
	dash := width - displayWidth(t) - displayWidth(leg)
	if dash < 0 {
		dash = 0
	}
	return t + stFaint.Render(strings.Repeat("─", dash)) + leg
}

// ---------- KPI 行 ----------

func renderKPI(snap *model.Snapshot, world *model.World, usable, h int) string {
	// lipgloss.JoinHorizontal 不插入间隙，卡片宽度按无间隙计算，整行精确等于 usable
	cw := usable / 4
	cw4 := usable - cw*3 // 最后一张补齐
	var cards []string
	cards = append(cards, kpiToken(snap, world, cw, h))
	cards = append(cards, kpiPrompt(snap, world, cw, h))
	cards = append(cards, kpiCtx(snap, cw, h))
	cards = append(cards, kpiMTP(snap, cw4, h))
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func kpiToken(snap *model.Snapshot, world *model.World, w, h int) string {
	val := "—"
	if snap.Llama.Online {
		val = fmt.Sprintf("%.1f", snap.Llama.GenTPS)
	}
	title := "Token 生成速度"
	var vstyle lipgloss.Style
	if snap.Llama.Online && snap.Llama.GenTPS > 0 {
		vstyle = stCyanB
	} else {
		vstyle = stFaint
	}
	body := stDim.Render(title) + "\n" +
		vstyle.Render(padRight(val, kpiValPad(w))) + stDim.Render(" tok/s") + "\n" +
		kpiSparkRange(world, "gen_speed", w-4)
	return panelNoTitle(body, w, h)
}

// kpiValPad KPI 数值右填充宽度：常规 10（对齐 tok/s 后缀），窄卡自动收窄防后缀被裁。
func kpiValPad(w int) int {
	pad := w - 10 // 内容宽 w-4 减去 " tok/s" 6 列
	if pad > 10 {
		pad = 10
	}
	if pad < 4 {
		pad = 4
	}
	return pad
}

func kpiPrompt(snap *model.Snapshot, world *model.World, w, h int) string {
	title := "预填充速度"
	val := "—"
	sub := ""
	// 仅预填充进行中显示实时值；结束后显示 "—"（不留过期的"上次"值）
	if snap.Log.Phase == "prompt_processing" && snap.Log.PromptSpeedTPS != nil {
		val = fmt.Sprintf("%.0f", *snap.Log.PromptSpeedTPS)
		if snap.Log.PromptProgress != nil {
			sub = fmt.Sprintf(" (%.0f%%)", *snap.Log.PromptProgress*100)
		}
	}
	var vstyle lipgloss.Style
	if val != "—" {
		vstyle = stGreenB
	} else {
		vstyle = stFaint
	}
	body := stDim.Render(title) + "\n" +
		vstyle.Render(padRight(val, kpiValPad(w))) + stDim.Render(" tok/s") + stDim.Render(sub) + "\n" +
		kpiSparkRange(world, "prompt_speed", w-4)
	return panelNoTitle(body, w, h)
}

func kpiCtx(snap *model.Snapshot, w, h int) string {
	title := "上下文占用"
	ctx := snap.Log.Context
	usedStr := "—"
	totalStr := ""
	pctTxt := ""
	level := "normal"
	if ctx.Used != nil {
		usedStr = fmt.Sprintf("%d", *ctx.Used)
	}
	if ctx.Total != nil {
		totalStr = fmt.Sprintf("/%d", *ctx.Total)
	}
	if ctx.Pct != nil {
		pctTxt = pctStr(*ctx.Pct)
		level = levelOf(*ctx.Pct, 80, 90)
	}
	contentW := w - 4
	if contentW < 15 {
		title = "上下文"
	}
	barW := contentW
	sub := ""
	if contentW >= 18 && ctx.Remaining != nil {
		sub = stFaint.Render(fmt.Sprintf("  剩余 %d", *ctx.Remaining))
		barW = contentW - displayWidth(sub)
	}
	if ctx.Truncated {
		sub += stRed.Render(" 截断")
	}
	body := stDim.Render(title) + " " + levelStyle(level).Render(pctTxt) + "\n" +
		stText.Render(usedStr+totalStr) + "\n" +
		barCtx(ctx, barW) + sub
	return panelNoTitle(body, w, h)
}

func barCtx(ctx model.Ctx, w int) string {
	if ctx.Total == nil || *ctx.Total == 0 || ctx.Used == nil {
		return stFaint.Render(strings.Repeat("░", w))
	}
	pct := float64(*ctx.Used) / float64(*ctx.Total) * 100
	return bar(pct, w, 80, 90)
}

func kpiMTP(snap *model.Snapshot, w, h int) string {
	title := "MTP 接受率"
	mtp := snap.Log.MTP
	// acceptance 为分数（0-1，与后端/前端一致），gauge 按百分比（0-100）渲染
	var acc *float64
	if mtp.Acceptance != nil {
		v := *mtp.Acceptance * 100
		acc = &v
	}
	body := stDim.Render(title) + "\n" +
		gauge(acc, w-10) + "\n" +
		mtpSub(mtp)
	return panelNoTitle(body, w, h)
}

func mtpSub(mtp model.MTP) string {
	if mtp.Accepted == nil || mtp.Generated == nil {
		return stFaint.Render("—")
	}
	return stFaint.Render(fmt.Sprintf("%d/%d 接受", *mtp.Accepted, *mtp.Generated))
}

// kpiSparkRange sparkline + min~max 范围标注（让趋势可读：字符高度=数值，
// 刻度 .:-=+*# 低→高）。空间不足时自动退化为纯 sparkline。
func kpiSparkRange(world *model.World, series string, w int) string {
	vals := ringValues(world, series, w)
	min, max, ok := sparkRange(vals)
	if !ok || w < 10 {
		return sparkline(vals, w)
	}
	var rangeStr string
	if int(min+0.5) == int(max+0.5) {
		rangeStr = fmt.Sprintf("  %d", int(min+0.5))
	} else {
		rangeStr = fmt.Sprintf("  %d~%d", int(min+0.5), int(max+0.5))
	}
	sparkW := w - displayWidth(rangeStr)
	if sparkW < 4 {
		return sparkline(vals, w)
	}
	return sparkline(vals, sparkW) + stFaint.Render(rangeStr)
}

// ---------- GPU ----------

func renderGPU(snap *model.Snapshot, usable, h int) string {
	gpus := snap.Host.GPUs
	if len(gpus) == 0 {
		return stFaint.Render("  无 GPU 数据（nvidia-smi 不可用）")
	}
	n := len(gpus)
	if n > 4 {
		n = 4
	}
	// lipgloss.JoinHorizontal 不插入间隙，卡片宽度按无间隙计算，整行精确等于 usable
	cw := usable / n
	cwLast := usable - cw*(n-1) // 最后一张补齐余数，整行右缘与其他分区对齐
	var cards []string
	for i := 0; i < n; i++ {
		cwCard := cw
		if i == n-1 {
			cwCard = cwLast
		}
		cards = append(cards, gpuCard(snap, gpus[i], cwCard, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func gpuCard(snap *model.Snapshot, g model.GPU, w, h int) string {
	title := fmt.Sprintf("GPU%d · %s", g.Index, g.Name)
	// 利用率/显存横条统一对齐：标签 "利用率 " 宽 7 列，两条同宽 barW、同左起点，
	// 左右两侧都对齐；barW=w-16 保证 "利用率 [bar] 100%" 不超内容宽 (w-4)。
	barW := w - 16
	lines := []string{
		stDim.Render(title),
		"利用率 " + barText(g.UtilPct, barW, 80, 90),
	}
	// 显存（数值行 + 横条；阈值 85/95 与 Web 面板同源）
	memPct := 0.0
	if g.MemTotalMB > 0 {
		memPct = float64(g.MemUsedMB) / float64(g.MemTotalMB) * 100
	}
	lines = append(lines, fmt.Sprintf("显存 %d/%d MB", g.MemUsedMB, g.MemTotalMB))
	// 百分比取整（与利用率一致）：一位小数（如 90.9%）在窄卡下会超出内容宽被截掉 %
	lines = append(lines, strings.Repeat(" ", 7)+bar(memPct, barW, 85, 95)+" "+
		levelStyle(levelOf(memPct, 85, 95)).Render(fmt.Sprintf("%.0f%%", memPct)))
	// 温度 / 功耗 / 风扇
	temp := "—"
	if g.TempC != nil {
		temp = fmt.Sprintf("%.0f°C", *g.TempC)
	}
	power := "—"
	if g.PowerW != nil {
		power = fmt.Sprintf("%.0fW", *g.PowerW)
		if g.PowerLimitW != nil {
			power = fmt.Sprintf("%.0f/%.0fW", *g.PowerW, *g.PowerLimitW)
		}
	}
	fan := "—"
	if g.FanPct != nil {
		fan = pctStr(*g.FanPct)
	}
	lines = append(lines, fmt.Sprintf("温度 %s  功耗 %s  风扇 %s", temp, power, fan))
	// 频率 / PCIe / P-State
	clock := "—"
	if g.ClockMHz != nil {
		clock = fmt.Sprintf("%dMHz", *g.ClockMHz)
	}
	pcie := "—"
	if g.PCIEGen != nil && g.PCIEWidth != nil {
		pcie = fmt.Sprintf("gen%d x%d", *g.PCIEGen, *g.PCIEWidth)
	}
	lines = append(lines, fmt.Sprintf("频率 %s  PCIe %s  P-State %s", clock, pcie, orDash(g.PState)))
	// 降频 / ECC
	throttle := "无"
	if g.Throttle != 0 {
		throttle = fmt.Sprintf("0x%x", g.Throttle)
	}
	ecc := "—"
	if g.ECCCorrected != nil && g.ECCUncorrected != nil {
		ecc = fmt.Sprintf("%d/%d", *g.ECCCorrected, *g.ECCUncorrected)
	}
	throttleStyle := stText
	if g.Throttle != 0 {
		throttleStyle = stAmber
	}
	lines = append(lines, fmt.Sprintf("降频 %s  ECC %s", throttleStyle.Render(throttle), ecc))
	// 占用进程（全部显示，与 Web 对齐；显存为该卡占用；过多时 fitBlock 截断）
	for _, a := range g.Apps {
		lines = append(lines, stDim.Render(fmt.Sprintf("占用 %s pid%d %dMB", a.Name, a.PID, a.MemMB)))
	}
	return panelNoTitle(strings.Join(lines, "\n"), w, h)
}

// ---------- 任务 + 事件 ----------

func renderTask(snap *model.Snapshot, usable, h int) string {
	// lipgloss.JoinHorizontal 不插入间隙，两面板宽度之和精确等于 usable
	taskW := usable * 40 / 100
	evW := usable - taskW
	return lipgloss.JoinHorizontal(lipgloss.Top,
		panel("实时生成任务", taskBody(snap), taskW, h),
		panel("事件流", eventsBody(snap), evW, h))
}

func taskBody(snap *model.Snapshot) string {
	logst := snap.Log
	phase := logst.Phase
	// 状态以 /slots API 为准（任一 slot is_processing 即生成中）：
	// journal 流有延迟/丢行，任务开始时日志 phase 滞后于 API，
	// 会出现「Token 速度有值、状态却空闲」。日志 phase 仅用于细化标签。
	busy := false
	for _, s := range snap.Llama.Slots {
		if s.IsProcessing {
			busy = true
			break
		}
	}
	// 任务 ID：生成中以 API 的 slot.id_task 为准（日志 task_id 可能是上一任务残留）；
	// 空闲时保留日志值（上一任务 ID）。
	taskID := logst.TaskID
	if busy {
		for _, s := range snap.Llama.Slots {
			if s.IsProcessing && s.IDTask != nil {
				taskID = s.IDTask
				break
			}
		}
	}
	phaseStr := "空闲"
	phaseStyle := stDim
	switch {
	case busy && phase == "decoding":
		phaseStr = "解码中"
		phaseStyle = stCyan
	case busy && phase == "prompt_processing":
		phaseStr = "预填充"
		phaseStyle = stGreen
	case busy:
		phaseStr = "生成中"
		phaseStyle = stCyan
	}
	var lines []string
	lines = append(lines, "状态 "+phaseStyle.Render(phaseStr)+"  "+taskIDStr(taskID))
	if busy && phase == "decoding" {
		// 「已解码 + 实时速度」合并一行：小终端（120x35）任务面板仅 2 行内容，
		// 速度独占一行会被 fitBlock 截掉不可见。
		if logst.NDecoded > 0 || logst.Tg3sTPS != nil {
			var parts []string
			if logst.NDecoded > 0 {
				parts = append(parts, fmt.Sprintf("已解码 %d tokens", logst.NDecoded))
			}
			if logst.Tg3sTPS != nil {
				parts = append(parts, fmt.Sprintf("实时 %.2f t/s", *logst.Tg3sTPS))
			}
			lines = append(lines, strings.Join(parts, " · "))
		}
		if logst.TgTPS != nil {
			lines = append(lines, fmt.Sprintf("任务均速 %.2f t/s", *logst.TgTPS))
		}
		if mtp := logst.MTP.Acceptance; mtp != nil {
			lines = append(lines, fmt.Sprintf("MTP 接受率 %.1f%%", *mtp*100))
		}
		if r := slotRemain(snap); r != nil {
			lines = append(lines, fmt.Sprintf("剩余 %d tokens", *r))
		}
	} else if busy && phase == "prompt_processing" {
		if logst.PromptSpeedTPS != nil {
			lines = append(lines, fmt.Sprintf("Prompt 速度 %.0f t/s", *logst.PromptSpeedTPS))
		}
		if logst.PromptProgress != nil {
			lines = append(lines, "进度 "+bar(*logst.PromptProgress*100, 12, 0, 101)+" "+pctStr(*logst.PromptProgress*100))
		}
		if logst.PromptTotalTokens != nil {
			lines = append(lines, fmt.Sprintf("Prompt 总量 %d tokens", *logst.PromptTotalTokens))
		}
	} else if !busy {
		lines = append(lines, stDim.Render("空闲 · 等待新任务"))
		if lt := logst.LastTask; lt != nil {
			lines = append(lines, lastTaskLine(lt))
		}
	}
	// 上下文
	if ctx := logst.Context; ctx.Used != nil && ctx.Total != nil {
		lines = append(lines, fmt.Sprintf("上下文 %d/%d (%s)", *ctx.Used, *ctx.Total, pctStr(ctxPct(ctx))))
	}
	return strings.Join(lines, "\n")
}

func lastTaskLine(lt *model.TaskSummary) string {
	parts := []string{"上次任务"}
	if lt.TaskID != nil {
		parts[0] = fmt.Sprintf("上次任务 #%d", *lt.TaskID)
	}
	if lt.TotalTokens != nil {
		parts = append(parts, fmt.Sprintf("%d tokens", *lt.TotalTokens))
	}
	if lt.GenSpeedTPS != nil {
		parts = append(parts, fmt.Sprintf("%.1f t/s", *lt.GenSpeedTPS))
	}
	return stDim.Render(strings.Join(parts, " · "))
}

func eventsBody(snap *model.Snapshot) string {
	events := snap.Events
	if len(events) == 0 {
		return stFaint.Render("（无事件）")
	}
	// 取最近 12 条，最新在最上（panel 的 fitBlock 保留顶部行，正序会只显示最旧事件）
	start := len(events) - 12
	if start < 0 {
		start = 0
	}
	var lines []string
	for i := len(events) - 1; i >= start; i-- {
		e := events[i]
		ts := time.Unix(int64(e.TS), 0).Format("15:04:05")
		style := stText
		switch e.Level {
		case "danger", "error":
			style = stRed
		case "warn":
			style = stAmber
		}
		lines = append(lines, stFaint.Render(ts)+" "+style.Render(e.Msg))
	}
	return strings.Join(lines, "\n")
}

// ---------- 系统资源 ----------

func renderSys(snap *model.Snapshot, usable, h int) string {
	// lipgloss.JoinHorizontal 不插入间隙，卡片宽度按无间隙计算，整行精确等于 usable
	cw := usable / 4
	cw4 := usable - cw*3
	return lipgloss.JoinHorizontal(lipgloss.Top,
		cpuCard(snap, cw, h),
		memCard(snap, cw, h),
		diskCard(snap, cw, h),
		netCard(snap, cw4, h))
}

func cpuCard(snap *model.Snapshot, w, h int) string {
	cpu := snap.Host.CPU
	barW := w - 8
	lines := []string{
		stDim.Render("CPU"),
		barText(valOr(cpu.UsagePct, 0), barW-6, 80, 90),
		fmt.Sprintf("load %.2f %.2f %.2f", cpu.Load[0], cpu.Load[1], cpu.Load[2]),
	}
	// 每核（最多显示 8 核，迷你条；数量按卡宽限制防溢出）
	if len(cpu.PerCorePct) > 0 {
		n := len(cpu.PerCorePct)
		if n > 8 {
			n = 8
		}
		if maxN := (w - 3) / 3; n > maxN {
			n = maxN
		}
		if n < 1 {
			n = 1
		}
		perCore := make([]string, n)
		for i := 0; i < n; i++ {
			p := cpu.PerCorePct[i]
			bw := (barW - n*2) / n
			if bw < 2 {
				bw = 2
			}
			perCore[i] = bar(p, bw, 80, 90)
		}
		lines = append(lines, strings.Join(perCore, " "))
	}
	return panelNoTitle(strings.Join(lines, "\n"), w, h)
}

func memCard(snap *model.Snapshot, w, h int) string {
	mem := snap.Host.Mem
	totalGB := float64(mem.TotalMB) / 1024
	usedGB := float64(mem.UsedMB) / 1024
	pct := 0.0
	if mem.TotalMB > 0 {
		pct = float64(mem.UsedMB) / float64(mem.TotalMB) * 100
	}
	barW := w - 8
	lines := []string{
		stDim.Render("内存"),
		barText(pct, barW-6, 85, 95),
		fmt.Sprintf("%.1f/%.1f GB", usedGB, totalGB),
		fmt.Sprintf("swap %d/%d MB", mem.SwapUsedMB, mem.SwapTotalMB),
	}
	return panelNoTitle(strings.Join(lines, "\n"), w, h)
}

func diskCard(snap *model.Snapshot, w, h int) string {
	disk := snap.Host.Disk
	lines := []string{stDim.Render("磁盘")}
	for _, m := range disk.Mounts {
		if len(lines) >= h-3 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s %s  %.0f/%.0fG", m.Mount, pctStr(m.UsePct), m.UsedGB, m.SizeGB))
	}
	lines = append(lines, fmt.Sprintf("读 %.1f 写 %.1f MB/s", disk.ReadMBs, disk.WriteMBs))
	return panelNoTitle(strings.Join(lines, "\n"), w, h)
}

func netCard(snap *model.Snapshot, w, h int) string {
	net := snap.Host.Net
	lines := []string{stDim.Render("网络")}
	for _, i := range net.Ifaces {
		if len(lines) >= h-2 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s rx%.1f tx%.1f MB/s", i.Name, i.RxMBs, i.TxMBs))
	}
	if len(net.Ifaces) == 0 {
		lines = append(lines, stFaint.Render("—"))
	}
	return panelNoTitle(strings.Join(lines, "\n"), w, h)
}

// ---------- 模型 + 进程 ----------

func renderModelProc(snap *model.Snapshot, usable, h int) string {
	// lipgloss.JoinHorizontal 不插入间隙，两面板宽度之和精确等于 usable
	modelW := usable * 45 / 100
	procW := usable - modelW
	return lipgloss.JoinHorizontal(lipgloss.Top,
		panel("模型与 Slot", modelBody(snap), modelW, h),
		panel("进程", procBody(snap), procW, h))
}

func modelBody(snap *model.Snapshot) string {
	m := snap.Llama.Model
	var lines []string
	name := m.Name
	if name == "" {
		name = "—"
	}
	lines = append(lines, "模型 "+stCyan.Render(name))
	meta := []string{}
	if m.NParams != nil {
		meta = append(meta, fmt.Sprintf("参数 %.2fB", *m.NParams/1e9))
	}
	if m.Ftype != "" {
		meta = append(meta, "ftype "+m.Ftype)
	}
	if m.NCtx != nil {
		meta = append(meta, fmt.Sprintf("ctx %d", *m.NCtx))
	}
	if m.NEmbD != nil {
		meta = append(meta, fmt.Sprintf("embd %d", *m.NEmbD))
	}
	if len(meta) > 0 {
		lines = append(lines, strings.Join(meta, "  "))
	}
	aux := []string{}
	if m.FileSize != nil {
		aux = append(aux, "体积 "+fmtBytesShort(*m.FileSize))
	}
	if len(m.Modalities) > 0 {
		aux = append(aux, "模态 "+strings.Join(m.Modalities, ","))
	}
	if len(aux) > 0 {
		lines = append(lines, strings.Join(aux, "  "))
	}
	if m.MMProjPath != "" {
		line := "mmproj " + filepath.Base(m.MMProjPath)
		if m.MMProjSize != nil {
			line += "  " + fmtBytesShort(*m.MMProjSize)
		}
		lines = append(lines, line)
	}
	// Slots
	lines = append(lines, stDim.Render("Slots"))
	for _, s := range snap.Llama.Slots {
		state := s.State
		if state == "" {
			state = "idle"
		}
		line := fmt.Sprintf("  Slot%d: %s", s.ID, state)
		if s.IDTask != nil {
			line += fmt.Sprintf(" task#%d", *s.IDTask)
		}
		if s.NRemain != nil {
			line += fmt.Sprintf(" 剩%d", *s.NRemain)
		}
		lines = append(lines, line)
	}
	if len(snap.Llama.Slots) == 0 {
		lines = append(lines, stFaint.Render("  （无）"))
	}
	return strings.Join(lines, "\n")
}

func procBody(snap *model.Snapshot) string {
	var lines []string
	p := snap.Host.Process
	pname := "llama-server"
	if p != nil && p.Name != "" {
		pname = p.Name
	}
	if p == nil || !p.Found {
		lines = append(lines, stRed.Render(pname+" 进程未找到"))
	} else {
		lines = append(lines, fmt.Sprintf("%s  PID %d", stBold.Render(snap.Host.Service.Unit), p.PID))
		cpu := "—"
		if p.CPUPctRealtime != nil {
			cpu = pctStr(*p.CPUPctRealtime)
		}
		rss := "—"
		if p.RSSMB != nil {
			rss = fmt.Sprintf("%.1fGB", float64(*p.RSSMB)/1024)
		}
		threads := "—"
		if p.Threads != nil {
			threads = fmt.Sprintf("%d", *p.Threads)
		}
		lines = append(lines, fmt.Sprintf("CPU %s  内存 %s  线程 %s  运行 %s", cpu, rss, threads, orDash(p.Elapsed)))
		if s := snap.Host.Service; s.Active != "" {
			lines = append(lines, "服务 "+s.Active+"  "+orDash(s.Memory))
		}
	}
	// Top CPU
	lines = append(lines, stDim.Render("Top CPU"))
	lines = append(lines, topRows(snap.Host.TopCPU, "cpu"))
	lines = append(lines, stDim.Render("Top MEM"))
	lines = append(lines, topRows(snap.Host.TopMem, "mem"))
	return strings.Join(lines, "\n")
}

func topRows(rows []model.TopProc, mode string) string {
	if len(rows) == 0 {
		return stFaint.Render("  —")
	}
	var lines []string
	n := len(rows)
	if n > 5 {
		n = 5
	}
	for _, r := range rows[:n] {
		val := ""
		if mode == "cpu" {
			val = pctStr(r.CPUPct)
		} else {
			val = fmt.Sprintf("%dMB", r.RSSMB)
		}
		lines = append(lines, fmt.Sprintf("  %6d %-16s %s", r.PID, trunc(r.Name, 16), val))
	}
	return strings.Join(lines, "\n")
}

// ---------- 趋势 ----------

func renderTrends(snap *model.Snapshot, world *model.World, usable int) string {
	type item struct {
		label  string
		series string
	}
	items := []item{
		{"生成速度", "gen_speed"},
		{"预填充", "prompt_speed"},
		{"上下文", "ctx_used"},
		{"GPU0", "gpu_util_0"},
		{"CPU", "cpu"},
		{"内存", "mem_used"},
		{"网络", "net_rx"},
		{"负载", "load_1"},
	}
	n := len(items)
	gap := 2
	labelW := 8
	// 每段 = 标签(labelW) + 空格(1) + sparkline，段间 gap；总宽精确等于 usable（余数给最后一段）
	totalSpark := usable - (n-1)*gap - n*(labelW+1)
	sparkW := totalSpark / n
	if sparkW < 4 {
		sparkW = 4
	}
	var parts []string
	for i, it := range items {
		sw := sparkW
		if i == n-1 {
			sw = totalSpark - sparkW*(n-1)
			if sw < 4 {
				sw = 4
			}
		}
		vals := ringValues(world, it.series, sw)
		parts = append(parts, stDim.Render(padRight(it.label, labelW))+" "+sparkline(vals, sw))
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

// ---------- 工具 ----------

func ringValues(world *model.World, series string, w int) []*float64 {
	if world == nil {
		return nil
	}
	// llama 序列 60s 窗口，host 序列 120s 窗口
	window := 120.0
	if series == "gen_speed" || series == "prompt_speed" || series == "ctx_used" || series == "mtp_acceptance" {
		window = 60.0
	}
	return world.Sparkline(series, window)
}

func slotRemain(snap *model.Snapshot) *int {
	for _, s := range snap.Llama.Slots {
		if s.IsProcessing && s.NRemain != nil {
			return s.NRemain
		}
	}
	return nil
}

func taskIDStr(id *int) string {
	if id == nil {
		return ""
	}
	return "任务 #" + fmt.Sprintf("%d", *id)
}

func ctxPct(ctx model.Ctx) float64 {
	if ctx.Pct != nil {
		return *ctx.Pct
	}
	return 0
}

func valOr(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func fmtBytesShort(b float64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%.0f B", b)
	}
	div, exp := float64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", b/div, "KMGTPE"[exp])
}
