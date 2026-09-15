package model

import (
	"os"
	"strconv"
	"sync"
	"time"

	"llamalens-cli/internal/cfg"
)

// World 共享状态：采集器写入、UI 读取（单一 RWMutex 保护）。
// 采集器必须整体替换切片/结构（不得原地修改），保证 UI 读到一致快照。
type World struct {
	mu sync.RWMutex

	cfg   *cfg.Config
	llama LlamaAPI
	host  HostMetrics
	logst LogState

	events *EventDetector
	diff   *DiffEngine
	ringL  *RingBuffer // llama 序列 @1s
	ringH  *RingBuffer // host 序列 @2s

	snap *Snapshot

	prevTaskID *int
	fileSize   map[string]float64
}

func NewWorld(c *cfg.Config, events *EventDetector, diff *DiffEngine, ringL, ringH *RingBuffer) *World {
	return &World{
		cfg:      c,
		events:   events,
		diff:     diff,
		ringL:    ringL,
		ringH:    ringH,
		fileSize: make(map[string]float64),
	}
}

// SetLlama 由 llama 采集器调用。
func (w *World) SetLlama(api LlamaAPI) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.llama = api
}

// SetHost 由 host 采集器调用（整体替换主机指标）。
func (w *World) SetHost(h HostMetrics) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.host = h
}

// SetLogState 由 journal 采集器调用。
func (w *World) SetLogState(ls LogState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logst = ls
}

// PushHostRing 由 host 采集器调用（@2s 写 host 序列）。
func (w *World) PushHostRing(h HostMetrics) {
	now := float64(time.Now().Unix())
	push := func(name string, v *float64) { w.ringH.Push(name, now, v) }
	push("cpu", h.CPU.UsagePct)
	push("mem_used", f64(float64(h.Mem.UsedMB)))
	push("mem_buff_cache", f64(float64(h.Mem.BuffCacheMB)))
	push("swap_used", f64(float64(h.Mem.SwapUsedMB)))
	push("net_rx", f64(sumIface(h.Net.Ifaces, true)))
	push("net_tx", f64(sumIface(h.Net.Ifaces, false)))
	if h.Process != nil {
		for _, p := range h.Process {
			push("proc_cpu", p.CPUPctRealtime)
		}
	}
	for i, name := range []string{"load_1", "load_5", "load_15"} {
		v := h.CPU.Load[i]
		push(name, &v)
	}
	push("disk_read", f64(h.Disk.ReadMBs))
	push("disk_write", f64(h.Disk.WriteMBs))
	for _, g := range h.GPUs {
		idx := strconv.Itoa(g.Index)
		push("gpu_util_"+idx, f64(g.UtilPct))
		push("gpu_mem_"+idx, f64(float64(g.MemUsedMB)))
		push("gpu_temp_"+idx, g.TempC)
		push("gpu_power_"+idx, g.PowerW)
	}
}

// Tick 由 tick goroutine 调用（@1s）：速度优先级、写 llama 序列、构建快照、评估告警。
func (w *World) Tick() {
	now := time.Now().Unix()
	w.mu.Lock()
	defer w.mu.Unlock()

	gen, prompt, source := w.speeds()
	online := w.llama.Online

	if online {
		g := gen
		p := prompt
		w.ringL.Push("gen_speed", float64(now), &g)
		w.ringL.Push("prompt_speed", float64(now), &p)
	} else {
		w.ringL.Push("gen_speed", float64(now), nil)
		w.ringL.Push("prompt_speed", float64(now), nil)
	}
	ctxUsed := w.mergedCtxUsed()
	if online && ctxUsed != nil {
		w.ringL.Push("ctx_used", float64(now), f64(float64(*ctxUsed)))
	} else {
		w.ringL.Push("ctx_used", float64(now), nil)
	}

	// 任务结束事件型序列（mtp_acceptance / ctx_used 权威值）
	lt := w.logst.LastTask
	if lt != nil && lt.TaskID != nil && (w.prevTaskID == nil || *w.prevTaskID != *lt.TaskID) {
		w.prevTaskID = lt.TaskID
		if lt.MTP != nil && lt.MTP.Acceptance != nil {
			w.ringL.Push("mtp_acceptance", float64(now), lt.MTP.Acceptance)
		}
		w.ringL.Push("ctx_used", float64(now), f64(float64(lt.CtxUsed)))
	}

	snap := w.buildSnapshot(float64(now), gen, prompt, source)
	snap.Alerts = EvaluateAlerts(snap.Llama, snap.Host, snap.Log)
	w.events.CheckAlerts(snap.Alerts)
	w.snap = &snap
}

// Snapshot 返回最近一次 tick 构建的快照。
func (w *World) Snapshot() *Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.snap
}

// Sparkline 返回某序列最近 windowS 秒的值（用于 UI sparkline/趋势）。
func (w *World) Sparkline(name string, windowS float64) []*float64 {
	now := float64(time.Now().Unix())
	w.mu.RLock()
	defer w.mu.RUnlock()
	var pts []pt
	if name == "gen_speed" || name == "prompt_speed" || name == "ctx_used" || name == "mtp_acceptance" {
		pts = w.ringL.Window(name, windowS, now)
	} else {
		pts = w.ringH.Window(name, windowS, now)
	}
	out := make([]*float64, 0, len(pts))
	for _, p := range pts {
		out = append(out, p.v)
	}
	return out
}

// speeds 速度来源优先级：日志 tg_3s / prompt 行 > /slots 差分。
func (w *World) speeds() (gen, prompt float64, source string) {
	gen = w.llama.GenTPS
	prompt = w.llama.PromptTPS
	st := w.logst
	fromLog := false
	if st.Available {
		if st.Phase == "decoding" && st.Tg3sTPS != nil {
			gen = *st.Tg3sTPS
			fromLog = true
		}
		if st.Phase == "prompt_processing" && st.PromptSpeedTPS != nil {
			prompt = *st.PromptSpeedTPS
			fromLog = true
		}
	}
	if fromLog {
		source = "log"
	} else {
		source = "api"
	}
	return
}

func (w *World) mergedCtxUsed() *int {
	if w.llama.CtxUsed != nil {
		return w.llama.CtxUsed
	}
	return w.logst.Context.Used
}

func (w *World) buildSnapshot(now, gen, prompt float64, source string) Snapshot {
	_ = gen
	_ = prompt
	// 模型合并：/props + /v1/models + 文件体积
	model := w.llama.Model
	if model.Path != "" {
		if sz, ok := w.fileSize[model.Path]; ok {
			model.FileSize = &sz
		} else if fi, err := os.Stat(model.Path); err == nil {
			sz := float64(fi.Size())
			w.fileSize[model.Path] = sz
			model.FileSize = &sz
		}
	}
	// mmproj 来自进程命令行
	if w.host.Process != nil {
		for _, p := range w.host.Process {
			if mp, ok := p.Flags["mmproj"]; ok {
				model.MMProjPath = mp
				if sz, ok2 := w.fileSize[mp]; ok2 {
					model.MMProjSize = &sz
				} else if fi, err := os.Stat(mp); err == nil {
					sz := float64(fi.Size())
					w.fileSize[mp] = sz
					model.MMProjSize = &sz
				}
				break // 取第一个找到的 mmproj
			}
		}
	}

	// 上下文：API 实时值（slot）优先，日志（任务结束行）兜底
	ctx := w.logst.Context
	if w.llama.CtxTotal > 0 {
		t := w.llama.CtxTotal
		ctx.Total = &t
	}
	if w.llama.CtxUsed != nil {
		ctx.Used = w.llama.CtxUsed
	}
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
	logst := w.logst
	logst.Context = ctx

	// model 是 w.llama.Model 的合并副本（体积/mmproj），必须用它返回，
	// 否则合并结果被丢弃（旧版直接返回 w.llama，体积/mmproj 行从未显示）
	llama := w.llama
	llama.Model = model

	return Snapshot{
		TS:          now,
		HostName:    w.cfg.Name,
		Llama:       llama,
		Log:         logst,
		Host:        w.host,
		SpeedSource: source,
		Events:      w.events.List(50),
	}
}

// ---------- 小工具 ----------

func f64(v float64) *float64 { return &v }

func sumIface(ifaces []Iface, rx bool) float64 {
	var s float64
	for _, i := range ifaces {
		if rx {
			s += i.RxMBs
		} else {
			s += i.TxMBs
		}
	}
	return s
}
