package model

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const taskEndGraceS = 2.0 // API 源任务结束事件宽限期：等日志源补全完整统计

// EventDetector 状态迁移检测（与面板 events.EventDetector 行为一致）。
type EventDetector struct {
	mu sync.Mutex

	events []Event
	maxlen int

	llamaOnline *bool
	taskRunning bool
	runningID   *int
	modelPath   *string

	pendingEnd   *taskEndArgs
	pendingTimer *time.Timer
	endedIDs     map[int]bool
	endedOrder   []int

	alertLevels map[string]alertLevel
}

type alertLevel struct {
	level    string
	lastEmit *float64
}

type taskEndArgs struct {
	ts             float64
	taskID         *int
	totalTokens    *int
	durationS      *float64
	avgTPS         *float64
	mtpAcceptance  *float64
	ctxUsed        *int
	note           string
}

func NewEventDetector(maxlen int) *EventDetector {
	if maxlen <= 0 {
		maxlen = 200
	}
	return &EventDetector{
		maxlen:      maxlen,
		endedIDs:    make(map[int]bool),
		alertLevels: make(map[string]alertLevel),
	}
}

func (d *EventDetector) Emit(ts float64, level, typ, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ts == 0 {
		ts = float64(time.Now().Unix())
	}
	d.events = append(d.events, Event{TS: ts, Level: level, Type: typ, Msg: msg})
	if len(d.events) > d.maxlen {
		d.events = d.events[len(d.events)-d.maxlen:]
	}
}

func (d *EventDetector) SetLlamaOnline(ts float64, online bool, modelName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.llamaOnline == nil {
		d.llamaOnline = &online
		if online {
			d.emitLocked(ts, "info", "llama_up", "llama 上线"+modelSuffix(modelName))
		}
		return
	}
	if *d.llamaOnline != online {
		d.llamaOnline = &online
		if online {
			d.emitLocked(ts, "info", "llama_up", "llama 恢复在线"+modelSuffix(modelName))
		} else {
			d.resetTaskStateLocked()
			d.emitLocked(ts, "error", "llama_down", "llama 离线")
		}
	}
}

func (d *EventDetector) ResetTaskState() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.resetTaskStateLocked()
}

func (d *EventDetector) resetTaskStateLocked() {
	d.taskRunning = false
	d.runningID = nil
	d.endedIDs = make(map[int]bool)
	d.endedOrder = nil
	d.cancelPendingLocked()
}

func (d *EventDetector) SetTaskRunning(ts float64, running bool, taskID *int, promptTokens *int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if running && !d.taskRunning {
		d.taskRunning = true
		d.runningID = taskID
		d.emitLocked(ts, "info", "task_start", taskStartMsg(taskID, promptTokens))
	} else if running && d.taskRunning && taskID != nil && (d.runningID == nil || *d.runningID != *taskID) {
		d.firePendingLocked()
		d.taskRunning = true
		d.runningID = taskID
		d.emitLocked(ts, "info", "task_start", taskStartMsg(taskID, promptTokens))
	} else if !running && d.taskRunning {
		d.taskRunning = false
		d.runningID = nil
		d.cancelPendingLocked()
		msg := "任务结束"
		if taskID != nil {
			msg = fmt.Sprintf("任务 #%d 结束", *taskID)
		}
		d.emitLocked(ts, "info", "task_end", msg)
	}
}

// TaskEndWithStats 任务结束（带统计）。完整统计（MTP/上下文）立即发；
// 不完整（API 源先到）延迟宽限期给日志源补全，超时用部分统计兜底。
func (d *EventDetector) TaskEndWithStats(ts float64, taskID *int, totalTokens *int,
	durationS, avgTPS, mtpAcceptance *float64, ctxUsed *int, note string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	complete := mtpAcceptance != nil || ctxUsed != nil
	if complete {
		d.cancelPendingLocked()
		d.emitTaskEndLocked(ts, taskID, totalTokens, durationS, avgTPS, mtpAcceptance, ctxUsed, note)
		return
	}
	if !d.taskRunning {
		return
	}
	if d.pendingEnd == nil {
		args := &taskEndArgs{ts: ts, taskID: taskID, totalTokens: totalTokens,
			durationS: durationS, avgTPS: avgTPS, mtpAcceptance: mtpAcceptance, ctxUsed: ctxUsed, note: note}
		d.pendingEnd = args
		d.pendingTimer = time.AfterFunc(taskEndGraceS*time.Second, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.firePendingLocked()
		})
	}
}

func (d *EventDetector) emitTaskEndLocked(ts float64, taskID *int, totalTokens *int,
	durationS, avgTPS, mtpAcceptance *float64, ctxUsed *int, note string) {
	if taskID != nil {
		if d.endedIDs[*taskID] {
			return // 双源去重
		}
		d.endedIDs[*taskID] = true
		d.endedOrder = append(d.endedOrder, *taskID)
		if len(d.endedOrder) > 200 {
			old := d.endedOrder[0]
			d.endedOrder = d.endedOrder[1:]
			delete(d.endedIDs, old)
		}
	}
	if d.runningID == nil || (taskID != nil && *d.runningID == *taskID) {
		d.taskRunning = false
		d.runningID = nil
	}
	var parts []string
	if taskID != nil {
		parts = append(parts, fmt.Sprintf("任务 #%d 结束", *taskID))
	} else {
		parts = append(parts, "任务结束")
	}
	if note != "" {
		parts = append(parts, "("+note+")")
	}
	var stats []string
	if totalTokens != nil {
		stats = append(stats, fmt.Sprintf("%d tokens", *totalTokens))
	}
	if durationS != nil {
		stats = append(stats, fmtDuration(*durationS))
	}
	if avgTPS != nil {
		stats = append(stats, fmt.Sprintf("平均 %.1f tok/s", *avgTPS))
	}
	if mtpAcceptance != nil {
		stats = append(stats, fmt.Sprintf("MTP 接受率 %.1f%%", *mtpAcceptance*100))
	}
	if ctxUsed != nil {
		stats = append(stats, fmt.Sprintf("上下文 %d", *ctxUsed))
	}
	msg := strings.Join(parts, " ")
	if len(stats) > 0 {
		msg += ": " + strings.Join(stats, " · ")
	}
	d.emitLocked(ts, "info", "task_end", msg)
}

func (d *EventDetector) firePendingLocked() {
	args := d.pendingEnd
	d.pendingEnd = nil
	if d.pendingTimer != nil {
		d.pendingTimer.Stop()
		d.pendingTimer = nil
	}
	if args == nil {
		return
	}
	d.emitTaskEndLocked(args.ts, args.taskID, args.totalTokens, args.durationS,
		args.avgTPS, args.mtpAcceptance, args.ctxUsed, args.note)
}

func (d *EventDetector) cancelPendingLocked() {
	d.pendingEnd = nil
	if d.pendingTimer != nil {
		d.pendingTimer.Stop()
		d.pendingTimer = nil
	}
}

func (d *EventDetector) SetModel(ts float64, modelPath string) {
	if modelPath == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.modelPath == nil {
		d.modelPath = &modelPath
		return
	}
	if *d.modelPath != modelPath {
		old := *d.modelPath
		d.modelPath = &modelPath
		d.emitLocked(ts, "warn", "model_change", fmt.Sprintf("模型变更: %s → %s", shortPath(old), shortPath(modelPath)))
	}
}

func (d *EventDetector) LlamaBoot(ts float64, info string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.emitLocked(ts, "info", "llama_boot", "llama 启动: "+info)
}

const alertCooldownS = 30.0

var alertNames = map[string]string{
	"cpu": "CPU", "mem": "内存", "ctx": "上下文", "mtp": "MTP 接受率",
}
var gpuKindNames = map[string]string{
	"util": "利用率", "mem": "显存", "temp": "温度", "power": "功耗",
}

func alertName(metric string) string {
	if n, ok := alertNames[metric]; ok {
		return n
	}
	if strings.HasPrefix(metric, "gpu") && strings.Contains(metric, ".") {
		parts := strings.SplitN(metric, ".", 2)
		return fmt.Sprintf("GPU%s %s", parts[0][3:], gpuKindNames[parts[1]])
	}
	if strings.HasPrefix(metric, "disk:") {
		return "磁盘 " + metric[5:]
	}
	return metric
}

// CheckAlerts 每次快照 evaluate_alerts 后调用：级别变化（升级/恢复）时发事件。
func (d *EventDetector) CheckAlerts(alerts []Alert) {
	now := float64(time.Now().UnixNano()) / 1e9
	d.mu.Lock()
	defer d.mu.Unlock()
	current := make(map[string]Alert)
	for _, a := range alerts {
		if a.Metric != "llama" && a.Metric != "ssh" {
			current[a.Metric] = a
		}
	}
	for metric, a := range current {
		level := a.Level
		prev, ok := d.alertLevels[metric]
		if !ok {
			d.alertLevels[metric] = alertLevel{level: level}
			continue
		}
		if level == prev.level {
			continue
		}
		if prev.lastEmit != nil && now-*prev.lastEmit < alertCooldownS {
			d.alertLevels[metric] = alertLevel{level: level, lastEmit: prev.lastEmit}
			continue
		}
		op := ">="
		if InvertedMetrics[metric] {
			op = "<="
		}
		tag := "警告"
		if level == "danger" {
			tag = "危险"
		}
		msg := fmt.Sprintf("%s %s %s %s（%s）", alertName(metric), fmtAlertVal(metric, a.Value),
			op, fmtAlertVal(metric, a.Threshold), tag)
		d.emitLocked(now, level, "alert", msg)
		d.alertLevels[metric] = alertLevel{level: level, lastEmit: &now}
	}
	for metric, lv := range d.alertLevels {
		if lv.level == "normal" {
			continue
		}
		if _, ok := current[metric]; ok {
			continue
		}
		if lv.lastEmit == nil {
			d.alertLevels[metric] = alertLevel{level: "normal"}
			continue
		}
		if now-*lv.lastEmit < alertCooldownS {
			continue
		}
		d.emitLocked(now, "info", "alert", alertName(metric)+" 恢复正常")
		d.alertLevels[metric] = alertLevel{level: "normal", lastEmit: &now}
	}
}

func (d *EventDetector) List(limit int) []Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := make([]Event, len(d.events))
	copy(items, d.events)
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func (d *EventDetector) emitLocked(ts float64, level, typ, msg string) {
	d.events = append(d.events, Event{TS: ts, Level: level, Type: typ, Msg: msg})
	if len(d.events) > d.maxlen {
		d.events = d.events[len(d.events)-d.maxlen:]
	}
}

func modelSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " · 模型 " + name
}

func taskStartMsg(taskID, promptTokens *int) string {
	msg := "任务开始"
	if taskID != nil {
		msg = fmt.Sprintf("任务 #%d 开始", *taskID)
	}
	if promptTokens != nil {
		msg += fmt.Sprintf(" (prompt %d tokens)", *promptTokens)
	}
	return msg
}

func shortPath(p string) string {
	if p == "" {
		return p
	}
	return p[strings.LastIndex(p, "/")+1:]
}

func fmtAlertVal(metric string, v float64) string {
	s := fmt.Sprintf("%.1f", v)
	if strings.HasSuffix(metric, ".temp") {
		return s + "°C"
	}
	return s + "%"
}

func fmtDuration(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	m := int(seconds) / 60
	s := int(seconds) % 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := m / 60
	return fmt.Sprintf("%dh%02dm", h, m%60)
}
