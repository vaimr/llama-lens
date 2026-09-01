package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"llamalens-cli/internal/cfg"
	"llamalens-cli/internal/model"
)

const failOffline = 3 // 连续失败次数 → 判定离线

// LlamaCollector 轮询本机 llama-server HTTP API（/slots @1s，/props+/v1/models @30s）。
type LlamaCollector struct {
	cfg    *cfg.Config
	world  *model.World
	events *model.EventDetector
	client *http.Client

	mu            sync.Mutex
	slots         map[int]*slotState
	failCount     int
	online        bool
	model         model.ModelInfo
	modelPath     *string
	lastSlow      time.Time
}

type slotState struct {
	prevTaskID        *int
	prevDecoded       int
	prevPromptProc    int
	prevTS            *float64
	taskStartTS       *float64
	genSpeed          float64
	promptSpeed       float64
}

func NewLlamaCollector(c *cfg.Config, w *model.World, ev *model.EventDetector) *LlamaCollector {
	return &LlamaCollector{
		cfg:     c,
		world:   w,
		events:  ev,
		client:  &http.Client{Timeout: time.Duration(c.LlamaTimeout * float64(time.Second))},
		slots:   make(map[int]*slotState),
	}
}

// Run 启动采集循环（阻塞，调用方放 goroutine）。
func (l *LlamaCollector) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(l.cfg.LlamaInterval * float64(time.Second)))
	defer ticker.Stop()
	l.pollFast(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.pollFast(ctx)
		}
	}
}

func (l *LlamaCollector) getJSON(path string, v interface{}) error {
	url := l.cfg.LlamaURL() + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func (l *LlamaCollector) pollFast(ctx context.Context) {
	now := time.Now().Unix()
	var raw json.RawMessage
	if err := l.getJSON("/slots", &raw); err != nil {
		l.onFail(now)
		return
	}
	// 新版 llama.cpp 返回裸数组，旧版返回 {"slots": [...]}（纯内存解析，无需锁）
	var slotsJSON []map[string]interface{}
	var wrap struct {
		Slots []map[string]interface{} `json:"slots"`
	}
	if err := json.Unmarshal(raw, &slotsJSON); err != nil {
		if err2 := json.Unmarshal(raw, &wrap); err2 != nil {
			return
		}
		slotsJSON = wrap.Slots
	}
	// 慢轮询：先拉 /props+/v1/models 更新模型，再应用 slots，确保首帧即有模型名
	//（pollSlow 自持锁；lastSlow 仅本协程访问）
	if time.Since(l.lastSlow) >= time.Duration(l.cfg.LlamaSlowInterval*float64(time.Second)) {
		l.lastSlow = time.Now()
		l.pollSlow(ctx)
	}
	l.mu.Lock()
	l.failCount = 0
	if !l.online {
		l.online = true
		l.mu.Unlock()
		l.events.SetLlamaOnline(float64(now), true, l.modelName())
		l.mu.Lock()
	}
	l.applySlots(slotsJSON, float64(now))
	l.mu.Unlock()
}

func (l *LlamaCollector) onFail(now int64) {
	l.mu.Lock()
	l.failCount++
	if l.online && l.failCount >= failOffline {
		l.online = false
		l.mu.Unlock()
		l.events.SetLlamaOnline(float64(now), false, "")
	} else {
		l.mu.Unlock()
	}
	// 离线时也推送当前（离线）状态
	l.push()
}

func (l *LlamaCollector) modelName() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.model.Name
}

func (l *LlamaCollector) applySlots(slots []map[string]interface{}, now float64) {
	out := make([]model.Slot, 0, len(slots))
	totalGen, totalPrompt := 0.0, 0.0
	aggUsed, aggTotal := 0, 0
	hasUsed := false
	for _, s := range slots {
		id := int64ToFloat(s["id"])
		slotID := int(id)
		st := l.slots[slotID]
		if st == nil {
			st = &slotState{}
			l.slots[slotID] = st
		}
		slot := model.Slot{
			ID:                     slotID,
			IsProcessing:           boolVal(s["is_processing"]),
			State:                  strVal(s["state"]),
			NPromptTokens:          intVal(s["n_prompt_tokens"]),
			NPromptTokensProcessed: intVal(s["n_prompt_tokens_processed"]),
		}
		if v, ok := s["id_task"]; ok && v != nil {
			t := int(int64ToFloat(v))
			slot.IDTask = &t
		}
		if nd := slotDecoded(s); nd != nil {
			slot.NDecoded = nd
		}
		if v, ok := s["n_remain"]; ok && v != nil {
			t := int(int64ToFloat(v))
			slot.NRemain = &t
		}
		l.diffSlot(st, slot, now)
		slot.GenTPS = st.genSpeed
		slot.PromptTPS = st.promptSpeed
		out = append(out, slot)
		totalGen = maxf(totalGen, st.genSpeed)
		totalPrompt = maxf(totalPrompt, st.promptSpeed)
		// 上下文聚合（API 兜底值）
		used := slot.NPromptTokens
		if slot.NDecoded != nil {
			used += *slot.NDecoded
		}
		if slot.NPromptTokens > 0 || (slot.NDecoded != nil && *slot.NDecoded > 0) {
			aggUsed += used
			hasUsed = true
		}
		if ct := intVal(s["n_ctx"]); ct > 0 {
			aggTotal += ct
		}
	}
	l.setSlots(out, totalGen, totalPrompt, aggUsed, aggTotal, hasUsed)
}

func (l *LlamaCollector) setSlots(out []model.Slot, totalGen, totalPrompt float64, aggUsed, aggTotal int, hasUsed bool) {
	api := l.currentAPI()
	api.Slots = out
	api.GenTPS = round2(totalGen)
	api.PromptTPS = round2(totalPrompt)
	if hasUsed {
		u := aggUsed
		api.CtxUsed = &u
	}
	if aggTotal > 0 {
		api.CtxTotal = aggTotal
	}
	api.Online = l.online
	api.Model = l.model
	l.world.SetLlama(api)
}

func (l *LlamaCollector) currentAPI() model.LlamaAPI {
	// 读取当前 world 中的 llama 状态作为基础（保留 online/model）
	// 由于 SetLlama 整体替换，这里构造新值
	return model.LlamaAPI{
		Online:    l.online,
		Model:     l.model,
		FailCount: l.failCount,
	}
}

func (l *LlamaCollector) diffSlot(st *slotState, slot model.Slot, now float64) {
	taskID := slot.IDTask
	nDecoded := -1
	if slot.NDecoded != nil {
		nDecoded = *slot.NDecoded
	}
	nPromptProcessed := slot.NPromptTokensProcessed
	var dt float64
	if st.prevTS != nil {
		dt = now - *st.prevTS
	}

	if slot.IsProcessing {
		if st.prevTaskID == nil {
			st.taskStartTS = &now
			l.events.SetTaskRunning(now, true, taskID, intPtr(slot.NPromptTokens))
		}
		if taskID != nil && (st.prevTaskID == nil || *taskID != *st.prevTaskID) ||
			(nDecoded >= 0 && nDecoded < st.prevDecoded) {
			// 新任务（或任务边界）：重置基线，本周期速度 = 0
			st.prevTaskID = taskID
			st.prevDecoded = max(0, nDecoded)
			st.prevPromptProc = nPromptProcessed
			st.genSpeed = 0
			st.promptSpeed = 0
		} else {
			if dt > 0 {
				if nDecoded >= 0 {
					st.genSpeed = maxf(0, float64(nDecoded-st.prevDecoded)/dt)
				}
				st.promptSpeed = maxf(0, float64(nPromptProcessed-st.prevPromptProc)/dt)
			}
			if nDecoded >= 0 {
				st.prevDecoded = nDecoded
			}
			st.prevPromptProc = nPromptProcessed
		}
	} else {
		if st.prevTaskID != nil {
			var duration *float64
			if st.taskStartTS != nil {
				d := now - *st.taskStartTS
				duration = &d
			}
			var avg *float64
			if duration != nil && *duration > 0 {
				a := float64(st.prevDecoded) / *duration
				avg = &a
			}
			l.events.TaskEndWithStats(now, st.prevTaskID, intPtr(st.prevDecoded), duration, avg, nil, nil, "")
			st.prevTaskID = nil
			st.taskStartTS = nil
		}
		st.genSpeed = 0
		st.promptSpeed = 0
	}
	st.prevTS = &now
}

func (l *LlamaCollector) pollSlow(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	model := l.model
	if props, err := l.getProps(); err == nil {
		model = mergeProps(model, props)
	}
	if m, err := l.getV1Model(); err == nil {
		model = mergeV1(model, m)
	}
	l.model = model
	if model.Path != "" && (l.modelPath == nil || *l.modelPath != model.Path) {
		if l.modelPath != nil {
			l.events.SetModel(float64(time.Now().Unix()), model.Path)
		}
		p := model.Path
		l.modelPath = &p
	}
}

func (l *LlamaCollector) getProps() (map[string]interface{}, error) {
	var props map[string]interface{}
	err := l.getJSON("/props", &props)
	return props, err
}

func (l *LlamaCollector) getV1Model() (map[string]interface{}, error) {
	var data struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := l.getJSON("/v1/models", &data); err != nil {
		return nil, err
	}
	if len(data.Data) == 0 {
		return nil, fmt.Errorf("no models")
	}
	return data.Data[0], nil
}

func (l *LlamaCollector) push() {
	l.mu.Lock()
	defer l.mu.Unlock()
	api := model.LlamaAPI{
		Online:    l.online,
		Model:     l.model,
		FailCount: l.failCount,
	}
	l.world.SetLlama(api)
}

// ---------- 模型合并 ----------

func mergeProps(m model.ModelInfo, props map[string]interface{}) model.ModelInfo {
	path, _ := props["model_path"].(string)
	m.Path = path
	if alias, ok := props["model_alias"].(string); ok && alias != "" {
		m.Name = alias
	} else if path != "" {
		m.Name = path[strings.LastIndex(path, "/")+1:]
	}
	for _, k := range []string{"ftype", "vocab_type", "owned_by"} {
		if v, ok := props[k].(string); ok {
			switch k {
			case "ftype":
				m.Ftype = v
			case "vocab_type":
				m.VocabType = v
			case "owned_by":
				m.OwnedBy = v
			}
		}
	}
	if v, ok := props["n_embd"]; ok {
		m.NEmbD = intPtr(int(int64ToFloat(v)))
	}
	if v, ok := props["n_vocab"]; ok {
		m.NVocab = intPtr(int(int64ToFloat(v)))
	}
	if v, ok := props["n_ctx"]; ok {
		m.NCtx = intPtr(int(int64ToFloat(v)))
	}
	if v, ok := props["n_ctx_train"]; ok {
		m.NCtxTrain = intPtr(int(int64ToFloat(v)))
	}
	if mods, ok := props["modalities"].([]interface{}); ok {
		m.Modalities = nil
		for _, x := range mods {
			if s, ok := x.(string); ok {
				m.Modalities = append(m.Modalities, s)
			}
		}
	}
	if caps, ok := props["capabilities"].([]interface{}); ok {
		m.Capabilities = nil
		for _, x := range caps {
			if s, ok := x.(string); ok {
				m.Capabilities = append(m.Capabilities, s)
			}
		}
	}
	return m
}

func mergeV1(m model.ModelInfo, v map[string]interface{}) model.ModelInfo {
	meta, _ := v["meta"].(map[string]interface{})
	get := func(k string) interface{} {
		if x, ok := v[k]; ok && x != nil {
			return x
		}
		if meta != nil {
			if x, ok := meta[k]; ok && x != nil {
				return x
			}
		}
		return nil
	}
	if x := get("n_params"); x != nil {
		m.NParams = f64ptr(int64ToFloat(x))
	}
	if x := get("n_embd"); x != nil {
		m.NEmbD = intPtr(int(int64ToFloat(x)))
	}
	if x := get("n_vocab"); x != nil {
		m.NVocab = intPtr(int(int64ToFloat(x)))
	}
	if x := get("size"); x != nil {
		m.Size = f64ptr(int64ToFloat(x))
		m.FileSize = m.Size
	}
	if x := get("ftype"); x != nil {
		if s, ok := x.(string); ok {
			m.Ftype = s
		}
	}
	if x := get("vocab_type"); x != nil {
		if s, ok := x.(string); ok {
			m.VocabType = s
		}
	}
	if x := get("n_ctx"); x != nil {
		m.NCtx = intPtr(int(int64ToFloat(x)))
	}
	if x := get("n_ctx_train"); x != nil {
		m.NCtxTrain = intPtr(int(int64ToFloat(x)))
	}
	if x, ok := v["owned_by"].(string); ok && x != "" {
		if m.OwnedBy == "" {
			m.OwnedBy = x
		}
	}
	return m
}

// ---------- 小工具 ----------

func slotDecoded(s map[string]interface{}) *int {
	if n, ok := s["n_decoded"]; ok && n != nil {
		return intPtr(int(int64ToFloat(n)))
	}
	if nt, ok := s["next_token"].([]interface{}); ok && len(nt) > 0 {
		if d, ok := nt[0].(map[string]interface{}); ok {
			if n, ok := d["n_decoded"]; ok && n != nil {
				return intPtr(int(int64ToFloat(n)))
			}
		}
	}
	return nil
}

func int64ToFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func intVal(v interface{}) int { return int(int64ToFloat(v)) }
func boolVal(v interface{}) bool {
	b, _ := v.(bool)
	return b
}
func strVal(v interface{}) string {
	s, _ := v.(string)
	return s
}
func f64ptr(f float64) *float64 { return &f }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
