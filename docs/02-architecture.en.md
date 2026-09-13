# llama-lens Architecture Design Document

| Item | Content |
|---|---|
| Document Version | v1.0 |
| Date | 2026-08-28 |
| Status | Design Phase |
| Upstream Document | 01-RequirementsDocument.md (requirements baseline) |

## 1. Overall Architecture

### 1.1 Architecture Diagram

```
                    config/hosts.yaml (N hosts, gitignore)
                              │
┌─────────────────────────────▼──────────────────────────────┐
│  FastAPI Single Process :8000 (local deployment)            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ MonitorRegistry                                       │  │
│  │  ├─ HostMonitor[ai]     ├─ HostMonitor[host2] ...    │  │
│  │  │   ├─ LlamaPoller 1s  │   (per-host independent:    │  │
│  │  │   ├─ SshPoller   2s  │    collection + buffer +    │  │
│  │  │   ├─ LogPoller (journal stream)                   │  │
│  │  │   ├─ RingBuffer (3600@1s + 1800@2s)               │  │
│  │  │   └─ EventDetector (task start/end + up/down)     │  │
│  └──────────────────────────────────────────────────────┘  │
│  REST: /api/hosts · /api/hosts/{id}/overview|history|events │
│  WS:   /ws/hosts/{id} (1s tick push, configurable)          │
│  Static: frontend/dist (Vue3 build output)                  │
└─────────────────────────────▲──────────────────────────────┘
                               │ WS 1s / HTTP fallback
               ┌───────────────┴───────────────┐
               │  Browser (Vue3 + ECharts)      │
               │  /            Host Portal (card wall)  │
               │  /host/:id  Single-host detail dashboard│
               └───────────────────────────────┘

Data Sources (per host): llama HTTP API (8080) + SSH read-only batch commands (22)
```

### 1.2 Layer Responsibilities

| Layer | Component | Responsibility |
|---|---|---|
| Configuration Layer | hosts.yaml + .env | Host registration: address/credentials/collection parameters/thresholds |
| Collection Layer | LlamaPoller / SshPoller / LogPoller | Per-host periodic collection + log stream, all read-only |
| Computation Layer | SpeedCalculator / DiffEngine | Speed, CPU, disk, network differential calculation |
| Storage Layer | RingBuffer | Memory ring buffer (llama 3600@1s, host 1800@2s) |
| Event Layer | EventDetector | State transition detection → event stream |
| Service Layer | FastAPI | REST + WS + static hosting |
| Presentation Layer | Vue3 SPA | Portal page + detail page |

### 1.3 Data Flow

```
llama-server :8080 ──HTTP(1s)──▶ LlamaPoller ─┐
                                               ├─▶ SpeedCalculator ─▶ RingBuffer ─▶ REST/WS ─▶ Browser
host :22 ──SSH(2s batch)──▶ SshPoller ─┘         │
                                                 └─▶ EventDetector ─▶ EventRing ─▶ /events, WS
```

## 2. Technology Selection

| Item | Choice | Version | Rationale |
|---|---|---|---|
| Backend Framework | FastAPI + uvicorn | 0.115.6 / 0.34.0 | Project rules tech stack; async support; built-in WS; pre-installed locally |
| SSH Client | paramiko | 2.7.2 | Pre-installed locally; persistent connections + keepalive; no sshpass needed |
| HTTP Client | httpx | latest | Async requests to llama API (aiohttp also pre-installed, either works) |
| Frontend Framework | Vue 3 + Vite | 3.x / 5.x | Project rules tech stack |
| Charts | ECharts | 5.x | Gauges/multi-series/stacked/dark theme; good performance |
| Routing | Vue Router | 4.x | Two-level pages |
| Python | 3.9 compatible | 3.9.2 | Actual Python version locally (not 3.11), avoid 3.10+ syntax |
| Node | Node 24 | v24.11.1 | Actual version locally, Vite compatible |
| Deployment | Single process direct run (run.sh) | — | FastAPI hosts frontend dist; simplest approach |

## 3. Multi-Host Design

### 3.1 Core Abstraction

```python
class HostMonitor:
    """One independent monitoring unit per host"""
    def __init__(self, cfg: HostConfig): ...
    def start(self): ...      # Start LlamaPoller + SshPoller + push tasks
    def stop(self): ...
    def snapshot(self) -> dict: ...   # Current snapshot (including alerts)
    def history(self, window_s: int) -> dict: ...
    def events(self, limit: int) -> list: ...

class MonitorRegistry:
    """Manage all HostMonitors"""
    def load(self, path: str): ...    # Parse hosts.yaml
    def get(self, host_id: str) -> HostMonitor: ...
    def list(self) -> list: ...       # Portal page data (per-host summaries)
```

- Each HostMonitor runs 3 independent async tasks internally: LlamaPoller, SshPoller, WsPusher
- Exceptions from any task are caught and logged, without affecting other tasks or other hosts
- Registry loads all hosts at startup; config changes require restart (hot reload not in scope)

### 3.2 hosts.yaml Configuration (Complete Example)

```yaml
# config/hosts.yaml (gitignore; template at hosts.example.yaml)
global:
  push_interval: 1.0        # WS push interval (seconds)
  history:
    llama_points: 3600      # llama metric ring buffer points @1s (1h)
    host_points: 1800       # host metric ring buffer points @2s (1h)
  thresholds:               # Global default thresholds
    gpu_util:   { warn: 80, danger: 90 }
    gpu_mem:    { warn: 85, danger: 95 }
    gpu_temp:   { warn: 75, danger: 85 }
    gpu_power:  { warn: 85, danger: 95 }   # Percentage of power limit
    cpu:        { warn: 80, danger: 90 }
    mem:        { warn: 85, danger: 95 }
    disk:       { warn: 80, danger: 90 }

hosts:
  - id: ai
    name: "AI Host (ai.lan)"
    llama:
      host: ai.lan
      port: 8080
      interval: 1.0         # /health + /slots polling interval (seconds)
      slow_interval: 30.0   # /props + /v1/models polling interval (seconds)
    ssh:
      host: ai.lan
      port: 22
      user: root
      password: ${AI_SSH_PASS}   # Read from environment variable .env, no plaintext
      interval: 2.0         # Batch collection interval (seconds)
      keepalive: 15         # SSH keepalive (seconds)
    process:
      name: llama-server    # Process monitoring target process name
    log:
      unit: llama-server    # systemd unit name (journalctl -u)
      follow: true          # Stream follow (false = periodic --since pull)
      catchup_sec: 30       # Seconds to catch up after reconnect
    systemd_unit: llama-server.service
    thresholds: {}          # Per-host override (optional, same structure as global)
```

### 3.3 Adding a New Host

1. Add an entry to hosts.yaml (id must be unique)
2. Add corresponding SSH password to .env (if using environment variable reference)
3. Restart the panel → new card appears on the portal page automatically

No code changes required.

## 4. Collection Layer Design

### 4.1 LlamaPoller (1s)

Endpoints and frequencies:

| Endpoint | Frequency | Purpose |
|---|---|---|
| GET /health | 1s | Online detection |
| GET /slots | 1s | Slot status + speed differential input |
| GET /props | 30s | Model info, default parameters, modalities |
| GET /v1/models | 30s | Parameter count / embedding dims / vocab / file size |

Speed differential algorithm (per slot):

```
state: prev_task_id, prev_decoded, prev_prompt_processed, prev_ts, task_start_ts

on_poll(slot):
  now = time(); dt = now - prev_ts
  if slot.is_processing:
    if prev_task_id is None:
        task_start_ts = now                      # Task start event
    if slot.id_task != prev_task_id or slot.n_decoded < prev_decoded:
        # New task (or task boundary): reset baseline, this cycle speed = 0
        prev_task_id = slot.id_task
        prev_decoded = slot.n_decoded
        prev_prompt_processed = slot.n_prompt_tokens_processed
        gen_speed = 0; prompt_speed = 0
    else:
        gen_speed    = (slot.n_decoded - prev_decoded) / dt
        prompt_speed = (slot.n_prompt_tokens_processed - prev_prompt_processed) / dt
        prev_decoded = slot.n_decoded
        prev_prompt_processed = slot.n_prompt_tokens_processed
  else:
    if prev_task_id is not None:
        # Task end event: total tokens = last n_decoded, duration = now - task_start_ts
        emit(task_end, total_tokens, duration, avg_tps)
        prev_task_id = None
    gen_speed = 0; prompt_speed = 0
  prev_ts = now
```

- Task start event: is_processing false→true → record task_start (task ID, prompt token count)
- Model change: /props.model_path changes → model_change event
- Failure handling: request timeout 3s; 3 consecutive failures → online=false + llama_down event; recovery → llama_up event; retain last data during offline period

### 4.2 SshPoller (2s)

Connection management:
- paramiko SSHClient, AutoAddPolicy (auto-trust fingerprint on first connection and log it)
- transport.set_keepalive(15)
- Disconnection: exponential backoff reconnect 1s→2s→4s→...→30s cap; ssh_down event on disconnect, ssh_up event on successful reconnect
- Static info (collected once on first connection): hostname, kernel, OS version (/etc/os-release), CPU model, core count

Per-cycle batch commands (1 exec, one network round-trip, all read-only):

```bash
echo ==GPU==
nvidia-smi --query-gpu=index,name,driver_version,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,power.limit,fan.speed,clocks.current.graphics,clocks.current.memory,pcie.link.gen.current,pcie.link.width.current,pstate,temperature.memory,ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total,clocks_throttle_reasons.active,cuda_version --format=csv,noheader,nounits
echo ==APPS==
nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits
echo ==STAT==
cat /proc/stat
echo ==MEM==
grep -E '^(MemTotal|MemFree|MemAvailable|Buffers|^Cached|SwapTotal|SwapFree)' /proc/meminfo
echo ==LOAD==
cat /proc/loadavg
echo ==UPTIME==
cat /proc/uptime
echo ==NET==
cat /proc/net/dev
echo ==DISKIO==
grep -E '^(sd|nvme|vd|dm)' /proc/diskstats
echo ==DF==
df -B1 --output=target,size,used,avail,pcent / /share 2>/dev/null
echo ==PROC==
PID=$(pgrep -x llama-server | head -1)
if [ -n "$PID" ]; then
  echo P:$PID
  awk '{print $14, $15, $23}' /proc/$PID/stat    # utime stime vsize
  grep -E '^(VmRSS|VmSize|Threads)' /proc/$PID/status
  ps -o pcpu=,pmem=,etime= -p $PID
  tr '\0' ' ' < /proc/$PID/cmdline; echo
fi
echo ==PS==
ps -eo pid,comm,pcpu,pmem,rss --no-headers 2>/dev/null
echo ==PSTICKS==
awk 'FNR==1 { n=split(FILENAME, p, "/"); pid=p[n-1]; i=index($0, ") "); if (i > 0) { s=substr($0, i+2); m=split(s, a, " "); if (m >= 13) print pid, a[12], a[13] } }' /proc/[0-9]*/stat 2>/dev/null
echo ==PROCS==
ls /proc | grep -c '^[0-9]'
```

Differential calculation (backend DiffEngine, based on two consecutive samples):
- Whole CPU% = 1 - Δidle/Δtotal (cpu summary row)
- Per-core CPU% = 1 - Δidle_i/Δtotal_i (cpu0..cpu5 rows)
- Process real-time CPU% = Δ(utime+stime)/Δt/CLK_TCK (per single core relative)
  - Process stuck: /proc/<pid>/stat fields 14/15 (clock ticks, 10ms precision)
  - Top list: ==PSTICKS== section (single awk reads utime/stime for all /proc/<pid>/stat, 10ms precision, correlated with PID from ==PS== section)
  - Note: procps stime column is start time, not system CPU time; utime column unavailable (outputs -); time column only 1s precision, 2s sampling window would quantize CPU% into 0/50/100/150 step values (all 0 when idle looks like "fake data"), so must read /proc directly
- Top CPU sort: real-time CPU% → cumulative CPU time → RSS; Top MEM sort: RSS → real-time CPU% (top 8 each, backend-sorted)
- Disk read/write rate = Δsectors×512/Δt
- Network rx/tx rate = Δbytes/Δt
- First sample has no baseline → differential metrics are null this cycle

### 4.3 EventDetector

State machine (per host):

```
llama: online ⇄ offline   (3 consecutive /health failures → offline)
ssh:   connected ⇄ disconnected
task:  idle → running (task_start) → idle (task_end, with stats)
model: props.model_path changes → model_change
prefill: first prompt line per task → prefill_start (with total prompt token count)
alert:   check_alerts(alerts) tracks level changes per metric (normal/warn/danger), emits event on change (minimum 30s between two events, first observation only records baseline)
```

Event ring: 200 entries, fields {ts, level, type, msg}
- task_start/task_end prefer log lines (launch_slot_ / release) as source, with full stats (MTP acceptance rate, context usage, duration)
- boot event: log "model loaded" line → llama_boot (with startup info)
- prefill_start: LogPoller emits on first prompt line per task (deduplicated by task ID, reset on task end/restart)
- alert: HostMonitor calls check_alerts after each snapshot's evaluate_alerts; llama/ssh not duplicated (already have dedicated up/down events)

### 4.4 LogPoller (Log Stream)

Data source: `journalctl -u {log.unit}` (systemd journal, read-only)

Collection method:
- Dedicated SSH channel (same transport as SshPoller, independent channel) runs `journalctl -u llama-server -f -o short-iso --no-pager -n 0` for stream follow
- Channel disconnect: reconnect + first `--since "N seconds ago"` (catchup_sec, default 30s) to catch up, prevent line loss
- follow=false (fallback mode): periodic (2s) `--since "@<last_ts>"` pull new lines
- First connection: pull last startup block (200 lines before "listening") to parse boot info

Log line format (short-iso):

```
2026-08-28T18:11:25+08:00 ai llama-server[55646]: 50.44.267.087 I slot      release: id  0 | task 14847 | stop processing: n_tokens = 201047, truncated = 0
└─ syslog time ─────────────────────┘ └host┘ └unit[pid]┘  └─ llama.cpp internal time ──┘ └L┘ └subsys┘ └function┘ └─ message ─┘
```

Parsing rules (regex, matched in order):

| # | Pattern (simplified) | Event | Extracted Fields |
|---|---|---|---|
| 1 | `slot print_timing: id (\d+) \| task (\d+) \| n_decoded = (\d+), tg = ([\d.]+) t/s, tg_3s = ([\d.]+) t/s` | decode_tick | task_id, n_decoded, tg, tg_3s |
| 2 | `slot print_timing: id (\d+) \| task (\d+) \| prompt processing, n_tokens = (\d+), progress = ([\d.]+), t = ([\d.]+) s / ([\d.]+) tokens per second` | prompt_tick | task_id, n_tokens, progress, elapsed_s, speed_tps |
| 3 | `slot print_timing: ... prompt eval time = ([\d.]+) ms / (\d+) tokens \(([\d.]+) ms per token, ([\d.]+) tokens per second\)` | task_summary.prompt | prompt_ms, prompt_tokens, prompt_speed_tps |
| 4 | `slot print_timing: ... eval time = ([\d.]+) ms / (\d+) tokens \(([\d.]+) ms per token, ([\d.]+) tokens per second\)` | task_summary.eval | eval_ms, decoded_tokens, gen_speed_tps |
| 5 | `slot print_timing: ... total time = ([\d.]+) ms / (\d+) tokens` | task_summary.total | total_ms, total_tokens |
| 6 | `slot print_timing: ... graphs reused = (\d+)` | task_summary.graphs | graphs_reused |
| 7 | `slot print_timing: ... draft acceptance = ([\d.]+) \((\d+) accepted / (\d+) generated\), mean len = ([\d.]+)` | task_summary.mtp | acceptance, accepted, generated, mean_len |
| 8 | `slot release: id (\d+) \| task (\d+) \| stop processing: n_tokens = (\d+), truncated = (\d+)` | task_end | ctx_used, truncated |
| 9 | `slot get_availabl: id (\d+) \| task -1 \| selected slot by (LRU\|LCP similarity)(?:, f_sim_best = ([\d.]+).*)?(?:, f_keep = ([\d.]+))?` | slot_select | method, f_sim_best, f_keep |
| 10 | `slot launch_slot_: id (\d+) \| task (\d+) \| processing task, is_child = (\d+)` | task_start | task_id, is_child |
| 11 | Startup lines: `loading model '(.+)'` / `n_slots = (\d+), n_ctx_slot = (\d+), kv_unified = '(\w+)'` / `creating MTP draft context` / `upgrading K from (\S+) to (\S+)` / `verbosity = (\d+)` / `listening on (\S+)` / `W` level lines | boot_info | model, n_slots, n_ctx_slot, kv_unified, mtp_draft, kv_upgrade, verbosity, listening, warnings[] |

State machine (current state, updated per line):

```
phase: idle → prompt_processing (prompt_tick) → decoding (decode_tick) → idle (task_end)
current: {phase, task_id, n_decoded, tg, tg_3s, prompt_progress, prompt_speed, prompt_total_tokens, prompt_elapsed_s, is_child, started_at}
started_at: task start timestamp (written at launch line, cleared at task_end; frontend calculates "running" duration)
last_prefill: {speed, ts, progress, n_tokens}   # Updated per prompt line, cleared on llama restart
context: {used (n_tokens from latest task_end), total (boot n_ctx_slot), pct, remaining, truncated}
mtp:     {acceptance, accepted, generated, mean_len}   # Updated at task_end
kv:      {f_keep, f_sim_best, selection}               # Updated at slot_select
boot:    {...}                                          # Updated at boot_info
```

Speed source priority:
1. Log tg_3s (3-second window, server-computed) — primary
2. /slots n_decoded differential (1s) — fallback when log unavailable
- Frontend displays data source annotation (log / API)

Failure handling:
- Channel disconnect → reconnect + catchup; during this period log_available=false, speed falls back to /slots differential
- Line parse failure → skip and count (parse_error counter), does not affect other lines
- llama-server restart (PID change) → reset state machine, reparse boot info

## 5. Data Model

### 5.1 Snapshot (GET /api/hosts/{id}/overview response)

```jsonc
{
  "ts": 1724000000.0,
  "host": { "id": "ai", "name": "AI Host (ai.lan)" },
  "llama": {
    "online": true,
    "model": {
      "name": "Qwen3.8-27B-Q6_K", "path": "/share/AI/LLM/unsloth/Qwen3.8-27B-Q6_K.gguf",
      "ftype": "Q6_K", "n_params": 27320697856, "n_embd": 5120, "n_vocab": 248320,
      "n_ctx": 262144, "n_ctx_train": 262144, "vocab_type": 2,
      "file_size": 22873411584,
      "mmproj_path": "/share/AI/LLM/unsloth/Qwen3.8-27B-mmproj-BF16.gguf",
      "mmproj_size": 889000000,
      "modalities": { "vision": true, "video": true, "audio": false },
      "capabilities": ["completion", "multimodal"], "owned_by": "llamacpp"
    },
    "gen_speed_tps": 42.3,
    "prompt_speed_tps": 123.4,
    "speed_source": "log",
    "log": {
      "available": true,
      "state": {
        "phase": "decoding",
        "task_id": 14847, "n_decoded": 2843, "is_child": 0,
        "tg_3s_tps": 28.46, "tg_tps": 26.21,
        "prompt_progress": null, "prompt_speed_tps": null,
        "prompt_total_tokens": null, "prompt_elapsed_s": null,
        "started_at": 1756361595.0
      },
      "last_prefill": { "speed": 516.98, "ts": 1756361595.0, "progress": 0.92, "n_tokens": 2048 },
      "context": { "used": 201047, "total": 262144, "pct": 76.7,
                   "remaining": 61097, "truncated": false },
      "mtp": { "acceptance": 0.698, "accepted": 1925, "generated": 2757,
               "mean_len": 3.09,
               "config": { "spec_type": "draft-mtp", "spec_draft_n_max": 3 } },
      "kv": { "f_keep": 1.0, "f_sim_best": 0.988, "selection": "LCP" },
      "graphs_reused": 15340,
      "boot": {
        "pid": 55646, "started_at": "2026-08-28T17:20:41",
        "verbosity": 3, "n_slots": 1, "n_ctx_slot": 262144, "kv_unified": false,
        "mtp_draft": true,
        "kv_cache_upgrade": "turbo3 -> q8_0 (K, auto-asymmetric GQA 6:1)",
        "listening": "http://0.0.0.0:8080",
        "warnings": ["CORS is set to allow all origins ('*') and no API key is set",
                     "NOTICE: server default port will be changed to :9931 in a future release"]
      },
      "last_task": {
        "task_id": 14847,
        "prompt_ms": "...", "prompt_tokens": "...", "prompt_speed_tps": "...",
        "eval_ms": 110069.09, "decoded_tokens": 2843, "gen_speed_tps": 25.83,
        "total_ms": 123991.42, "total_tokens": 7569,
        "graphs_reused": 15340,
        "mtp": { "acceptance": 0.698, "accepted": 1925, "generated": 2757, "mean_len": 3.09 },
        "ctx_used": 201047, "truncated": false
      }
    },
    "slots": [
      {
        "id": 0, "is_processing": true, "id_task": 5199, "n_ctx": 262144,
        "n_prompt_tokens": 19032, "n_prompt_tokens_processed": 626,
        "n_prompt_tokens_cache": 0, "n_decoded": 795, "n_remain": 31205,
        "ctx_used": 19827, "ctx_pct": 7.6,
        "gen_speed_tps": 42.3, "prompt_speed_tps": 123.4,
        "speculative": true,
        "params": { "temperature": 0.3, "top_k": 40, "top_p": 0.9, "max_tokens": 32000, "...": "..." }
      }
    ]
  },
  "host_metrics": {
    "reachable": true,
    "sys": { "hostname": "ai", "kernel": "6.8.0-124-generic", "os": "Ubuntu 24.04",
             "uptime_s": 28320, "procs": 293 },
    "cpu": { "model": "Intel Core i5-8500", "cores": 6, "usage_pct": 12.3,
             "per_core_pct": [15, 10, 12, 11, 13, 12],
             "load": [1.09, 0.75, 0.58], "mhz": 3000 },
    "mem": { "total_mb": 31744, "used_mb": 5939, "free_mb": 2765,
             "buff_cache_mb": 25600, "available_mb": 25600,
             "swap_total_mb": 8192, "swap_used_mb": 0 },
    "disk": { "mounts": [ { "mount": "/", "size_gb": 1234, "used_gb": 801,
                            "avail_gb": 347, "use_pct": 70 } ],
              "read_mb_s": 1.2, "write_mb_s": 0.3 },
    "net": { "ifaces": [ { "name": "eth0", "rx_mb_s": 0.5, "tx_mb_s": 0.2,
                           "rx_total_mb": 12345, "tx_total_mb": 678 } ] },
    "gpus": [
      { "index": 0, "name": "NVIDIA GeForce RTX 3080", "driver": "580.159.03",
        "util_pct": 0, "mem_util_pct": 0,
        "mem_total_mb": 20480, "mem_used_mb": 18459, "mem_free_mb": 2021,
        "temp_c": 54, "power_w": 141.5, "power_limit_w": 280,
        "fan_pct": null, "clock_mhz": 1440, "mem_clock_mhz": 1188,
        "pcie_gen": 3, "pcie_width": 16, "pstate": "P2",
        "temp_mem_c": null, "ecc_corrected": null, "ecc_uncorrected": null, "throttle": 0,
        "apps": [ { "pid": 31255, "name": "llama-server", "mem_mb": 18414 } ] }
    ],
    "process": {
      "found": true, "pid": 31255,
      "cpu_pct_realtime": 45.2, "cpu_pct_lifetime": 98.6, "mem_pct": 16.0,
      "rss_mb": 5153, "vsz_mb": 12000, "threads": 10, "elapsed": "02:28",
      "cmdline": "/home/wx/llama-cpp-turboquant-new/build/bin/llama-server -m ... --port 8080",
      "flags": { "model": "...", "mmproj": "...", "n_gpu_layers": "all",
                 "flash_attn": "enabled", "tensor_split": "24,20",
                 "batch": 1024, "ubatch": 512, "np": 1, "ctx_size": 262144,
                 "kv_offload": true, "cache_type_k": "turbo3", "cache_type_v": "turbo3",
                 "fit": "off", "threads": 6, "threads_batch": 6, "threads_http": 1,
                 "temperature": 0.3, "top_p": 0.9, "top_k": 40,
                 "spec_type": "draft-mtp", "spec_draft_n_max": 3,
                 "port": 8080, "host": "0.0.0.0" }
    },
    "service": {
      "unit": "llama-server.service",
      "description": "llama-server Qwen3.8-27B (TurboQuant)",
      "active": "active (running)", "since": "2026-08-28 15:53:10",
      "cpu_total": "9min 57.800s", "memory": "4.6G", "memory_peak": "4.6G", "tasks": 10
    },
    "top": {
      "cpu": [ { "pid": 31255, "name": "llama-server", "cpu_pct": 98.6,
                 "mem_pct": 16.0, "rss_mb": 5153 } ],
      "mem": [ "..." ]
    }
  },
  "alerts": [
    { "metric": "gpu0.util", "level": "danger", "value": 92.0, "threshold": 90 }
  ]
}
```

### 5.2 Ring Buffer

- llama sequence (1s): gen_speed (tg_3s), prompt_speed, ctx_used (1s sample, API real-time value preferred / log fallback; task-end point written separately by LogPoller, authoritative value)
- log sequence (per event): context.used (n_tokens, updated at task_end), mtp.acceptance (updated at task_end)
- host sequence (2s): cpu, per_core[], load_1/load_5/load_15, mem_used, mem_buff_cache, swap_used, per-GPU util/mem/temp/power, disk rw, net rx/tx, process cpu
- Implementation: collections.deque(maxlen=N) + ts array; history API slices by window and downsamples (take every kth point when point count > 600)

### 5.3 Events

- deque(maxlen=200), {ts, level, type, msg}

## 6. API Design

### 6.1 REST

| Method | Path | Description |
|---|---|---|
| GET | /api/health | Panel self-check: {status, hosts: {id: {llama_online, ssh_ok}}} |
| GET | /api/hosts | Portal page: [{id, name, online, model_name, n_params, gen_speed_tps, gpus: [{util_pct, mem_pct}], cpu_pct, mem_pct, alerts_count}] |
| GET | /api/hosts/{id}/overview | Full snapshot (§5.1) |
| GET | /api/hosts/{id}/history?window=300 | Historical series (window seconds: 300/900/3600) |
| GET | /api/hosts/{id}/events?limit=50 | Event stream |

history response:

```jsonc
{
  "ts": [1724000000, 1724000001, "..."],
  "gen_speed": [0, 42.1, "..."],
  "prompt_speed": [120.5, 0, "..."],
  "gpu_util": [[0, 87, "..."], [0, 92, "..."]],
  "gpu_mem": [[18459, "..."], [17639, "..."]],
  "gpu_temp": [[54, "..."], [49, "..."]],
  "cpu": [12.3, "..."],
  "mem_used": [5939, "..."],
  "mem_buff_cache": [25600, "..."],
  "net_rx": [0.5, "..."], "net_tx": [0.2, "..."],
  "proc_cpu": [45.2, "..."],
  "load_1": [1.09, "..."], "load_5": [0.75, "..."], "load_15": [0.58, "..."],
  "ctx_used": [130453, 135504, "..."],
  "mtp_acceptance": [0.931, 0.850, "..."]
}
```

### 6.2 WebSocket

- Path: /ws/hosts/{id}
- Server → Client: push snapshot every push_interval (default 1s, structure same as overview)
- Client → Server: {"type": "ping"} (heartbeat, once per 10s)
- Server → Client: {"type": "pong"}
- Heartbeat timeout 30s → server disconnects; client auto-reconnects (1s/2s/4s backoff), falls back to HTTP polling during disconnection
- Portal page: /ws/portal pushes /api/hosts data every 1s

## 7. Frontend Architecture

- Routing: / (Portal) /host/:id (Detail)
- State: module-level reactive store (no Pinia, scenario is simple) + page-level composable
- WS client: useHostStream(hostId) composable — WS primary, auto-fallback to polling after 3 heartbeat failures, exposes {snapshot, connected, mode}
- Component tree:

```
App
├─ PortalView
│  ├─ BrandBar
│  └─ HostCard × N
└─ HostDetailView
   ├─ TopBar
   ├─ OverviewSection (BarCard × 3 + GaugeCard × 1)
   ├─ TaskSection (LlamaStateCard + EventFeed)
   ├─ GpuSection (GpuPanel × N)
   ├─ SystemSection (CpuPanel, MemPanel, DiskPanel, NetPanel)
   ├─ ProcessSection (LlamaProcessCard, TopProcessTable × 2)
   ├─ ModelSection (ModelInfoCard, SlotTable)
   └─ TrendSection (TrendChart × 12, 3 groups: llama/GPU/system)
```

- ECharts: shared dark theme config; charts updated via setOption (do not rebuild instances); animations disabled (animation: false, monitoring charts refresh instantly without flickering)
- Numbers: count-up animation (requestAnimationFrame, 300ms)

## 8. Threshold Engine

- Evaluation timing: every snapshot generation (backend), produces alerts[] (metric/level/value/threshold)
- Frontend only renders: level=danger → red style, warn → yellow style
- Configuration: global defaults + per-host override (see hosts.yaml)

## 9. Deployment and Operations

- Directory structure (implementation phase):

```
/data/case/LlamaLens/
├── backend/
│   ├── main.py            # FastAPI entry, static hosting, Registry startup
│   ├── config.py          # hosts.yaml + .env parsing
│   ├── store.py           # RingBuffer
│   ├── diff.py            # DiffEngine
│   ├── events.py          # EventDetector
│   ├── api.py             # REST routes
│   ├── ws.py              # WS routes
│   ├── pollers/
│   │   ├── llama_api.py
│   │   └── ssh_host.py
│   └── requirements.txt
├── frontend/
│   ├── package.json
│   ├── vite.config.js     # dev proxy /api,/ws → :8000
│   ├── index.html
│   └── src/
│       ├── main.js, App.vue, router.js
│       ├── api.js, stream.js
│       ├── theme/echarts-dark.js
│       ├── views/PortalView.vue, HostDetailView.vue
│       └── components/ (15 components, see 03-UIandInteractionDesignDocument.md §4)
├── config/
│   ├── hosts.yaml         # gitignore
│   └── hosts.example.yaml
├── .env                   # gitignore (AI_SSH_PASS=...)
├── .env.example
├── .gitignore
├── Dockerfile             # Multi-stage build: node builds frontend → python slim runs backend
├── .dockerignore
├── docker-compose.yml     # docker compose up -d --build
├── run.sh                 # uvicorn backend.main:app --host 0.0.0.0 --port 8000
└── README.md
```

- Startup: ./run.sh (first time: cd frontend && npm install && npm run build)
- Docker deployment: multi-stage Dockerfile (node:20-alpine builds frontend → python:3.11-slim runs backend,
  code compatible with Python 3.9+), `docker compose up -d --build` or
  `docker build -t llamalens:latest . && docker run ...`;
  config/hosts.yaml and .env mounted read-only at runtime (not baked into image), logs/ mounted to host;
  built-in HEALTHCHECK (/api/health); port controlled by PORT env variable (default 8000)
- Logging: uvicorn stdout + backend logging (logging, INFO, output to logs/llamalens.log)
- Resource overhead estimate: Python process ~150-300MB (including history buffer), CPU < 5% single core; one SSH command per host every 2s
- Port: 8000 (configurable via env PORT)

## 10. Security Design

- Credentials: .env (gitignore) + hosts.yaml references environment variables; hosts.yaml itself is gitignore (contains host topology); only hosts.example.yaml template is committed
- SSH: password authentication (current), key authentication reserved (config field key_path)
- All collection commands are read-only, no write operations
- Panel listens on 0.0.0.0:8000, for LAN use, no authentication (risk acknowledged, see NFR-06)
- Frontend code contains no credentials; API does not return password fields

## 11. Extensibility Design

| Extension Point | Method |
|---|---|
| Add new host | Add entry to hosts.yaml |
| Add new metric | Add one line to SshPoller batch command + snapshot field + frontend panel |
| Add new event type | Add one transition to EventDetector state machine |
| --metrics integration | If instance enables --metrics, LlamaPoller prefers /metrics (llama_tokens_generated_total, etc.), differential method as fallback |
| Multiple processes per host | process.name supports list, snapshot.processes[] |
| History persistence | Add writer to RingBuffer (SQLite), API unchanged |

## 12. Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Local Python 3.9.2 | Code 3.9 compatible (no match/3.10+ syntax); all dependencies pre-installed |
| /metrics not enabled | Differential method is primary (practically /slots fields are sufficient) |
| SSH disconnect | keepalive + exponential backoff reconnect + degraded display |
| llama-server restart (model switch) | id_task resets baseline; /props refreshes every 30s; model_change event |
| 1s polling load | /slots is lightweight read-only JSON (~2KB); --threads-http 1 impact negligible; interval configurable |
| IPv6 hosts | paramiko/requests natively support IPv6 (tested connectivity) |
| GPU1 PCIe x4 (abnormal link) | Display truthfully, user can judge on the page |
| Log format changes (llama.cpp version upgrade) | Parsing rules centralized in one place (regex table), single-point modification; parse failure only skips that line, does not affect overall; speed has /slots differential as fallback |
| Journal log volume (print_timing one line per 3s) | Only stream follow + first pull 200-line startup block, no full history read; single-line parse O(1) |

## 13. Local CLI (llamalens)

The htop-style full-screen TUI running **on the local monitored host**, sharing the same source as the Web panel single-host detail page (same collection fields, thresholds, events, and log parsing rules). Zero runtime dependencies: a single static binary (`CGO_ENABLED=0`), target host does not need Go / Python / pip.

### 13.1 Technology Selection

| Item | Choice | Rationale |
|---|---|---|
| Language | Go 1.24 | Single static binary, cross-platform, convenient `/proc` direct reading |
| TUI Framework | bubbletea + lipgloss (charm) | Manages terminal lifecycle (raw mode / alternate screen / cursor / resize / exit restore), **no hand-written ANSI escapes** — avoids full-screen garbled text caused by early hand-written escapes |
| Rendering | lipgloss styles + custom sparkline/bar | Same colors as frontend Terminal theme (cyan/green/amber/red) |

### 13.2 Architecture (single process, no network dependency)

```
llamalens (single process)
├── collect.LlamaCollector   Local HTTP: /slots@1s, /props+/v1/models@30s
├── collect.HostCollector    /proc direct read@2s (stat/meminfo/loadavg/net/dev/diskstats/[pid]/stat) + nvidia-smi@2s + systemctl show
├── logparse.Poller          11 log line regex patterns + state machine (same source as backend/pollers/log_poller.py)
├── collect.JournalCollector journalctl -u <unit> -f stream (reconnect with --since catchup on disconnect; PID change re-pulls boot block)
├── model.World              Aggregation: differential engine + ring buffer (llama@1s/3600, host@2s/1800) + event detection + alert evaluation
└── ui.App (bubbletea)       4fps rendering; q/p/t/g keys; WindowSizeMsg adaptive
```

Data flow same as Web version: collectors → World.Tick (1s: speed priority log tg_3s > /slots differential, write ring, build snapshot, evaluate alerts) → snapshot → render.

Task stuck status is **based on /slots API** (any slot `is_processing` means processing): journal stream has delay/line loss, task start log phase lags behind API; if relying on log phase, you would see "token speed has value but status is idle". Log phase is only used for refined labels (decoding/prefill); when processing and log hasn't caught up, display "processing". Task ID same: during processing, use `slot.id_task` (log task_id may be leftover from previous task).

nvidia-smi collection details: `--query-gpu` includes `uuid`, `--query-compute-apps` includes `gpu_uuid`, processes **per-card ownership** (each card's "usage" row shows that card's VRAM, same PID appears once per card in use; old drivers without `gpu_uuid` fall back to all on first card). Note CSV has spaces after commas (`" 17920"`), numeric parsing must trim, otherwise `ParseInt` fails and always returns 0 (once caused "GPU usage 0MB").

### 13.3 Rendering Correctness Constraints (Critical)

Early versions with hand-written ANSI escapes caused full-screen garbled text (packet-like). After rewrite, three mandatory constraints enforced, verified on PTY:

1. **No hand-written escapes**: terminal lifecycle fully delegated to bubbletea; styles all via lipgloss.
2. **ANSI-aware width/truncation**: `displayWidth`/`trunc` skips escape sequences by byte (distinguishes CSI leader `[` from terminator byte), CJK counts as 2 columns; never truncate in the middle of an escape sequence (otherwise color codes leak + line width overflow).
3. **Line width ≤ terminal width - 1**: filling the last column triggers terminal auto-wrap, causing subsequent lines to shift down and overlap across frames. `Render()` ends with `trunc(l, w-1)` per line as fallback; lipgloss `Width/Height` are inner dimensions (excluding borders), outer panel dimensions need -2 first.
4. **TopBar truncation priority**: left segments are "brand · host · model  online status", when space is insufficient first truncate model name, then host name, ensuring online status always visible (full left truncation would first cut off the trailing online status).
5. **Border color brighter than frontend**: frontend card borders `#242428` rely on card background color; TUI has no background color so nearly invisible on dark terminal, thus TUI borders use `#55555e` (remaining colors consistent with frontend Terminal theme).
6. **Right-edge alignment per line**: each line's total width must exactly equal available width (terminal width − 2). `lipgloss.JoinHorizontal` does not insert separator columns (easily mistaken for occupying space — old code reserved (n−1) gap columns but they did not actually exist, causing ragged right edges). When splitting columns, last column takes remainder: `cw=usable/n`, `cwLast=usable−cw×(n−1)`.

### 13.4 Configuration (flags, defaults match ai.lan)

`--llama-host 127.0.0.1 --llama-port 8080 --process llama-server --unit llama-server --log journal --mounts / --once --dump-frame <file> --no-color`. `--dump-frame` for troubleshooting: writes raw bytes of each render frame (including ANSI) to file (overwrites each time), press `p` to pause to retain that frame when reproducing display issues. `--no-color` for troubleshooting: `lipgloss.SetColorProfile(termenv.Ascii)` plain-text rendering (lipgloss reads global profile at render time, set before startup for global effect). Process name / unit / port must match the corresponding host in `hosts.yaml` (see README "Preparing the Monitored Host" four conditions). `--process` process name first matches exactly via `/proc/<pid>/comm`, falls back to cmdline argv[0] basename match on failure (kernel truncates comm to 15 characters, custom builds can pass full name directly, e.g. `llama-cpp-turboquant`). TopBar model name same as Web TopBar: alias shows only filename when full path; model card shows full name + size + mmproj (mmproj from process cmdline `--mmproj`).

### 13.5 Relationship with Web Panel

CLI is a **read-only sidecar**: does not pass through backend, does not write any files, does not change llama-server state. Collection fields, thresholds, event types, and log parsing regex are same-source as backend (backend is Python, CLI is Go, rules maintained separately but semantically consistent). When backend upgrades collection fields, need to simultaneously check whether CLI needs alignment.
