package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"llamalens-cli/internal/cfg"
	"llamalens-cli/internal/model"
)

// HostCollector 每 SysInterval（默认 2s）采集本机指标：
// /proc 直读（CPU/内存/负载/网络/磁盘/进程）+ nvidia-smi / rocm-smi（GPU）+ systemctl（服务）。
// 全部只读，不修改主机任何配置。
type HostCollector struct {
	cfg    *cfg.Config
	world  *model.World
	diff   *model.DiffEngine
	events *model.EventDetector

	staticOnce  bool
	sys         model.SysInfo
	lastDF      time.Time
	dfCache     []model.Mount
	lastService time.Time
	svcCache    model.ServiceInfo
}

func NewHostCollector(c *cfg.Config, w *model.World, diff *model.DiffEngine, ev *model.EventDetector) *HostCollector {
	return &HostCollector{cfg: c, world: w, diff: diff, events: ev}
}

// Run 启动采集循环（阻塞）。
func (h *HostCollector) Run(ctx context.Context) {
	if !h.staticOnce {
		h.sys = h.collectStatic()
		h.staticOnce = true
	}
	ticker := time.NewTicker(time.Duration(h.cfg.SysInterval * float64(time.Second)))
	defer ticker.Stop()
	h.collect(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.collect(ctx)
		}
	}
}

func (h *HostCollector) collect(ctx context.Context) {
	now := time.Now().Unix()
	hm := model.HostMetrics{
		Reachable: true,
		Sys:       h.sys,
	}

	// CPU（整机 + 每核差分）
	stat := readProcStat()
	if stat.total > 0 {
		hm.CPU.UsagePct = h.diff.CPUPct("cpu", float64(now), float64(stat.total), float64(stat.idle))
		cores := make([]int, 0, len(stat.cores))
		for i := range stat.cores {
			cores = append(cores, i)
		}
		sort.Ints(cores)
		for _, i := range cores {
			total, idle := stat.cores[i][0], stat.cores[i][1]
			pct := h.diff.CPUPct(fmt.Sprintf("cpu:%d", i), float64(now), float64(total), float64(idle))
			if pct != nil {
				hm.CPU.PerCorePct = append(hm.CPU.PerCorePct, *pct)
			}
		}
	}
	hm.CPU.Load = readLoadavg()

	// 内存
	hm.Mem = readMeminfo()

	// 磁盘（df 30s 降频：挂载占用变化慢；diskstats 差分仍每周期）
	if h.lastDF.IsZero() || time.Since(h.lastDF) >= 30*time.Second {
		h.dfCache = readDF(h.cfg.Mounts)
		h.lastDF = time.Now()
	}
	hm.Disk.Mounts = h.dfCache
	dio := readDiskstats()
	if r := h.diff.BytesRate("diskr", float64(now), float64(dio.read)); r != nil {
		hm.Disk.ReadMBs = *r * 512 / 1024 / 1024
	}
	if w := h.diff.BytesRate("diskw", float64(now), float64(dio.written)); w != nil {
		hm.Disk.WriteMBs = *w * 512 / 1024 / 1024
	}

	// 网络（差分）
	ifaces := readNetdev()
	netOut := make([]model.Iface, 0, len(ifaces))
	for _, itf := range ifaces {
		var rx, tx *float64
		if r := h.diff.BytesRate("netrx:"+itf.name, float64(now), float64(itf.rxBytes)); r != nil {
			rx = r
		}
		if t := h.diff.BytesRate("nettx:"+itf.name, float64(now), float64(itf.txBytes)); t != nil {
			tx = t
		}
		nio := model.Iface{
			Name:      itf.name,
			RxTotalMB: float64(itf.rxBytes) / 1024 / 1024,
			TxTotalMB: float64(itf.txBytes) / 1024 / 1024,
		}
		if rx != nil {
			nio.RxMBs = *rx / 1024 / 1024
		}
		if tx != nil {
			nio.TxMBs = *tx / 1024 / 1024
		}
		netOut = append(netOut, nio)
	}
	hm.Net.Ifaces = netOut

	// 进程（llama-server）
	hm.Process = h.readLlamaProcess(float64(now), hm.Mem.TotalMB)

	// 服务（systemctl 15s 降频：D-Bus 往返较慢，服务状态变化慢）
	if h.lastService.IsZero() || time.Since(h.lastService) >= 15*time.Second {
		h.svcCache = h.readService()
		h.lastService = time.Now()
	}
	hm.Service = h.svcCache
	hm.Service.Unit = h.cfg.Unit + ".service"

	// GPU
	hm.GPUs = h.readGPUs(ctx)

	// Top 进程
	hm.TopCPU, hm.TopMem = h.readTopProcs(float64(now))

	// 系统
	if up := readUptime(); up != nil {
		hm.Sys.UptimeS = up
	}
	hm.Sys.Procs = countProcs()

	h.world.SetHost(hm)
	h.world.PushHostRing(hm)
}

// ---------- 静态信息（首次采集一次） ----------

func (h *HostCollector) collectStatic() model.SysInfo {
	s := model.SysInfo{Hostname: h.cfg.Name}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		s.Kernel = strings.TrimSpace(string(out))
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				s.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cores := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "processor") {
				cores++
			}
			if s.CPUModel == "" {
				if idx := strings.Index(line, "model name"); idx >= 0 {
					if colon := strings.Index(line, ":"); colon >= 0 {
						s.CPUModel = strings.TrimSpace(line[colon+1:])
					}
				}
			}
		}
		s.Cores = cores
	}
	return s
}

// ---------- /proc 直读 ----------

type procStat struct {
	total int64
	idle  int64
	cores map[int][2]int64
}

func readProcStat() procStat {
	res := procStat{cores: make(map[int][2]int64)}
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return res
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		name := fields[0]
		var total int64
		for _, x := range fields[1:] {
			total += atoi64(x)
		}
		idle := atoi64(fields[4])
		if len(fields) > 5 {
			idle += atoi64(fields[5])
		}
		if name == "cpu" {
			res.total = total
			res.idle = idle
		} else if i, err := strconv.Atoi(name[3:]); err == nil {
			res.cores[i] = [2]int64{total, idle}
		}
	}
	return res
}

func readLoadavg() [3]float64 {
	var load [3]float64
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return load
	}
	fields := strings.Fields(string(b))
	for i := 0; i < 3 && i < len(fields); i++ {
		load[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return load
}

func readMeminfo() model.MemInfo {
	var m model.MemInfo
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return m
	}
	kv := make(map[string]int64)
	for _, line := range strings.Split(string(b), "\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			rest := strings.Fields(line[idx+1:])
			if len(rest) > 0 {
				kv[key] = atoi64(rest[0])
			}
		}
	}
	total := kv["MemTotal"]
	free := kv["MemFree"]
	buffCache := kv["Buffers"] + kv["Cached"]
	m.TotalMB = int(total / 1024)
	m.UsedMB = int((total - free - buffCache) / 1024)
	m.FreeMB = int(free / 1024)
	m.BuffCacheMB = int(buffCache / 1024)
	m.AvailableMB = int(kv["MemAvailable"] / 1024)
	m.SwapTotalMB = int(kv["SwapTotal"] / 1024)
	m.SwapUsedMB = int((kv["SwapTotal"] - kv["SwapFree"]) / 1024)
	return m
}

func readUptime() *float64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return nil
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	return &f
}

var netExcludePrefixes = []string{"docker", "br-", "veth", "virbr", "kube", "cni", "cali", "fla", "tun", "tap"}

type netIface struct {
	name    string
	rxBytes int64
	txBytes int64
}

func readNetdev() []netIface {
	var out []netIface
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name == "lo" || isExcludedIface(name) {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 9 {
			continue
		}
		out = append(out, netIface{
			name:    name,
			rxBytes: atoi64(fields[0]),
			txBytes: atoi64(fields[8]),
		})
	}
	return out
}

func isExcludedIface(name string) bool {
	for _, p := range netExcludePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

type diskIO struct {
	read   int64
	written int64
}

var diskRe = regexp.MustCompile(`^(sd|vd|nvme)`)

func readDiskstats() diskIO {
	var d diskIO
	b, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return d
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		dev := fields[2]
		if !diskRe.MatchString(dev) {
			continue
		}
		// 排除分区（sda1、nvme0n1p1 等）
		if isPartition(dev) {
			continue
		}
		d.read += atoi64(fields[5])
		d.written += atoi64(fields[9])
	}
	return d
}

func isPartition(dev string) bool {
	// sda1 / vda1 / nvme0n1p1 / xvd a1
	if len(dev) < 2 {
		return false
	}
	last := dev[len(dev)-1]
	if last >= '0' && last <= '9' {
		// nvme 设备以 p 开头分区（nvme0n1p1），其余直接数字
		if strings.HasPrefix(dev, "nvme") {
			return strings.Contains(dev[len("nvme"):], "p")
		}
		// 基础设备名（sda/vda/hda）后直接跟数字 = 分区
		base := dev
		for len(base) > 0 {
			c := base[len(base)-1]
			if c >= '0' && c <= '9' {
				base = base[:len(base)-1]
			} else {
				break
			}
		}
		return base != dev && len(base) >= 3
	}
	return false
}

func countProcs() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && e.Name()[0] >= '0' && e.Name()[0] <= '9' {
			n++
		}
	}
	return n
}

// ---------- df ----------

func readDF(mounts []string) []model.Mount {
	args := append([]string{"-B1", "--output=source,target,size,used,avail,pcent"}, mounts...)
	out, err := exec.Command("df", args...).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var outMounts []model.Mount
	seen := make(map[string]bool)
	for _, line := range lines[1:] { // 跳过表头
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		src := fields[0]
		if seen[src] {
			continue
		}
		seen[src] = true
		m := model.Mount{Mount: fields[1]}
		m.SizeGB = float64(atoi64(fields[2])) / 1024 / 1024 / 1024
		m.UsedGB = float64(atoi64(fields[3])) / 1024 / 1024 / 1024
		m.AvailGB = float64(atoi64(fields[4])) / 1024 / 1024 / 1024
		m.UsePct, _ = strconv.ParseFloat(strings.TrimSuffix(fields[5], "%"), 64)
		outMounts = append(outMounts, m)
	}
	return outMounts
}

// ---------- llama-server 进程 ----------

func (h *HostCollector) readLlamaProcess(now float64, memTotalMB int) *model.ProcInfo {
	pid := findPID(h.cfg.ProcessName)
	if pid <= 0 {
		return &model.ProcInfo{Found: false, Name: h.cfg.ProcessName}
	}
	p := &model.ProcInfo{Found: true, PID: pid, Name: h.cfg.ProcessName}

	// /proc/<pid>/stat → utime stime vsize starttime（生命周期 CPU%/运行时长由 /proc 计算，免 ps 子进程）
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if utime, stime, vsize, starttime, ok := parseProcStat(b); ok {
			p.VSZMB = intPtr(int(vsize / 1024 / 1024))
			rt := h.diff.ProcessCPUPct(fmt.Sprintf("proc:%d", pid), now, float64(utime+stime))
			p.CPUPctRealtime = rt
			if up := readUptime(); up != nil && starttime > 0 {
				elapsed := *up - float64(starttime)/userHz
				if elapsed > 0 {
					p.Elapsed = formatEtime(int64(elapsed))
					if cpuSec := float64(utime+stime) / userHz; cpuSec > 0 {
						p.CPUPctLifetime = f64p(cpuSec / elapsed * 100)
					}
				}
			}
		}
	}
	// /proc/<pid>/status → VmRSS VmSize Threads
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch fields[0] {
			case "VmRSS:":
				p.RSSMB = intPtr(int(atoi64(fields[1]) / 1024))
			case "VmSize:":
				p.VSZMB = intPtr(int(atoi64(fields[1]) / 1024))
			case "Threads:":
				p.Threads = intPtr(int(atoi64(fields[1])))
			}
		}
	}
	// 内存% = RSS / MemTotal（与 ps pmem 同口径）
	if p.RSSMB != nil && memTotalMB > 0 {
		p.MemPct = f64p(float64(*p.RSSMB) / float64(memTotalMB) * 100)
	}
	// cmdline
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		cmd := strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
		p.Cmdline = cmd
		p.Flags = parseCmdline(cmd)
	}
	return p
}

func findPID(name string) int {
	return findPIDIn(name, "/proc")
}

// findPIDIn 先 comm 精确匹配（快路径）；失败再回退 cmdline argv[0] basename 匹配。
// 内核把 comm 截断到 15 字符，长进程名（如 llama-cpp-turboquant）只能靠 cmdline 匹配全名。
func findPIDIn(name, procDir string) int {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !isProcEntry(e) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(procDir, e.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == name {
			pid, _ := strconv.Atoi(e.Name())
			return pid
		}
	}
	for _, e := range entries {
		if !isProcEntry(e) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(procDir, e.Name(), "cmdline"))
		if err != nil || len(b) == 0 {
			continue
		}
		argv0 := strings.SplitN(string(b), "\x00", 2)[0]
		if filepath.Base(argv0) == name {
			pid, _ := strconv.Atoi(e.Name())
			return pid
		}
	}
	return 0
}

func isProcEntry(e os.DirEntry) bool {
	n := e.Name()
	return e.IsDir() && len(n) > 0 && n[0] >= '0' && n[0] <= '9'
}

// userHz Linux USER_HZ（clock ticks/秒），/proc stat 的 utime/stime/starttime 单位
const userHz = 100

func parseProcStat(data []byte) (utime, stime, vsize, starttime int64, ok bool) {
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, 0, 0, 0, false
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 21 {
		return 0, 0, 0, 0, false
	}
	utime = atoi64(fields[11])
	stime = atoi64(fields[12])
	vsize = atoi64(fields[20])
	starttime = atoi64(fields[19])
	return utime, stime, vsize, starttime, true
}

// formatEtime 按 ps etime 风格格式化秒数：[[dd-]hh:]mm:ss
func formatEtime(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	d, rem := sec/86400, sec%86400
	h, rem := rem/3600, rem%3600
	m, s := rem/60, rem%60
	switch {
	case d > 0:
		return fmt.Sprintf("%d-%02d:%02d:%02d", d, h, m, s)
	case h > 0:
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	default:
		return fmt.Sprintf("%d:%02d", m, s)
	}
}

// ---------- 服务 ----------

func (h *HostCollector) readService() model.ServiceInfo {
	var s model.ServiceInfo
	unit := h.cfg.Unit + ".service"
	out, err := exec.Command("systemctl", "show", unit,
		"-p", "Description,ActiveState,SubState,ExecMainStartTimestamp,CPUUsageNSec,MemoryCurrent,MemoryPeak,NTasks").Output()
	if err != nil {
		return s
	}
	kv := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		if idx := strings.Index(line, "="); idx > 0 {
			kv[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	s.Description = kv["Description"]
	active, sub := kv["ActiveState"], kv["SubState"]
	if active != "" {
		s.Active = active + " (" + sub + ")"
	}
	s.Since = kv["ExecMainStartTimestamp"]
	if ns := atoi64(kv["CPUUsageNSec"]); ns > 0 {
		s.CPUTotal = fmtSeconds(float64(ns) / 1e9)
	}
	if b := atoi64(kv["MemoryCurrent"]); b > 0 {
		s.Memory = fmtBytes(b)
	}
	if b := atoi64(kv["MemoryPeak"]); b > 0 {
		s.MemoryPeak = fmtBytes(b)
	}
	s.Tasks = int(atoi64(kv["NTasks"]))
	return s
}

// ---------- Top 进程 ----------

type procSample struct {
	pid   int
	name  string
	ticks int64
	rssMB int
}

func (h *HostCollector) readTopProcs(now float64) (topCPU, topMem []model.TopProc) {
	samples := h.scanProcs()
	totalMB := readMeminfo().TotalMB
	rows := make([]model.TopProc, 0, len(samples))
	keep := make(map[int]bool)
	for _, s := range samples {
		keep[s.pid] = true
		rt := h.diff.ProcessCPUPct(fmt.Sprintf("ps:%d", s.pid), now, float64(s.ticks))
		cpu := 0.0
		if rt != nil {
			cpu = *rt
		}
		memPct := 0.0
		if totalMB > 0 {
			memPct = float64(s.rssMB) / float64(totalMB) * 100.0
		}
		rows = append(rows, model.TopProc{
			PID:    s.pid,
			Name:   s.name,
			CPUPct: cpu,
			MemPct: memPct,
			RSSMB:  s.rssMB,
		})
	}
	h.diff.Prune("ps:", keep)

	// Top CPU：实时 CPU% 并列时按累计 ticks 兜底，再按 RSS
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CPUPct != rows[j].CPUPct {
			return rows[i].CPUPct > rows[j].CPUPct
		}
		return rows[i].RSSMB > rows[j].RSSMB
	})
	if len(rows) > 8 {
		topCPU = rows[:8]
	} else {
		topCPU = rows
	}
	// Top Mem：按 RSS
	memRows := make([]model.TopProc, len(rows))
	copy(memRows, rows)
	sort.SliceStable(memRows, func(i, j int) bool {
		if memRows[i].RSSMB != memRows[j].RSSMB {
			return memRows[i].RSSMB > memRows[j].RSSMB
		}
		return memRows[i].CPUPct > memRows[j].CPUPct
	})
	if len(memRows) > 8 {
		topMem = memRows[:8]
	} else {
		topMem = memRows
	}
	return
}

func (h *HostCollector) scanProcs() []procSample {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []procSample
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		pid, _ := strconv.Atoi(e.Name())
		commB, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(commB))
		statB, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		utime, stime, _, _, ok := parseProcStat(statB)
		if !ok {
			continue
		}
		rssMB := 0
		if stB, err := os.ReadFile(filepath.Join("/proc", e.Name(), "status")); err == nil {
			for _, line := range strings.Split(string(stB), "\n") {
				if strings.HasPrefix(line, "VmRSS:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						rssMB = int(atoi64(fields[1]) / 1024)
					}
					break
				}
			}
		}
		out = append(out, procSample{pid: pid, name: name, ticks: utime + stime, rssMB: rssMB})
	}
	return out
}

// ---------- GPU（nvidia-smi） ----------

const gpuQuery = "index,name,driver_version,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,power.limit,fan.speed,clocks.current.graphics,clocks.current.memory,pcie.link.gen.current,pcie.link.width.current,pstate,temperature.memory,ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total,clocks_throttle_reasons.active,uuid"

func (h *HostCollector) readGPUs(ctx context.Context) []model.GPU {
	// Try NVIDIA first, then AMD. Return first that yields data.
	if gpus := h.readGPUsNVIDIA(ctx); len(gpus) > 0 {
		return gpus
	}
	return h.readGPUsAMD(ctx)
}

// ---------- GPU（nvidia-smi） ----------

func (h *HostCollector) readGPUsNVIDIA(ctx context.Context) []model.GPU {
	// 1. Presence probe — nvidia-smi 不存在＝非 NVIDIA GPU 或无 GPU，正常静默。
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}

	smiHost := ""
	if h.cfg != nil {
		smiHost = h.cfg.Name
	}

	// 2. GPU query — direct exec (no bash wrapper).
	gpuCmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu="+gpuQuery,
		"--format=csv,noheader,nounits")
	gpuOut, gpuErr := gpuCmd.CombinedOutput()

	if gpuErr != nil {
		// nvidia-smi failed — log exit code + stderr.
		exitCode := -1
		if gpuCmd.ProcessState != nil {
			exitCode = gpuCmd.ProcessState.ExitCode()
		}
		var msg strings.Builder
		msg.WriteString(fmt.Sprintf("[collect] nvidia-smi (%s): query failed (exit %d)", smiHost, exitCode))
		outTrim := strings.TrimSpace(string(gpuOut))
		if outTrim != "" {
			lines := strings.Split(outTrim, "\n")
			if len(lines) > 40 {
				lines = lines[:40]
			}
			msg.WriteString(":\n  ")
			msg.WriteString(strings.ReplaceAll(strings.Join(lines, "\n  "), "\n", "\n  "))
		}
		log.Printf("%s", msg.String())
		return nil
	}

	// 3. Check for empty output (nvidia-smi succeeded but no GPU rows).
	gpuLines := strings.Split(string(gpuOut), "\n")
	gpuRows := 0
	for _, line := range gpuLines {
		if strings.TrimSpace(line) != "" {
			gpuRows++
		}
	}
	if gpuRows == 0 {
		log.Printf("[collect] nvidia-smi (%s): no GPU data", smiHost)
		return nil
	}

	// 4. Parse GPU rows (22-field CSV).
	var gpus []model.GPU
	for _, line := range gpuLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 22 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		g := model.GPU{
			Index:      atoi(parts[0]),
			Name:       parts[1],
			Driver:     parts[2],
			MemTotalMB: atoi(parts[3]),
			MemUsedMB:  atoi(parts[4]),
			MemFreeMB:  atoi(parts[5]),
			UtilPct:    atof(parts[6]),
			MemUtilPct: atof(parts[7]),
		}
		g.TempC = f64pOrNull(parts[8])
		g.PowerW = f64pOrNull(parts[9])
		g.PowerLimitW = f64pOrNull(parts[10])
		if fan := atof(parts[11]); fan >= 0 {
			g.FanPct = f64p(fan)
		}
		g.ClockMHz = intFromStr(parts[12])
		g.MemClockMHz = intFromStr(parts[13])
		g.PCIEGen = intFromStr(parts[14])
		g.PCIEWidth = intFromStr(parts[15])
		g.PState = parts[16]
		g.TempMemC = f64pOrNull(parts[17])
		g.ECCCorrected = intFromStr(parts[18])
		g.ECCUncorrected = intFromStr(parts[19])
		g.Throttle = parseThrottle(parts[20])
		g.UUID = parts[21]
		gpus = append(gpus, g)
	}

	// 5. Compute-apps query — best effort (don't fail GPU set if this fails).
	appsCmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-compute-apps=gpu_uuid,pid,process_name,used_memory",
		"--format=csv,noheader,nounits")
	appsOut, appsErr := appsCmd.CombinedOutput()
	if appsErr == nil {
		if appsByUUID := parseGPUApps(string(appsOut)); len(appsByUUID) > 0 {
			for i := range gpus {
				if apps, ok := appsByUUID[gpus[i].UUID]; ok {
					gpus[i].Apps = apps
				}
			}
			if apps, ok := appsByUUID[""]; ok && len(gpus) > 0 {
				gpus[0].Apps = apps
			}
		}
	}

	return gpus
}

// parseGPUApps 解析 nvidia-smi --query-compute-apps 输出（gpu_uuid,pid,process_name,used_memory）。
// 返回 uuid → 该卡进程列表（逐卡归属；同一 PID 每卡一行，显存为该卡占用）。
// 注意：CSV 逗号后带空格（" 17920"），数值解析必须容忍前导空格（atoi/atof 内部 trim）；
// 同卡同 PID 多行（多 CUDA 上下文）按 PID 求和；旧驱动无 gpu_uuid（3 字段）时 uuid 为空，
// 由调用方回退挂第一张卡。
func parseGPUApps(section string) map[string][]model.GPUApp {
	type acc struct {
		name string
		mem  int
	}
	byUUID := make(map[string]map[int]*acc)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		var uuid, pidStr, memStr string
		var nameParts []string
		switch {
		case len(parts) >= 4 && strings.HasPrefix(strings.TrimSpace(parts[0]), "GPU-"):
			uuid = strings.TrimSpace(parts[0])
			pidStr = parts[1]
			nameParts = parts[2 : len(parts)-1]
			memStr = parts[len(parts)-1]
		case len(parts) >= 3:
			pidStr = parts[0]
			nameParts = parts[1 : len(parts)-1]
			memStr = parts[len(parts)-1]
		default:
			continue
		}
		pid := atoi(pidStr)
		mem := atoi(memStr)
		name := strings.TrimSpace(strings.Join(nameParts, ","))
		if name != "" {
			name = name[strings.LastIndex(name, "/")+1:]
		}
		m, ok := byUUID[uuid]
		if !ok {
			m = make(map[int]*acc)
			byUUID[uuid] = m
		}
		a, ok := m[pid]
		if !ok {
			a = &acc{name: name}
			m[pid] = a
		}
		a.mem += mem
	}
	out := make(map[string][]model.GPUApp, len(byUUID))
	for uuid, m := range byUUID {
		apps := make([]model.GPUApp, 0, len(m))
		for pid, a := range m {
			apps = append(apps, model.GPUApp{PID: pid, Name: a.name, MemMB: a.mem})
		}
		sort.Slice(apps, func(i, j int) bool { return apps[i].PID < apps[j].PID })
		out[uuid] = apps
	}
	return out
}

func parseThrottle(raw string) int {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(strings.ToUpper(s), "NOT") {
		return 0
	}
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		n, _ := strconv.ParseInt(s[2:], 16, 64)
		return int(n)
	}
	n, _ := strconv.Atoi(s)
	return n
}

// ---------- GPU（AMD: rocm-smi / amd-smi） ----------

func (h *HostCollector) readGPUsAMD(ctx context.Context) []model.GPU {
	// Check for rocm-smi or amd-smi (prefer rocm-smi).
	var toolPath string
	for _, tool := range []string{"rocm-smi", "amd-smi"} {
		if p, err := exec.LookPath(tool); err == nil {
			toolPath = p
			break
		}
	}
	if toolPath == "" {
		return nil
	}

	cmd := exec.CommandContext(ctx, toolPath, "--showallinfo", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[collect] %s: %v (output: %s)", toolPath, err, strings.TrimSpace(string(out)))
		return nil
	}
	return parseAMDJSON(string(out))
}

// parseAMDJSON parses rocm-smi --showallinfo --json (or amd-smi --json) output.
// Returns whatever data can be extracted; never panics on unexpected structure.
func parseAMDJSON(raw string) []model.GPU {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil
	}

	// rocm-smi JSON: top-level keys include "rocm_smi_version" and GPU data
	// under keys like "GPU" or a "gpus" array. amd-smi uses "gpu" array.
	var gpuEntries []map[string]interface{}

	// Try rocm-smi format: look for "GPUTOP" or "gpu" or iterate top-level keys
	if v, ok := root["GPUTOP"]; ok {
		if arr, ok := v.([]interface{}); ok {
			gpuEntries = make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					gpuEntries = append(gpuEntries, m)
				}
			}
		}
	}
	if len(gpuEntries) == 0 {
		// Try "gpu" key (lowercase, some rocm-smi versions)
		if v, ok := root["gpu"]; ok {
			if arr, ok := v.([]interface{}); ok {
				gpuEntries = make([]map[string]interface{}, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						gpuEntries = append(gpuEntries, m)
					}
				}
			}
		}
	}
	if len(gpuEntries) == 0 {
		// Try "gpus" key (amd-smi format)
		if v, ok := root["gpus"]; ok {
			if arr, ok := v.([]interface{}); ok {
				gpuEntries = make([]map[string]interface{}, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						gpuEntries = append(gpuEntries, m)
					}
				}
			}
		}
	}
	if len(gpuEntries) == 0 {
		// Fallback: iterate top-level keys looking for GPU data
		for key, val := range root {
			if key == "rocm_smi_version" || key == "amdsmi_version" {
				continue
			}
			if arr, ok := val.([]interface{}); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						gpuEntries = append(gpuEntries, m)
					}
				}
			}
		}
	}
	if len(gpuEntries) == 0 {
		return nil
	}

	var gpus []model.GPU
	for _, entry := range gpuEntries {
		gpu := mapEntryToGPU(entry)
		if gpu.Name != "" { // Only include GPUs with a recognizable name
			gpus = append(gpus, gpu)
		}
	}
	return gpus
}

// mapEntryToGPU maps a single rocm-smi JSON GPU entry to model.GPU.
func mapEntryToGPU(entry map[string]interface{}) model.GPU {
	var g model.GPU
	g.Index = -1 // sentinel; will be set if available

	// gpu_id
	if v, ok := entry["gpu_id"]; ok {
		g.Index = toInt(v)
	}

	// Identification: PCI address as UUID proxy
	if v, ok := entry["drm_sysfs_class_drm"]; ok {
		g.UUID = toString(v)
	}
	if g.UUID == "" {
		// Try pci_address
		if v, ok := entry["pci_identifier"]; ok {
			g.UUID = toString(v)
		}
	}
	if g.UUID == "" {
		// Try product_name as fallback identifier
		if v, ok := entry["product_name"]; ok {
			g.UUID = toString(v)
		}
	}

	// Name
	if v, ok := entry["drm_device_name"]; ok {
		g.Name = toString(v)
	}
	if g.Name == "" {
		if v, ok := entry["product_name"]; ok {
			g.Name = toString(v)
		}
	}

	// Driver
	if v, ok := entry["hardware.hwdriverversion"]; ok {
		g.Driver = toString(v)
	}
	if g.Driver == "" {
		if v, ok := entry["driver_version"]; ok {
			g.Driver = toString(v)
		}
	}

	// Memory (rocm-smi uses KB)
	if hw, ok := entry["hardware"].(map[string]interface{}); ok {
		if v, ok := hw["hwmemtotal"]; ok {
			g.MemTotalMB = toIntKB(v)
		}
		if v, ok := hw["hwmemused"]; ok {
			g.MemUsedMB = toIntKB(v)
		}
	}
	// Fallback: top-level mem fields (some rocm-smi versions)
	if g.MemTotalMB == 0 {
		if v, ok := entry["mem_total"]; ok {
			g.MemTotalMB = toIntKB(v)
		}
	}
	if g.MemUsedMB == 0 {
		if v, ok := entry["mem_used"]; ok {
			g.MemUsedMB = toIntKB(v)
		}
	}
	g.MemFreeMB = g.MemTotalMB - g.MemUsedMB

	// Utilization
	if act, ok := entry["gpu_activity_percent"].(map[string]interface{}); ok {
		if v, ok := act["gpu"]; ok {
			g.UtilPct = toFloat(v)
		}
	}
	// Fallback: top-level gpu_activity_percent
	if g.UtilPct == 0 {
		if v, ok := entry["gpu_activity_percent"]; ok {
			g.UtilPct = toFloat(v)
		}
	}

	// Temperature
	if temp, ok := entry["temperature"].(map[string]interface{}); ok {
		if v, ok := temp["sensortemp"]; ok {
			g.TempC = f64p(toFloat(v))
		}
	}
	// Fallback: top-level temperature
	if g.TempC == nil {
		if v, ok := entry["temperature"]; ok {
			g.TempC = f64p(toFloat(v))
		}
	}

	// Power (rocm-smi uses mW)
	if pwr, ok := entry["power"].(map[string]interface{}); ok {
		if mc, ok := pwr["manchip"].(map[string]interface{}); ok {
			if v, ok := mc["totalpower"]; ok {
				g.PowerW = f64p(toFloatMW(v))
			}
			if v, ok := mc["pptlimit"]; ok {
				g.PowerLimitW = f64p(toFloatMW(v))
			}
		}
	}
	// Fallback: top-level power fields
	if g.PowerW == nil {
		if v, ok := entry["power_draw"]; ok {
			g.PowerW = f64p(toFloatMW(v))
		}
	}
	if g.PowerLimitW == nil {
		if v, ok := entry["power_limit"]; ok {
			g.PowerLimitW = f64p(toFloatMW(v))
		}
	}

	// Fan speed
	if fan, ok := entry["fan_speed"].([]interface{}); ok && len(fan) > 0 {
		g.FanPct = f64p(toFloat(fan[0]))
	} else if fan, ok := entry["fan_speed"].(float64); ok {
		g.FanPct = f64p(fan)
	} else if fan, ok := entry["fan_speed"].(string); ok {
		g.FanPct = f64p(toFloat(fan))
	}

	// Clock speeds
	if clocks, ok := entry["clocks_current_clk"].(map[string]interface{}); ok {
		if v, ok := clocks["gfxclk"]; ok {
			g.ClockMHz = intPtr(toInt(v))
		}
		if g.ClockMHz == nil {
			if v, ok := clocks["coreclk"]; ok {
				g.ClockMHz = intPtr(toInt(v))
			}
		}
	}
	// Fallback: top-level sclk or gfxclk
	if g.ClockMHz == nil {
		if v, ok := entry["sclk"]; ok {
			g.ClockMHz = intPtr(toInt(v))
		}
	}
	if g.ClockMHz == nil {
		if v, ok := entry["gfxclk"]; ok {
			g.ClockMHz = intPtr(toInt(v))
		}
	}
	// Memory clock
	if clocks, ok := entry["clocks_current_clk"].(map[string]interface{}); ok {
		if v, ok := clocks["memclk"]; ok {
			g.MemClockMHz = intPtr(toInt(v))
		}
	}
	if g.MemClockMHz == nil {
		if v, ok := entry["memclk"]; ok {
			g.MemClockMHz = intPtr(toInt(v))
		}
	}

	// PCIe link
	if pcie, ok := entry["pcielink"].(map[string]interface{}); ok {
		if link, ok := pcie["linkgen"].(map[string]interface{}); ok {
			if v, ok := link["current"]; ok {
				g.PCIEGen = intPtr(toInt(v))
			}
		}
		if link, ok := pcie["linkwidth"].(map[string]interface{}); ok {
			if v, ok := link["current"]; ok {
				g.PCIEWidth = intPtr(toInt(v))
			}
		}
	}
	// Fallback: top-level pcie fields
	if g.PCIEGen == nil {
		if v, ok := entry["pcie_gen"]; ok {
			g.PCIEGen = intPtr(toInt(v))
		}
	}
	if g.PCIEWidth == nil {
		if v, ok := entry["pcie_width"]; ok {
			g.PCIEWidth = intPtr(toInt(v))
		}
	}

	// PState
	if v, ok := entry["pstate"]; ok {
		g.PState = toString(v)
	}
	if g.PState == "" {
		if v, ok := entry["power_state"]; ok {
			g.PState = toString(v)
		}
	}

	// Apps: not tracked for AMD (leave nil)
	// CUDA: not applicable for AMD
	// TempMemC, ECC: not available from rocm-smi

	// Throttle
	if throttle, ok := entry["throttle_reason"].(string); ok {
		g.Throttle = parseThrottle(throttle)
	} else if throttle, ok := entry["throttle_reasons"]; ok {
		g.Throttle = parseThrottle(toString(throttle))
	}

	return g
}

// ---------- 命令行参数解析（与面板 llama_flags.parse_cmdline 一致） ----------

var flagMap = map[string][2]string{
	"-m":      {"model", "0"},
	"--model": {"model", "0"},
	"--mmproj": {"mmproj", "0"},
	"-ngl":    {"n_gpu_layers", "0"},
	"--n-gpu-layers": {"n_gpu_layers", "0"},
	"--flash-attn": {"flash_attn", "0"},
	"-ts":     {"tensor_split", "0"},
	"--tensor-split": {"tensor_split", "0"},
	"-b":      {"batch", "0"},
	"--batch": {"batch", "0"},
	"-ub":     {"ubatch", "0"},
	"--ubatch": {"ubatch", "0"},
	"-np":     {"np", "0"},
	"--parallel": {"np", "0"},
	"-c":      {"ctx_size", "0"},
	"--ctx-size": {"ctx_size", "0"},
	"--kv-offload": {"kv_offload", "1"},
	"--cache-type-k": {"cache_type_k", "0"},
	"--cache-type-v": {"cache_type_v", "0"},
	"--fit":   {"fit", "0"},
	"-t":      {"threads", "0"},
	"--threads": {"threads", "0"},
	"--threads-batch": {"threads_batch", "0"},
	"--threads-http": {"threads_http", "0"},
	"--temperature": {"temperature", "0"},
	"--top-p": {"top_p", "0"},
	"-tk":     {"top_k", "0"},
	"--top-k": {"top_k", "0"},
	"--spec-type": {"spec_type", "0"},
	"--spec-draft-n-max": {"spec_draft_n_max", "0"},
	"--port":  {"port", "0"},
	"--host":  {"host", "0"},
}

func parseCmdline(cmdline string) map[string]string {
	flags := make(map[string]string)
	if cmdline == "" {
		return flags
	}
	tokens := strings.Fields(cmdline)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		hit, ok := flagMap[tok]
		if !ok {
			continue
		}
		key, isBool := hit[0], hit[1]
		// 与 backend/llama_flags.py 语义一致：for 循环的 i++ 负责步进，
		// 这里只在消费了值 token 时额外 i++（旧版 i+=2 会多跳一个 token）
		if isBool == "1" {
			flags[key] = "true"
		} else if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
			flags[key] = tokens[i+1]
			i++
		} else {
			flags[key] = "true"
		}
	}
	return flags
}

// ---------- 小工具 ----------

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
func atoi(s string) int { return int(atoi64(s)) }
func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
func f64p(f float64) *float64 { return &f }
func f64pOrNull(s string) *float64 {
	if s == "" || s == "[N/A]" || s == "[Not Supported]" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}
func intPtr(i int) *int { return &i }
func intFromStr(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == "[N/A]" || s == "[Not Supported]" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// ---------- AMD JSON helpers ----------

// toInt safely converts an interface{} to int.
func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		return atoi(val)
	}
	return 0
}

// toFloat safely converts an interface{} to float64.
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	}
	return 0
}

// toFloatMW converts a value that may be in milliwatts to watts (divide by 1000).
func toFloatMW(v interface{}) float64 {
	return toFloat(v) / 1000
}

// toIntKB converts a value in kilobytes to megabytes (divide by 1024).
func toIntKB(v interface{}) int {
	return toInt(v) / 1024
}

// toString safely converts an interface{} to string.
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

func fmtSeconds(totalS float64) string {
	m := int(totalS) / 60
	s := totalS - float64(m)*60
	return fmt.Sprintf("%dmin %.3fs", m, s)
}

func fmtBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
