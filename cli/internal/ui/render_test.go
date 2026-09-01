package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"llamalens-cli/internal/cfg"
	"llamalens-cli/internal/model"
)

func pf(v float64) *float64 { return &v }
func pi(v int) *int         { return &v }

// stripANSI 去掉 ANSI 转义序列，便于纯文本检查布局。
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			i = ansiSeqEnd(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func syntheticWorld() *model.World {
	c := cfg.Default()
	c.Name = "ai"
	w := model.NewWorld(c, model.NewEventDetector(200), model.NewDiffEngine(), model.NewRingBuffer(3600), model.NewRingBuffer(1800))

	w.SetLlama(model.LlamaAPI{
		Online:   true,
		Model:    model.ModelInfo{Name: "Qwen3.8-27B-Q6_K", Ftype: "Q6_K", NParams: pf(27e9), NCtx: pi(262144), NEmbD: pi(3584), FileSize: pf(17.2e9), Modalities: []string{"text", "image"}},
		Slots:    []model.Slot{{ID: 0, IsProcessing: true, State: "processing", IDTask: pi(7), NRemain: pi(1234), GenTPS: 38.5}},
		CtxUsed:  pi(18621),
		CtxTotal: 262144,
		GenTPS:   38.5,
	})

	w.SetLogState(model.LogState{
		Available:   true,
		Phase:       "decoding",
		TaskID:      pi(7),
		NDecoded:    512,
		TgTPS:       pf(38.2),
		Tg3sTPS:     pf(39.1),
		Context:     model.Ctx{Used: pi(18621), Total: pi(262144)},
		MTP:         model.MTP{Acceptance: pf(0.82), Accepted: pi(41), Generated: pi(50)},
		LastPrefill: &model.PrefillInfo{Speed: pf(1042), TS: 1234567890},
		LastTask:    &model.TaskSummary{TaskID: pi(6), TotalTokens: pi(2048), GenSpeedTPS: pf(41.2), PromptSpeedTPS: pf(980)},
	})

	temp, power, powerLim, fan := pf(72.0), pf(345.0), pf(450.0), pf(62.0)
	clock, pcieGen, pcieW := pi(2520), pi(4), pi(16)
	eccC, eccU := pi(0), pi(0)
	w.SetHost(model.HostMetrics{
		Reachable: true,
		CPU: model.CPUInfo{
			UsagePct:   pf(45.2),
			PerCorePct: []float64{12, 34, 56, 78, 23, 45, 67, 89},
			Load:       [3]float64{3.21, 2.87, 2.45},
		},
		Mem: model.MemInfo{TotalMB: 65536, UsedMB: 41984, FreeMB: 23552, SwapTotalMB: 8192, SwapUsedMB: 0},
		Disk: model.DiskInfo{
			Mounts:  []model.Mount{{Mount: "/", SizeGB: 476, UsedGB: 289, UsePct: 63}, {Mount: "/share", SizeGB: 1863, UsedGB: 1521, UsePct: 82}},
			ReadMBs: 12.5, WriteMBs: 3.2,
		},
		Net:     model.NetInfo{Ifaces: []model.Iface{{Name: "eth0", RxMBs: 1.2, TxMBs: 0.4}}},
		Process: &model.ProcInfo{Found: true, PID: 1693, CPUPctRealtime: pf(120.5), RSSMB: pi(18432), Threads: pi(13), Elapsed: "2h 15m"},
		Service: model.ServiceInfo{Unit: "llama-server.service", Active: "active (running)", Memory: "18.0G"},
		GPUs: []model.GPU{
			{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemTotalMB: 20480, MemUsedMB: 18621, MemFreeMB: 1859, UtilPct: 67, TempC: temp, PowerW: power, PowerLimitW: powerLim, FanPct: fan, ClockMHz: clock, PCIEGen: pcieGen, PCIEWidth: pcieW, PState: "P0", ECCCorrected: eccC, ECCUncorrected: eccU,
				Apps: []model.GPUApp{{PID: 1693, Name: "llama-server", MemMB: 18456}}},
			{Index: 1, Name: "NVIDIA GeForce RTX 4090", MemTotalMB: 20480, MemUsedMB: 17965, MemFreeMB: 2515, UtilPct: 12, TempC: temp, PowerW: power, PowerLimitW: powerLim, FanPct: fan, ClockMHz: clock, PCIEGen: pcieGen, PCIEWidth: pcieW, PState: "P0", ECCCorrected: eccC, ECCUncorrected: eccU,
				Apps: []model.GPUApp{{PID: 1693, Name: "llama-server", MemMB: 17809}}},
		},
		TopCPU: []model.TopProc{{PID: 1693, Name: "llama-server", CPUPct: 120.5}, {PID: 42, Name: "kworker/0:1", CPUPct: 12.0}},
		TopMem: []model.TopProc{{PID: 1693, Name: "llama-server", RSSMB: 18432}, {PID: 42, Name: "systemd", RSSMB: 1200}},
	})

	for i := 0; i < 60; i++ {
		w.PushHostRing(model.HostMetrics{
			CPU: model.CPUInfo{UsagePct: pf(30 + float64(i%20)), Load: [3]float64{3, 2, 1}},
			Mem: model.MemInfo{UsedMB: 40000 + i*10},
			Net: model.NetInfo{Ifaces: []model.Iface{{Name: "eth0", RxMBs: 1 + float64(i%5), TxMBs: 0.5}}},
			Disk: model.DiskInfo{ReadMBs: 10, WriteMBs: 2},
			GPUs: []model.GPU{{Index: 0, UtilPct: 50 + float64(i%40)}},
		})
	}
	w.Tick()
	return w
}

func TestRenderSynthetic(t *testing.T) {
	w := syntheticWorld()
	snap := w.Snapshot()
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	for _, size := range [][2]int{{120, 45}, {120, 35}, {100, 30}, {80, 24}} {
		out := Render(snap, w, size[0], size[1], options{showTrends: true, showGPU: true})
		t.Logf("=== %dx%d ===\n%s", size[0], size[1], stripANSI(out))
	}
}

func TestTaskBodyStatus(t *testing.T) {
	// 任务卡状态以 /slots is_processing 为准；日志 phase 仅细化标签。
	cases := []struct {
		name    string
		slots   []model.Slot
		log     model.LogState
		want    string
		notWant string
	}{
		{"API 生成中+日志解码→解码中", []model.Slot{{ID: 0, IsProcessing: true, IDTask: pi(9)}}, model.LogState{Phase: "decoding"}, "解码中", ""},
		{"API 生成中+日志预填充→预填充", []model.Slot{{ID: 0, IsProcessing: true, IDTask: pi(9)}}, model.LogState{Phase: "prompt_processing"}, "预填充", ""},
		{"API 生成中+日志空闲(journal 滞后)→生成中", []model.Slot{{ID: 0, IsProcessing: true, IDTask: pi(9)}}, model.LogState{Phase: "idle"}, "生成中", "空闲 · 等待新任务"},
		{"API 空闲+日志空闲→空闲", []model.Slot{{ID: 0}}, model.LogState{Phase: "idle"}, "空闲 · 等待新任务", "生成中"},
		{"API 空闲+日志解码残留→空闲", []model.Slot{{ID: 0}}, model.LogState{Phase: "decoding"}, "空闲 · 等待新任务", "解码中"},
		{"API 生成中+日志残留旧任务 ID→以 API 为准", []model.Slot{{ID: 0, IsProcessing: true, IDTask: pi(9)}}, model.LogState{Phase: "idle", TaskID: pi(5)}, "任务 #9", "任务 #5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := model.NewWorld(cfg.Default(), model.NewEventDetector(200), model.NewDiffEngine(), model.NewRingBuffer(3600), model.NewRingBuffer(1800))
			w.SetLlama(model.LlamaAPI{Online: true, Slots: tc.slots})
			w.SetLogState(tc.log)
			w.Tick()
			snap := w.Snapshot()
			if snap == nil {
				t.Fatal("snapshot is nil")
			}
			out := stripANSI(taskBody(snap))
			if !strings.Contains(out, tc.want) {
				t.Fatalf("应包含 %q，实际:\n%s", tc.want, out)
			}
			if tc.notWant != "" && strings.Contains(out, tc.notWant) {
				t.Fatalf("不应包含 %q，实际:\n%s", tc.notWant, out)
			}
		})
	}
}

func TestAppDumpFrame(t *testing.T) {
	w := syntheticWorld()
	f := filepath.Join(t.TempDir(), "frame.txt")
	a := NewApp(w, f)
	a.width, a.height = 120, 45
	view := a.View()
	if view == "" {
		t.Fatal("view 为空")
	}
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("dump 文件读取失败: %v", err)
	}
	if string(data) != view {
		t.Fatal("dump 文件内容与 View() 输出不一致")
	}
}

func TestNoColorRender(t *testing.T) {
	w := syntheticWorld()
	a := NewApp(w, "")
	a.width, a.height = 120, 45
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(termenv.TrueColor)
	view := a.View()
	if view == "" {
		t.Fatal("view 为空")
	}
	if i := strings.IndexByte(view, 0x1b); i >= 0 {
		t.Fatalf("ASCII 颜色配置下渲染仍含 ANSI 转义（偏移 %d）", i)
	}
}

// TestThemeColorsVisibleUnderANSI256 回归：前景文字色在 ANSI256 终端下不得被
// termenv 映射到近黑灰阶（232-238），否则深色终端上不可见。
// 历史 bug：colRed #e06c75 被映射到 232(#080808)，导致所有红色文字（离线/截断/
// danger 事件/百分比飘红）在 ANSI256 终端上消失。
func TestThemeColorsVisibleUnderANSI256(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.TrueColor)

	fgStyles := map[string]lipgloss.Style{
		"text":  stText,
		"cyan":  stCyan,
		"green": stGreen,
		"amber": stAmber,
		"red":   stRed,
		"dim":   stDim,
	}
	for name, st := range fgStyles {
		idx := ansi256Code(st.Render("X"))
		if idx < 0 {
			t.Fatalf("%s: ANSI256 渲染未产生 38;5;N 色码", name)
		}
		if idx >= 232 && idx <= 238 {
			t.Errorf("%s: 颜色被映射到近黑灰阶 %d（深色终端上不可见）", name, idx)
		}
	}
}

// ansi256Code 提取渲染输出中的 38;5;N 色码，无则返回 -1。
func ansi256Code(s string) int {
	i := strings.Index(s, "38;5;")
	if i < 0 {
		return -1
	}
	j := i + len("38;5;")
	n := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		n = n*10 + int(s[j]-'0')
		j++
	}
	return n
}

func TestTopBarModelNameBasename(t *testing.T) {
	// 定制 build 的 model_alias 是完整路径：TopBar 只显示文件名（与 Web TopBar 一致）
	w := model.NewWorld(cfg.Default(), model.NewEventDetector(200), model.NewDiffEngine(), model.NewRingBuffer(3600), model.NewRingBuffer(1800))
	w.SetLlama(model.LlamaAPI{Online: true, Model: model.ModelInfo{Name: "/share/AI/LLM/unsloth/Qwen3.8-27B-Q6_K.gguf"}})
	w.Tick()
	snap := w.Snapshot()
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	out := stripANSI(renderTopBar(snap, 120, options{}))
	if !strings.Contains(out, "Qwen3.8-27B-Q6_K.gguf") {
		t.Fatalf("TopBar 应含模型文件名: %s", out)
	}
	if strings.Contains(out, "/share/AI") {
		t.Fatalf("TopBar 不应含路径: %s", out)
	}
}
