package model

import "fmt"

// 默认阈值表（与面板 config.DEFAULT_THRESHOLDS 一致）。
// mtp 为“低于阈值告警”（inverted），其余为“高于阈值告警”。
var DefaultThresholds = map[string][2]float64{
	"gpu_util":  {80, 90},
	"gpu_mem":   {85, 95},
	"gpu_temp":  {75, 85},
	"gpu_power": {85, 95},
	"cpu":       {80, 90},
	"mem":       {85, 95},
	"disk":      {80, 90},
	"ctx":       {80, 90},
	"mtp":       {80, 65},
}

var InvertedMetrics = map[string]bool{"mtp": true}

// EvaluateAlerts 每次快照生成时评估，产出 alerts[]（与面板 alerts.evaluate_alerts 一致）。
func EvaluateAlerts(llama LlamaAPI, host HostMetrics, logst LogState) []Alert {
	var alerts []Alert

	add := func(metric, level string, value, threshold float64) {
		alerts = append(alerts, Alert{Metric: metric, Level: level, Value: value, Threshold: threshold})
	}
	check := func(metric string, value *float64, key string) {
		if value == nil {
			return
		}
		t := DefaultThresholds[key]
		warn, danger := t[0], t[1]
		v := *value
		if InvertedMetrics[key] {
			if v < danger {
				add(metric, "danger", v, danger)
			} else if v < warn {
				add(metric, "warn", v, warn)
			}
		} else {
			if v >= danger {
				add(metric, "danger", v, danger)
			} else if v >= warn {
				add(metric, "warn", v, warn)
			}
		}
	}

	if !llama.Online {
		add("llama", "danger", 0, 1)
	}
	if !host.Reachable {
		add("ssh", "warn", 0, 1)
	}

	for _, g := range host.GPUs {
		idx := g.Index
		util := g.UtilPct
		check(fmtMetric("gpu%d.util", idx), &util, "gpu_util")
		if g.MemTotalMB > 0 {
			memPct := round1(float64(g.MemUsedMB) / float64(g.MemTotalMB) * 100.0)
			check(fmtMetric("gpu%d.mem", idx), &memPct, "gpu_mem")
		}
		check(fmtMetric("gpu%d.temp", idx), g.TempC, "gpu_temp")
		if g.PowerLimitW != nil && *g.PowerLimitW > 0 && g.PowerW != nil {
			powerPct := round1(*g.PowerW / *g.PowerLimitW * 100.0)
			check(fmtMetric("gpu%d.power", idx), &powerPct, "gpu_power")
		}
	}

	if cpu := host.CPU.UsagePct; cpu != nil {
		check("cpu", cpu, "cpu")
	}
	if host.Mem.TotalMB > 0 {
		memPct := round1(float64(host.Mem.UsedMB) / float64(host.Mem.TotalMB) * 100.0)
		check("mem", &memPct, "mem")
	}
	for _, mnt := range host.Disk.Mounts {
		p := mnt.UsePct
		check("disk:"+mnt.Mount, &p, "disk")
	}

	if ctx := logst.Context.Pct; ctx != nil {
		check("ctx", ctx, "ctx")
	}
	if mtp := logst.MTP.Acceptance; mtp != nil {
		pct := round1(*mtp * 100.0)
		check("mtp", &pct, "mtp")
	}
	return alerts
}

func fmtMetric(f string, idx int) string {
	return fmt.Sprintf(f, idx)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
