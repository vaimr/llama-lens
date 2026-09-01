// Package logparse 解析 llama-server 日志行，维护"llama 当前在干嘛"状态机。
// 正则与状态迁移逻辑与面板 backend/pollers/log_poller.py 完全一致。
package logparse

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"llamalens-cli/internal/model"
)

// 行前缀：2026-08-28T18:11:25+08:00 ai llama-server[55646]: 50.44.267.087 I slot release: ...
var rePrefix = regexp.MustCompile(`^(\S+)\s+(\S+)\s+([\w.-]+)\[(\d+)\]:\s*(.*)$`)
var reLevel = regexp.MustCompile(`^\S+\s+([IWE])\s+\w+\s+\w+:\s*`)

// 11 类解析规则（按序匹配，见架构文档 §4.4）
var (
	reDecode = regexp.MustCompile(
		`slot\s+print_timing:\s+id\s+(\d+)\s*\|\s*task\s+(\d+)\s*\|\s*` +
			`n_decoded\s*=\s*(\d+),\s*tg\s*=\s*([\d.]+)\s*t/s,\s*tg_3s\s*=\s*([\d.]+)\s*t/s`)
	rePrompt = regexp.MustCompile(
		`slot\s+print_timing:\s+id\s+(\d+)\s*\|\s*task\s+(\d+)\s*\|\s*` +
			`prompt processing,\s*n_tokens\s*=\s*(\d+),\s*progress\s*=\s*([\d.]+),\s*` +
			`t\s*=\s*([\d.]+)\s*s\s*/\s*([\d.]+)\s*tokens per second`)
	rePromptSummary = regexp.MustCompile(
		`prompt eval time\s*=\s*([\d.]+)\s*ms\s*/\s*(\d+)\s*tokens\s*` +
			`\(\s*([\d.]+)\s*ms per token,\s*([\d.]+)\s*tokens per second\)`)
	// 注意：Go RE2 不支持 lookbehind。Python 原式有 (?<!prompt ) 前缀断言，
	// 但 rePromptSummary 在本规则之前匹配并 return，"prompt eval time" 行已被消费，
	// 故此处去掉 lookbehind 行为等价。
	reEvalSummary = regexp.MustCompile(
		`eval time\s*=\s*([\d.]+)\s*ms\s*/\s*(\d+)\s*tokens\s*` +
			`\(\s*([\d.]+)\s*ms per token,\s*([\d.]+)\s*tokens per second\)`)
	reTotalSummary = regexp.MustCompile(`total time\s*=\s*([\d.]+)\s*ms\s*/\s*(\d+)\s*tokens`)
	reGraphs     = regexp.MustCompile(`graphs reused\s*=\s*(\d+)`)
	reMTP = regexp.MustCompile(
		`draft acceptance\s*=\s*([\d.]+)\s*\(\s*(\d+) accepted\s*/\s*(\d+) generated\),\s*` +
			`mean len\s*=\s*([\d.]+)`)
	reRelease = regexp.MustCompile(
		`slot\s+release:\s+id\s+(\d+)\s*\|\s*task\s+(\d+)\s*\|\s*` +
			`stop processing:\s*n_tokens\s*=\s*(\d+),\s*truncated\s*=\s*(\d+)`)
	reSlotSelect = regexp.MustCompile(
		`slot\s+get_availabl:\s+id\s+(\d+)\s*\|\s*task\s+-1\s*\|\s*` +
			`selected slot by\s+(LRU|LCP similarity)` +
			`(?:,\s*f_sim_best\s*=\s*([\d.]+)\s*\(>\s*[\d.]+\s+thold\),\s*f_keep\s*=\s*([\d.]+))?`)
	reLaunch = regexp.MustCompile(
		`slot\s+launch_slot_:\s+id\s+(\d+)\s*\|\s*task\s+(\d+)\s*\|\s*` +
			`processing task,\s*is_child\s*=\s*(\d+)`)
)

// 启动行
var (
	reBootModel  = regexp.MustCompile(`loading model\s+'(.+?)'`)
	reBootSlots  = regexp.MustCompile(`n_slots\s*=\s*(\d+),\s*n_ctx_slot\s*=\s*(\d+),\s*kv_unified\s*=\s*'(\w+)'`)
	reBootMTP    = regexp.MustCompile(`creating MTP draft context`)
	reBootKVUp   = regexp.MustCompile(`upgrading K from\s+(\S+)\s+to\s+(\S+)`)
	reBootVerb   = regexp.MustCompile(`verbosity\s*=\s*(\d+)`)
	reBootListen = regexp.MustCompile(`listening on\s+(\S+)`)
)

type summaryAcc struct {
	promptMS       *float64
	promptTokens   *int
	promptSpeedTPS *float64
	evalMS         *float64
	decodedTokens  *int
	genSpeedTPS    *float64
	totalMS        *float64
	totalTokens    *int
	graphsReused   *int
	mtp            *model.MTP
}

// Poller 日志状态机（线程安全）。
type Poller struct {
	mu sync.Mutex

	st model.LogState

	pid          *int
	pidCandidate *int
	bootPids     map[int]bool
	lastLineTS   float64
	streamOpenTS *float64
	bootSeen     bool
	summary      summaryAcc
	prefillTask  *int

	events *model.EventDetector
	source string // journal | file

	pendingBoot *int // PID 变化后待补拉 boot 块的 PID
}

func NewPoller(events *model.EventDetector, source string) *Poller {
	return &Poller{
		bootPids: make(map[int]bool),
		events:   events,
		source:   source,
	}
}

func emptyState() model.LogState {
	return model.LogState{
		Phase:   "idle",
		Context: model.Ctx{Truncated: false},
	}
}

// SetStreamOpen 标记流打开时刻（用于补拉行判定）。
func (p *Poller) SetStreamOpen() {
	now := float64(time.Now().Unix())
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamOpenTS = &now
}

// BootNeeded 返回待补拉 boot 块的 PID（无则 nil）。
func (p *Poller) BootNeeded() *int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pendingBoot
}

// MarkBootFetched 标记某 PID 的 boot 块已补拉。
func (p *Poller) MarkBootFetched(pid int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bootPids[pid] = true
	if p.pendingBoot != nil && *p.pendingBoot == pid {
		p.pendingBoot = nil
	}
}

// SetAvailable 标记日志源是否可用。
func (p *Poller) SetAvailable(avail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.st.Available = avail
}

// State 返回当前状态快照（副本）。
func (p *Poller) State() model.LogState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st
}

// HandleLine 处理一行日志（journal 前缀行或裸行）。
func (p *Poller) HandleLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	msg := line
	var syslogTS string
	var unit string
	var pid int
	var hasPrefix bool
	if m := rePrefix.FindStringSubmatch(line); m != nil {
		syslogTS = m[1]
		unit = m[3]
		pid, _ = strconv.Atoi(m[4])
		msg = m[5]
		hasPrefix = true
		if unit == "systemd" {
			return // systemd 生命周期行，非服务端输出
		}
	}

	if hasPrefix {
		if !p.isCatchup(syslogTS) {
			if p.pid != nil && *p.pid != pid {
				if p.pidCandidate != nil && *p.pidCandidate == pid {
					p.pid = &pid
					p.pidCandidate = nil
					p.resetMachine()
					if !p.bootPids[pid] {
						pb := pid
						p.pendingBoot = &pb
					}
				} else {
					p.pidCandidate = &pid
				}
			} else if p.pid != nil {
				p.pidCandidate = nil
			} else {
				p.pid = &pid
			}
		}
		p.lastLineTS = float64(time.Now().Unix())
	}

	// file 模式：行内无 PID，用 "loading model" 启动行作为重启标记
	if p.source == "file" {
		if reBootModel.MatchString(msg) {
			if p.bootSeen {
				p.resetMachine()
			}
			p.bootSeen = true
		}
	}

	p.parse(msg)
}

func (p *Poller) isCatchup(syslogTS string) bool {
	if p.streamOpenTS == nil {
		return false
	}
	t, err := time.Parse("2006-01-02T15:04:05-07:00", syslogTS)
	if err != nil {
		return false
	}
	return float64(t.Unix()) < *p.streamOpenTS-1
}

func (p *Poller) resetMachine() {
	avail := p.st.Available
	p.st = emptyState()
	p.st.Available = avail
	p.summary = summaryAcc{}
	p.prefillTask = nil
	p.events.ResetTaskState()
}

// ParseBootLine 只匹配启动行（boot 块补拉用，不走 PID 变化检测）。
func (p *Poller) ParseBootLine(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := rePrefix.FindStringSubmatch(line)
	msg := line
	if m != nil {
		msg = m[5]
	}
	p.parseBootLine(msg)
}

func (p *Poller) parse(msg string) {
	st := &p.st

	if m := reDecode.FindStringSubmatch(msg); m != nil {
		st.Phase = "decoding"
		tid := atoi(m[2])
		st.TaskID = &tid
		st.NDecoded = atoi(m[3])
		tg := atof(m[4])
		tg3 := atof(m[5])
		st.TgTPS = &tg
		st.Tg3sTPS = &tg3
		return
	}
	if m := rePrompt.FindStringSubmatch(msg); m != nil {
		st.Phase = "prompt_processing"
		tid := atoi(m[2])
		st.TaskID = &tid
		prog := atof(m[4])
		speed := atof(m[6])
		tok := atoi(m[3])
		elapsed := atof(m[5])
		st.PromptProgress = &prog
		st.PromptSpeedTPS = &speed
		st.PromptTotalTokens = &tok
		st.PromptElapsedS = &elapsed
		now := time.Now().Unix()
		p.st.LastPrefill = &model.PrefillInfo{Speed: &speed, TS: float64(now), Progress: &prog, NTokens: &tok}
		if p.prefillTask == nil || *p.prefillTask != tid {
			p.prefillTask = &tid
			ev := "任务 #" + itoa(tid) + " 预填充开始"
			if tok > 0 {
				ev += " (prompt " + itoa(tok) + " tokens)"
			}
			p.events.Emit(float64(now), "info", "prefill_start", ev)
		}
		return
	}
	if m := reRelease.FindStringSubmatch(msg); m != nil {
		p.onTaskEnd(atoi(m[2]), atoi(m[3]), atoi(m[4]) == 1)
		return
	}
	if m := reLaunch.FindStringSubmatch(msg); m != nil {
		tid := atoi(m[2])
		child := atoi(m[3])
		now := float64(time.Now().Unix())
		st.TaskID = &tid
		st.IsChild = &child
		st.StartedAt = &now
		p.summary = summaryAcc{}
		promptTok := st.PromptTotalTokens
		p.events.SetTaskRunning(float64(now), true, &tid, promptTok)
		return
	}
	if m := reSlotSelect.FindStringSubmatch(msg); m != nil {
		kv := &p.st.KV
		kv.Selection = m[2]
		if m[3] != "" {
			v := atof(m[3])
			kv.FSimBest = &v
		}
		if m[4] != "" {
			v := atof(m[4])
			kv.FKeep = &v
		}
		return
	}

	// 任务结束汇总（5 类，可乱序到达，先累积后随 release 落盘）
	if m := rePromptSummary.FindStringSubmatch(msg); m != nil {
		p.summary.promptMS = ptr(atof(m[1]))
		p.summary.promptTokens = ptr(atoi(m[2]))
		p.summary.promptSpeedTPS = ptr(atof(m[4]))
		return
	}
	if m := reEvalSummary.FindStringSubmatch(msg); m != nil {
		p.summary.evalMS = ptr(atof(m[1]))
		p.summary.decodedTokens = ptr(atoi(m[2]))
		p.summary.genSpeedTPS = ptr(atof(m[4]))
		return
	}
	if m := reTotalSummary.FindStringSubmatch(msg); m != nil {
		p.summary.totalMS = ptr(atof(m[1]))
		p.summary.totalTokens = ptr(atoi(m[2]))
		return
	}
	if m := reGraphs.FindStringSubmatch(msg); m != nil {
		g := atoi(m[1])
		p.summary.graphsReused = &g
		p.st.GraphsReused = &g
		return
	}
	if m := reMTP.FindStringSubmatch(msg); m != nil {
		p.summary.mtp = &model.MTP{
			Acceptance: ptr(atof(m[1])),
			Accepted:   ptr(atoi(m[2])),
			Generated:  ptr(atoi(m[3])),
			MeanLen:    ptr(atof(m[4])),
		}
		return
	}

	if p.parseBootLine(msg) {
		return
	}

	// W 级行 → 警告列表（去重，最多 10 条）
	if lm := reLevel.FindStringSubmatch(msg); lm != nil && lm[1] == "W" {
		text := strings.TrimSpace(msg)
		warns := p.st.Boot.Warnings
		exists := false
		for _, w := range warns {
			if w == text {
				exists = true
				break
			}
		}
		if !exists && len(warns) < 10 {
			p.st.Boot.Warnings = append(warns, text)
		}
	}
}

func (p *Poller) parseBootLine(msg string) bool {
	if m := reBootModel.FindStringSubmatch(msg); m != nil {
		p.st.Boot.Model = m[1]
		return true
	}
	if m := reBootSlots.FindStringSubmatch(msg); m != nil {
		p.st.Boot.NSlots = ptr(atoi(m[1]))
		p.st.Boot.NCtxSlot = ptr(atoi(m[2]))
		p.st.Boot.KVUnified = m[3] == "true"
		p.setCtxTotal(atoi(m[2]))
		return true
	}
	if reBootMTP.MatchString(msg) {
		p.st.Boot.MTPDraft = true
		return true
	}
	if m := reBootKVUp.FindStringSubmatch(msg); m != nil {
		p.st.Boot.KVCacheUpgrade = m[1] + " -> " + m[2]
		return true
	}
	if m := reBootVerb.FindStringSubmatch(msg); m != nil {
		p.st.Boot.Verbosity = ptr(atoi(m[1]))
		return true
	}
	if m := reBootListen.FindStringSubmatch(msg); m != nil {
		p.st.Boot.Listening = m[1]
		return true
	}
	return false
}

func (p *Poller) setCtxTotal(total int) {
	if total > 0 {
		t := total
		p.st.Context.Total = &t
		p.refreshCtx()
	}
}

func (p *Poller) refreshCtx() {
	ctx := &p.st.Context
	if ctx.Used != nil && ctx.Total != nil && *ctx.Total > 0 {
		pct := float64(*ctx.Used) / float64(*ctx.Total) * 100.0
		pct = float64(int(pct*10+0.5)) / 10
		rem := *ctx.Total - *ctx.Used
		if rem < 0 {
			rem = 0
		}
		ctx.Pct = &pct
		ctx.Remaining = &rem
	}
}

func (p *Poller) onTaskEnd(taskID, nTokens int, truncated bool) {
	now := time.Now().Unix()
	ctx := &p.st.Context
	t := nTokens
	ctx.Used = &t
	ctx.Truncated = truncated
	p.refreshCtx()

	mtp := p.summary.mtp
	if mtp != nil {
		p.st.MTP = *mtp
	}

	lastTask := &model.TaskSummary{
		TaskID:         &taskID,
		PromptMS:       p.summary.promptMS,
		PromptTokens:   p.summary.promptTokens,
		PromptSpeedTPS: p.summary.promptSpeedTPS,
		EvalMS:         p.summary.evalMS,
		DecodedTokens:  p.summary.decodedTokens,
		GenSpeedTPS:    p.summary.genSpeedTPS,
		TotalMS:        p.summary.totalMS,
		TotalTokens:    p.summary.totalTokens,
		GraphsReused:   p.summary.graphsReused,
		MTP:            mtp,
		CtxUsed:        nTokens,
		Truncated:      truncated,
	}
	p.st.LastTask = lastTask
	p.summary = summaryAcc{}

	st := &p.st
	st.Phase = "idle"
	st.NDecoded = 0
	st.TgTPS = nil
	st.Tg3sTPS = nil
	st.PromptProgress = nil
	st.PromptSpeedTPS = nil
	st.PromptTotalTokens = nil
	st.PromptElapsedS = nil
	st.StartedAt = nil
	p.prefillTask = nil

	totalTokens := lastTask.TotalTokens
	if totalTokens == nil {
		totalTokens = lastTask.DecodedTokens
	}
	var durationS *float64
	if lastTask.TotalMS != nil {
		d := *lastTask.TotalMS / 1000.0
		durationS = &d
	}
	var mtpAcc *float64
	if mtp != nil {
		mtpAcc = mtp.Acceptance
	}
	note := ""
	if lastTask.TotalMS == nil && lastTask.EvalMS == nil {
		note = "已中断"
	}
	p.events.TaskEndWithStats(float64(now), &taskID, totalTokens, durationS,
		lastTask.GenSpeedTPS, mtpAcc, &nTokens, note)
}

// ---------- 小工具 ----------

func ptr[T any](v T) *T { return &v }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func itoa(n int) string { return strconv.Itoa(n) }
