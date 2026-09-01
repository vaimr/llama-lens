package model

import "sync"

const CLKTCK = 100 // Linux 标准时钟滴答频率（/proc 计数字段单位）

// DiffEngine 基于连续两次采样计算速率/利用率（与面板 diff.DiffEngine 一致）。
type DiffEngine struct {
	mu   sync.Mutex
	prev map[string]prevEntry
}

type prevEntry struct {
	ts  float64
	a   float64 // total / value / ticks
	idle float64
}

func NewDiffEngine() *DiffEngine {
	return &DiffEngine{prev: make(map[string]prevEntry)}
}

// CPUPct 整机/每核 CPU 利用率（%）。total/idle 为 /proc/stat 累计字段。
func (d *DiffEngine) CPUPct(key string, ts, total, idle float64) *float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, ok := d.prev[key]
	d.prev[key] = prevEntry{ts, total, idle}
	if !ok {
		return nil
	}
	dtTotal := total - prev.a
	dtIdle := idle - prev.idle
	if dtTotal <= 0 {
		return nil
	}
	pct := (1.0 - dtIdle/dtTotal) * 100.0
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return &pct
}

// ProcessCPUPct 进程实时 CPU%（相对单核）。ticks = utime+stime。
func (d *DiffEngine) ProcessCPUPct(key string, ts, ticks float64) *float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, ok := d.prev[key]
	d.prev[key] = prevEntry{ts, ticks, 0}
	if !ok {
		return nil
	}
	dt := ts - prev.ts
	if dt <= 0 {
		return nil
	}
	pct := (ticks - prev.a) / CLKTCK / dt * 100.0
	if pct < 0 {
		pct = 0
	}
	return &pct
}

// BytesRate 通用速率：Δvalue/Δt（磁盘扇区、网络字节等）。
func (d *DiffEngine) BytesRate(key string, ts, value float64) *float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, ok := d.prev[key]
	d.prev[key] = prevEntry{ts, value, 0}
	if !ok {
		return nil
	}
	dt := ts - prev.ts
	if dt <= 0 {
		return nil
	}
	r := (value - prev.a) / dt
	if r < 0 {
		r = 0
	}
	return &r
}

// Prune 删除 prefix 下不在 keep 中的基线条目（防字典无限增长）。
func (d *DiffEngine) Prune(prefix string, keep map[int]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k := range d.prev {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			tail := k[len(prefix):]
			pid := 0
			for _, c := range tail {
				if c < '0' || c > '9' {
					pid = -1
					break
				}
				pid = pid*10 + int(c-'0')
			}
			if pid >= 0 && !keep[pid] {
				delete(d.prev, k)
			}
		}
	}
}
