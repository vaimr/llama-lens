package model

import "sync"

// RingBuffer 内存环形缓冲（与面板 store.RingBuffer 行为一致）。
// 每个序列为定长环形队列；value 为 nil 表示断点（离线/未知），曲线不插值。
type RingBuffer struct {
	mu     sync.Mutex
	maxlen int
	series map[string][]pt
	pos    map[string]int
}

type pt struct {
	ts float64
	v  *float64
}

func NewRingBuffer(maxlen int) *RingBuffer {
	return &RingBuffer{
		maxlen: maxlen,
		series: make(map[string][]pt),
		pos:    make(map[string]int),
	}
}

func (r *RingBuffer) Push(name string, ts float64, v *float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dq, ok := r.series[name]
	if !ok {
		dq = make([]pt, 0, r.maxlen)
		r.series[name] = dq
		r.pos[name] = 0
	}
	if len(dq) < r.maxlen {
		dq = append(dq, pt{ts, v})
	} else {
		dq[r.pos[name]] = pt{ts, v}
		r.pos[name] = (r.pos[name] + 1) % r.maxlen
	}
	r.series[name] = dq
}

// Window 返回 [now-windowS, now] 内的 (ts, value) 序列（按时间升序）。
func (r *RingBuffer) Window(name string, windowS, now float64) []pt {
	r.mu.Lock()
	defer r.mu.Unlock()
	dq := r.series[name]
	if len(dq) == 0 {
		return nil
	}
	cutoff := now - windowS
	out := make([]pt, 0, len(dq))
	if len(dq) < r.maxlen {
		for _, p := range dq {
			if p.ts >= cutoff {
				out = append(out, p)
			}
		}
		return out
	}
	start := r.pos[name]
	for i := 0; i < len(dq); i++ {
		p := dq[(start+i)%len(dq)]
		if p.ts >= cutoff {
			out = append(out, p)
		}
	}
	return out
}
