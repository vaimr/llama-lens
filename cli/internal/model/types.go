// Package model 定义 llamalens 的数据模型与共享状态。
// 字段与面板（backend/）快照结构一一对应，保证 CLI 与 Web 面板展示同源。
package model

// ---------- llama-server（HTTP API） ----------

type ModelInfo struct {
	Path         string
	Name         string
	Ftype        string
	NEmbD        *int
	NVocab       *int
	NParams      *float64
	NCtx         *int
	NCtxTrain    *int
	VocabType    string
	OwnedBy      string
	Size         *float64 // 文件字节数（/v1/models size）
	FileSize     *float64 // 文件字节数（ls -l，优先）
	Modalities   []string
	Capabilities []string
	MMProjPath   string
	MMProjSize   *float64
}

type Slot struct {
	ID                     int
	IDTask                 *int
	IsProcessing           bool
	State                  string
	NDecoded               *int
	NPromptTokens          int
	NPromptTokensProcessed int
	NRemain                *int
	GenTPS                 float64
	PromptTPS              float64
}

type LlamaAPI struct {
	Online    bool
	Model     ModelInfo
	Slots     []Slot
	CtxUsed   *int
	CtxTotal  int
	GenTPS    float64 // /slots 差分兜底速度
	PromptTPS float64
	FailCount int
}

// ---------- 日志状态机（llama-server 日志解析） ----------

type Ctx struct {
	Used      *int
	Total     *int
	Pct       *float64
	Remaining *int
	Truncated bool
}

type MTP struct {
	Acceptance *float64
	Accepted   *int
	Generated  *int
	MeanLen    *float64
}

type KV struct {
	Selection string
	FSimBest  *float64
	FKeep     *float64
}

type BootInfo struct {
	Model          string
	NSlots         *int
	NCtxSlot       *int
	KVUnified      bool
	MTPDraft       bool
	KVCacheUpgrade string
	Verbosity      *int
	Listening      string
	Warnings       []string
}

type TaskSummary struct {
	TaskID         *int
	PromptMS       *float64
	PromptTokens   *int
	PromptSpeedTPS *float64
	EvalMS         *float64
	DecodedTokens  *int
	GenSpeedTPS    *float64
	TotalMS        *float64
	TotalTokens    *int
	GraphsReused   *int
	MTP            *MTP
	CtxUsed        int
	Truncated      bool
}

type PrefillInfo struct {
	Speed    *float64
	TS       float64
	Progress *float64
	NTokens  *int
}

type LogState struct {
	Available         bool
	Phase             string // idle | prompt_processing | decoding
	TaskID            *int
	NDecoded          int
	TgTPS             *float64
	Tg3sTPS           *float64
	PromptProgress    *float64
	PromptSpeedTPS    *float64
	PromptTotalTokens *int
	PromptElapsedS    *float64
	IsChild           *int
	StartedAt         *float64
	Context           Ctx
	MTP               MTP
	KV                KV
	GraphsReused      *int
	Boot              BootInfo
	LastTask          *TaskSummary
	LastPrefill       *PrefillInfo
}

// ---------- 主机指标（/proc + nvidia-smi + systemctl） ----------

type SysInfo struct {
	Hostname string
	Kernel   string
	OS       string
	CPUModel string
	Cores    int
	UptimeS  *float64
	Procs    int
}

type CPUInfo struct {
	UsagePct   *float64
	PerCorePct []float64
	Load       [3]float64
}

type MemInfo struct {
	TotalMB     int
	UsedMB      int
	FreeMB      int
	BuffCacheMB int
	AvailableMB int
	SwapTotalMB int
	SwapUsedMB  int
}

type Mount struct {
	Mount   string
	SizeGB  float64
	UsedGB  float64
	AvailGB float64
	UsePct  float64
}

type DiskInfo struct {
	Mounts   []Mount
	ReadMBs  float64
	WriteMBs float64
}

type Iface struct {
	Name      string
	RxMBs     float64
	TxMBs     float64
	RxTotalMB float64
	TxTotalMB float64
}

type NetInfo struct {
	Ifaces []Iface
}

type ProcInfo struct {
	Found          bool
	Name           string // 匹配的进程名（展示用）
	PID            int
	CPUPctRealtime *float64
	CPUPctLifetime *float64
	MemPct         *float64
	RSSMB          *int
	VSZMB          *int
	Threads        *int
	Elapsed        string
	Cmdline        string
	Flags          map[string]string
}

type ServiceInfo struct {
	Unit        string
	Description string
	Active      string
	Since       string
	CPUTotal    string
	Memory      string
	MemoryPeak  string
	Tasks       int
}

type GPUApp struct {
	PID   int
	Name  string
	MemMB int
}

type GPU struct {
	Index          int
	UUID           string
	Name           string
	Driver         string
	MemTotalMB     int
	MemUsedMB      int
	MemFreeMB      int
	UtilPct        float64
	MemUtilPct     float64
	TempC          *float64
	PowerW         *float64
	PowerLimitW    *float64
	FanPct         *float64
	ClockMHz       *int
	MemClockMHz    *int
	PCIEGen        *int
	PCIEWidth      *int
	PState         string
	TempMemC       *float64
	ECCCorrected   *int
	ECCUncorrected *int
	Throttle       int
	Apps           []GPUApp
	CUDA           string
}

type TopProc struct {
	PID    int
	Name   string
	CPUPct float64
	MemPct float64
	RSSMB  int
}

type HostMetrics struct {
	Reachable bool
	Sys       SysInfo
	CPU       CPUInfo
	Mem       MemInfo
	Disk      DiskInfo
	Net       NetInfo
	Process   *ProcInfo
	Service   ServiceInfo
	GPUs      []GPU
	TopCPU    []TopProc
	TopMem    []TopProc
}

// ---------- 事件 / 告警 ----------

type Event struct {
	TS    float64
	Level string // info | warn | danger
	Type  string
	Msg   string
}

type Alert struct {
	Metric    string
	Level     string // warn | danger
	Value     float64
	Threshold float64
}

// ---------- 快照（UI 读取的不可变视图） ----------

type Snapshot struct {
	TS          float64
	HostName    string
	Llama       LlamaAPI
	Log         LogState
	Host        HostMetrics
	SpeedSource string // log | api
	Events      []Event
	Alerts      []Alert
}
