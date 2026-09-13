# llama Lingjing Requirements Document

| Item | Content |
|---|---|
| Document Version | v1.0 |
| Date | 2026-08-28 |
| Status | Design Phase (Subsequent Development Baseline) |
| Project Path | /data/case/LlamaLens |
| Related Documents | 02-ArchitectureDesignDocument.md, 03-UIandInteractionDesignDocument.md |

> This document is the primary reference for subsequent development. Any requirement changes must first update this document, then modify the code.

## 1. Background and Objectives

### 1.1 Background

- A llama.cpp llama-server inference instance exists on the LAN (first instance: ai.lan), running Qwen3.8-27B-Q6_K (27.32B parameters, dual RTX 3080 tensor split inference, with vision capabilities).
- Currently, there is no visual means to observe instance load: token generation speed, GPU utilization/memory/temperature/power, CPU, memory, process status, etc. can only be viewed via SSH + nvidia-smi + manual commands.
- More inference hosts will be connected later, requiring a "one page per host + unified portal" monitoring solution.

### 1.2 Objectives

1. Build a Web monitoring dashboard called llama Lingjing, deployed locally (single process, port 8000), to remotely monitor N llama-server hosts.
2. Each host has an independent detail dashboard; the portal page aggregates all hosts.
3. Data refreshes in real time at 1-second granularity (configurable), with a high-tech, dynamic visual style.
4. Display all AI/llama-related information in detail (80+ data items, full list in §7).
5. Display-only, no alert notifications, but metrics exceeding thresholds must visually flash red.

### 1.3 Out of Scope (This Phase)

- No alert push notifications (email/IM/webhook)
- No historical data persistence (only in-memory ring buffer, lost on restart)
- No user authentication (internal LAN tool)
- No modifications to monitored hosts' configurations (no --metrics flag, no agent installation)
- No Docker/K8s packaging (run.sh runs directly; Docker is for subsequent iterations)

## 2. Terminology

| Term | Description |
|---|---|
| llama-server | Official llama.cpp HTTP inference service (this instance is a custom build: llama-cpp-turboquant) |
| Slot | llama-server concurrent session slot (-np 1 means 1 slot) |
| tok/s | Tokens generated per second |
| Prompt Processing Speed | Prompt token prefill speed (tok/s) |
| Tensor Split | Model layers split across multiple GPUs (-ts 24,20) |
| Speculative Decoding | draft-mtp speculative decoding (--spec-type draft-mtp --spec-draft-n-max 3) |
| Host | A monitored inference machine (registered in hosts.yaml) |
| Portal Page | Top-level page listing all hosts (/) |
| Detail Page | Single host dashboard (/host/:id) |
| Threshold Red Flash | Visual highlight when a metric exceeds its threshold (yellow = warning / red = danger) |

## 3. Users and Use Cases

- **Users**: AI infrastructure operations / developers (on the LAN)
- **Use Cases**:
  1. Daily inspection: Open the portal page and see all host statuses at a glance.
  2. Task monitoring: While a long inference task runs, observe token speed and GPU load in real time.
  3. Troubleshooting: When a task slows down, check GPU utilization/temperature/power/PCIe links for anomalies.
  4. Capacity assessment: Observe memory/disk/context usage to determine if an instance is nearing its limits.

## 4. Functional Requirements

### FR-01 Multi-Host Management and Configuration

- Hosts are registered in `config/hosts.yaml`: id, name, llama address (host:port), SSH (host/port/user/password), collection interval, threshold overrides.
- Adding/removing hosts only requires changing the config + restarting the service, no code changes.
- Each host is monitored independently: one host failure does not affect other hosts or the dashboard itself.
- The first host is ai.lan (live environment details in Appendix A).

### FR-02 Host Portal Page (/)

- Top brand bar: llama Lingjing logo, total hosts / online count.
- Host card wall (grid layout), each card displays:
  - Host name + status dot (online green pulse / offline red / SSH disconnected yellow)
  - Model name + parameter count
  - Token generation speed (large number + 60s sparkline)
  - One utilization bar per GPU (one row per card)
  - CPU utilization, memory usage
  - Threshold red flash (red border + red corner badge)
- Click a card to enter the detail page /host/:id.
- Portal page also refreshes in real time (1s).

### FR-03 Single Host Detail Dashboard (/host/:id)

- Full layout in 03-UIandInteractionDesignDocument.md §3; sections are:
  TopBar / Real-Time Overview Area (4 cards: 3 bar cards + 1 gauge) / GPU Area (aggregated per card) / Real-Time Generation Task Area (status card + event stream) / System Area / Process Area / Model & Slot Area / Trend Area (12 charts, 3 groups)

### FR-04 AI Core Metrics

- Display locations: speed / context usage as bar cards (context usage large number is raw token count, percentage in top-right corner), MTP acceptance rate as gauge (value-driven color), all in the real-time overview area (see 03-UIandInteractionDesignDocument §3.3); task status and event stream in the real-time generation task area (§3.4).
- Token Generation Speed (tok/s): real-time value + sparkline + historical curve.
  - Primary data source: log tg_3s (3-second window real-time speed, server-side calculated, more accurate); fallback: /slots n_decoded delta.
- Prompt Processing Speed (tok/s): same as above.
  - Primary data source: log "prompt processing" line (real-time speed + progress %); fallback: /slots n_prompt_tokens_processed delta.
- Context Usage: used / total and percentage.
  - Primary data source: log "release" line n_tokens (authoritative value, includes system prompt); fallback: /slots (n_prompt_tokens + n_decoded).
  - Display: used (large raw token number), percentage (top-right corner), remaining, total (n_ctx, full scale and ticks), whether truncated (truncated).
- Task Status: running / idle, task ID, remaining tokens (n_remain), current task cumulative decoded (n_decoded).
- Task-Level Statistics: total tokens, duration, average speed, MTP acceptance rate, context usage recorded at task end (enter event stream).

### FR-05 GPU Monitoring (aggregated per card)

- One independent panel per GPU, containing all metrics for that card:
  - Utilization (circular gauge)
  - Memory: used/free/total + progress bar
  - Temperature, power (draw/limit), fan speed, graphics clock, memory clock
  - PCIe link (gen/width), performance state (P-state)
  - Driver version, card model
  - Processes using this card (pid/name/memory)
- Historical curves: utilization, memory (one line per card)

### FR-06 System Resource Monitoring

- CPU: model, core count, total utilization, per-core utilization (one bar per core), load 1/5/15, clock speed.
- Memory: total/used/free/buff_cache/available, swap used/total.
- Disk: per mount point size/used/avail/percentage + read/write rates.
- Network: per NIC rx/tx rates and cumulative.
- System: hostname, kernel, OS version, uptime, total process count.

### FR-07 Process Monitoring

- llama-server process card:
  - PID, real-time CPU% (delta), cumulative CPU%, RSS, VSZ, thread count, uptime.
  - systemd service status (unit name/description/active/start time/service-level cumulative CPU/service-level memory including peak/Tasks count).
  - Full command line (collapsible) + parsed parameter table (model, mmproj, n-gpu-layers, flash-attn, tensor-split, batch, ubatch, np, ctx-size, kv-offload, cache-type-k/v, fit, threads, threads-batch, threads-http, temperature, top-p, top-k, spec-type, spec-draft-n-max, port, host).
- Top Processes: Top 8 by CPU, Top 8 by memory (pid/name/cpu%/mem%/rss).

### FR-08 Model and Slot Details

- Model Information: name, path, ftype, parameter count, embedding dimension, vocab size, file size, mmproj size, modalities (vision/video/audio), n_ctx, n_ctx_train, vocab_type, capabilities, owned_by.
- Slot Details: id, status, task ID, prompt tokens (total/processed/cached), decoded, remaining, context usage %, full sampling parameters (temperature, dynatemp_*, top_k, top_p, min_p, top_n_sigma, xtc_*, typical_p, repeat_last_n, repeat_penalty, presence_penalty, frequency_penalty, dry_*, mirostat_*, adaptive_*, max_tokens, n_predict, n_keep, n_discard, ignore_eos, stream, n_probs, min_keep, chat_format, reasoning_format, reasoning_in_content, generation_prompt, samplers, speculative.types, timings_per_token, post_sampling_probs, backend_sampling, lora).

### FR-09 Historical Trend Charts

- Chart types (12 charts, 3 groups, related metrics adjacent):
  - llama group: Token Generation Speed, Prefill Speed, **Context Usage (1s sample line + n_ctx upper reference line)**, **MTP Acceptance Rate (per-task step trend)**
  - GPU group: Utilization (per card), Memory (per card), Temperature (per card), Power (per card)
  - System group: CPU, Memory (used/buff_cache stacked), Network (rx/tx), Load Average (1/5/15 three-line)
- Time window switching: 5m / 15m / 1h.
- Data source: in-memory ring buffer (llama 3600 points @1s, host 1800 points @2s).
- Broken line during disconnection (no interpolation), retain last value.

### FR-10 Event Stream

- Event types: task start, prefill start (prompt total tokens), task end (with total tokens/duration/average speed), model change, llama online/offline, SSH disconnect/reconnect, threshold crossing (alert level upgrade/recovery).
- Terminal-style display, auto-scroll, memory retains last 200 events.
- Each event: timestamp + level (info/warn/error) + description.

### FR-11 Real-Time Refresh Mechanism

- Default 1-second refresh (WebSocket push as primary channel).
- Frontend refresh control: real-time (WS) / 1s / 2s / 5s / pause.
- Backend push interval is configurable per host (push_interval, default 1s).
- Collection interval is configurable: llama 1s, SSH 2s (default values).
- WS disconnection automatically degrades to HTTP polling with a prompt.

### FR-12 Threshold Red Flash

- Two-level color scale: yellow (warn) / red (danger), rule table in §8.
- Red flash effect: number turns red + card border red glow + pulse animation.
- Thresholds can be overridden per host.
- Purely visual, no notification push.

### FR-13 llama-server Log Parsing

- Real-time tail systemd journal (journalctl -u llama-server), parse the following log lines:
  - `print_timing ... n_decoded = X, tg = Y t/s, tg_3s = Z t/s` (every 3s during generation): real-time speed, task average speed, decoded count.
  - `print_timing ... prompt processing, n_tokens = X, progress = P, t = T s / S t/s` (every ~2s during prefill): prompt progress, prompt speed.
  - `print_timing ... prompt eval time / eval time / total time / graphs reused / draft acceptance` (task-end summary): prompt/decode/total time, CUDA graph reuse count, **MTP acceptance rate/accepted count/average accepted length**.
  - `release ... stop processing: n_tokens = X, truncated = T` (task end): **total context usage, whether truncated**.
  - `get_availabl ... selected slot by LCP/LRU, f_sim_best = X, f_keep = Y` (new task): slot selection method, LCP similarity, **KV cache retention rate**.
  - `launch_slot_ ... task N, is_child = C` (task start): task ID, whether sub-task.
  - Startup log (once per startup): model path, n_slots/n_ctx_slot/kv_unified, MTP draft context, KV cache type upgrade (e.g., turbo3→q8_0), verbosity, listening address, warning list (CORS, port change notification, etc.).
- "What is llama doing right now" state machine: prompt processing (with progress) / generating (with decoded count and speed) / idle.
- Log unavailable (SSH disconnected / unit name mismatch) → fall back to /slots delta, mark log_available=false.
- MTP parameters: static configuration (spec-type/spec-draft-n-max, from command-line parsing) + runtime statistics (acceptance rate/mean len, from logs) displayed simultaneously.


### FR-14 Local CLI (llamalens)

- An htop-style full-screen TUI running **locally** on the monitored host, displaying content from the same source as the single-host detail page (FR-03~FR-13).
- Zero runtime dependencies: single static binary (Go, `CGO_ENABLED=0`), target hosts do not need Go / Python / pip.
- Data collected locally directly (no backend, no file writes, no llama-server state changes):
  - llama HTTP API (`/slots`@1s, `/props`+`/v1/models`@30s)
  - `/proc` direct read@2s (CPU/memory/disk/network/load/top processes)
  - `nvidia-smi`@2s (GPU)
  - `systemctl show` + `journalctl -u <unit> -f` (service status + log event stream, parsing rules same as FR-13).
- Interactions: `q` exit / `p` pause / `t` trend / `g` GPU; terminal resize adaptive; auto-restore terminal on exit (no residue).
- Threshold red flash shares the same source as the Web panel (§8).
- Terminal requirements: ≥80×20, requires interactive TTY; also provides `--once` single-run text snapshot (usable without TTY).
- Process name / unit / port must match the corresponding host in hosts.yaml.

## 5. Non-Functional Requirements


| ID | Requirement | Metric |
|---|---|---|
| NFR-01 | Refresh Latency | Data generation to page display ≤ 2s (1s collection + 1s push) |
| NFR-02 | API Response | /api/hosts/{id}/overview P95 < 100ms |
| NFR-03 | Degradation | Single host llama offline / SSH disconnected does not affect other hosts or the dashboard; page does not go blank |
| NFR-04 | Resource Overhead | Dashboard process CPU < 5% (single core), memory < 300MB (including 1h history) |
| NFR-05 | SSH Overhead | One batch command per host every 2s, one network round-trip, all read-only |
| NFR-06 | Security | Credentials not in Git; dashboard listens on 0.0.0.0:8000 (LAN), no authentication (internal LAN tool, risk declared) |
| NFR-07 | Scalability | New host = add one config line; new metric = extend collection layer + snapshot fields |
| NFR-08 | Compatibility | Latest Chrome/Edge; 1920×1080 baseline, 1366×768 usable; responsive layout (full-width content, no horizontal scroll in narrow windows, breakpoints 1500/1100/640px) |

## 6. Acceptance Criteria

| ID  | Criteria                                                                                                                                                                  | Verification Method                          |
| -------| -------------------------------------------------------------------------------------------------| -----------------------------------|
| AC-01 | Portal page displays ai.lan host card, status online, model Qwen3.8-27B-Q6_K (27.32B)                                                                                      | Open /                                   |
| AC-02 | Detail page token generation speed > 0 during task execution and matches actual speed; 0 when idle                                                                         | Compare with running task / send small request |
| AC-03 | GPU panel data (memory/temperature/power) matches direct nvidia-smi connection                                                                                              | Compare                                  |
| AC-04 | CPU/memory/disk data matches free -h / df -h                                                                                                                               | Compare                                  |
| AC-05 | Process card command line matches ps output, parameter table parsed correctly                                                                                               | Compare                                  |
| AC-06 | Page refreshes at 1s, refresh control (1s/2s/5s/pause) takes effect                                                                                                         | Visual inspection + browser network panel |
| AC-07 | Metrics exceeding thresholds (e.g., GPU utilization ≥90%) cause corresponding numbers and card borders to turn red                                                          | Create high load or temporarily lower threshold |
| AC-08 | Historical curves are continuous, window switching (5m/15m/1h) takes effect                                                                                                 | Visual inspection                        |
| AC-09 | Event stream records task start/end events (with statistics)                                                                                                               | Observe during task execution            |
| AC-10 | Stop llama-server → portal card turns red "offline", detail page degrades display; automatically recovers after restart                                                    | Stop/start service                       |
| AC-11 | Wrong SSH port → host area shows yellow "unavailable", llama area normal                                                                                                    | Config change test                       |
| AC-12 | hosts.yaml/.env (with passwords) not in Git; .gitignore takes effect                                                                                                        | git status                               |
| AC-13 | Add second host configuration (can point to a test entry on the same machine) → portal shows 2 cards                                                                       | Config test                              |
| AC-14 | Log parsing: current status card correctly shows phase (prompt processing/generating/idle), task ID, real-time speed matches log tg_3s                                       | Compare with journalctl                  |
| AC-15 | Context usage displays n_tokens/n_ctx and remaining amount, matches log "release" line; context usage trend chart is continuous                                             | Compare with journalctl                  |
| AC-16 | MTP card displays acceptance rate/accepted count/mean len, matches log "draft acceptance" line                                                                              | Compare with journalctl                  |

## 7. Data Item Inventory (80+ Fields)

> Sources: H = llama-server HTTP API, S = SSH read-only collection, L = Log parsing (journalctl -u llama-server), C = Backend computation (delta/aggregation)

### 7.1 Model (H: /props, /v1/models; S: ls -l)

| Field                        | Source | Description (ai.lan live values)                                       |
| -------------------------| ------| --------------------------------------------------|
| model.name                 | H     | Model name (path)                                                  |
| model.path                 | H     | /share/AI/LLM/unsloth/Qwen3.8-27B-Q6_K.gguf                           |
| model.ftype                | H     | Quantization type (Q6_K)                                             |
| model.n_params             | H     | Parameter count (27,320,697,856 ≈ 27.32B)                             |
| model.n_embd               | H     | Embedding dimension (5120)                                           |
| model.n_vocab              | H     | Vocabulary size (248320)                                             |
| model.n_ctx                | H     | Runtime context length (262144)                                      |
| model.n_ctx_train          | H     | Training context length (262144)                                     |
| model.vocab_type           | H     | Vocabulary type (2)                                                  |
| model.file_size            | H/S   | Model file size (22,873,411,584 B ≈ 22.87GB)                          |
| model.mmproj_path          | S     | Vision projection file path (Qwen3.8-27B-mmproj-BF16.gguf)             |
| model.mmproj_size          | S     | mmproj file size (889MB)                                             |
| model.modalities.vision    | H     | Vision support (true)                                                |
| model.modalities.video     | H     | Video support (true)                                                 |
| model.modalities.audio     | H     | Audio support (false)                                                |
| model.capabilities         | H     | completion, multimodal                                               |
| model.owned_by             | H     | llamacpp                                                             |

### 7.2 Service (S: systemctl)

| Field              | Description (ai.lan live values)                           |
| ---------------------| ---------------------------------------|
| service.unit         | llama-server.service                                   |
| service.description  | llama-server Qwen3.8-27B (TurboQuant)                  |
| service.active       | active (running)                                       |
| service.since        | Start time (2026-08-28 15:53:10)                       |
| service.cpu_total    | Service-level cumulative CPU (9min 57.800s)            |
| service.memory       | Service-level memory (4.6G)                            |
| service.memory_peak  | Memory peak (4.6G)                                     |
| service.tasks        | Tasks count (10)                                       |

### 7.3 Process (S: /proc/<pid>/stat, ps, cmdline)

| Field | Description |
|---|---|
| process.found | Whether process was found |
| process.pid | 31255 |
| process.cpu_pct_realtime | Real-time CPU% (delta, relative to single core) |
| process.cpu_pct_lifetime | Cumulative CPU% (ps) |
| process.mem_pct | Memory percentage (16.0%) |
| process.rss_mb | Resident set (≈5153MB) |
| process.vsz_mb | Virtual memory |
| process.threads | Thread count |
| process.elapsed | Uptime |
| process.cmdline | Full command line |
| process.flags.* | Parsed parameters (model/mmproj/n_gpu_layers/flash_attn/tensor_split/batch/ubatch/np/ctx_size/kv_offload/cache_type_k/cache_type_v/fit/threads/threads_batch/threads_http/temperature/top_p/top_k/spec_type/spec_draft_n_max/port/host) |

### 7.4 Slot (H: /slots)

| Field | Description |
|---|---|
| slot.id | Slot number (0) |
| slot.is_processing | Whether processing |
| slot.id_task | Task ID (5199) |
| slot.n_ctx | Slot context length (262144) |
| slot.n_prompt_tokens | Total prompt tokens (19032) |
| slot.n_prompt_tokens_processed | Prompt processed (626) |
| slot.n_prompt_tokens_cache | Prompt cache hits (0) |
| slot.n_decoded | Decoded count (795) |
| slot.n_remain | Remaining tokens (31205) |
| slot.ctx_used | Context usage (C: prompt+decoded) |
| slot.speculative | Whether speculative decoding is enabled (true) |
| slot.params.* | Full sampling parameters (temperature/dynatemp_range/dynatemp_exponent/top_k/top_p/min_p/top_n_sigma/xtc_probability/xtc_threshold/typical_p/repeat_last_n/repeat_penalty/presence_penalty/frequency_penalty/dry_multiplier/dry_base/dry_allowed_length/dry_penalty_last_n/mirostat/mirostat_tau/mirostat_eta/adaptive_target/adaptive_decay/max_tokens/n_predict/n_keep/n_discard/ignore_eos/stream/n_probs/min_keep/chat_format/reasoning_format/reasoning_in_content/generation_prompt/samplers/speculative.types/timings_per_token/post_sampling_probs/backend_sampling/lora) |

### 7.5 Speed (L: logs primary, C: delta fallback)

| Field | Source | Description |
|---|---|---|
| speed.gen_tps | L/C | Generation speed tok/s (log tg_3s primary, /slots delta fallback) |
| speed.gen_tg_tps | L | Task average generation speed (log tg) |
| speed.prompt_tps | L/C | Prompt processing speed tok/s (log primary, /slots delta fallback) |
| speed.prompt_progress | L | Prompt processing progress 0-1 (log progress) |
| speed.prompt_total_tokens | L | Total prompt tokens (log n_tokens) |
| speed.prompt_elapsed_s | L | Prompt elapsed seconds (log t) |
| log.last_prefill | L | Last prefill {speed, ts, progress, n_tokens} (persistent, updated per prompt line, cleared on restart) |
| speed.task_total_tokens | L | Current task cumulative tokens (event) |
| speed.task_duration_s | L | Current task duration (event, log total time) |
| speed.task_avg_tps | L | Current task average speed (event, log eval time) |

### 7.6 GPU (S: nvidia-smi, per card)

| Field          | Description (ai.lan live values)                              |
| -------------------| ---------------------------------|
| gpu.index          | Card number (0/1)                                               |
| gpu.name           | Model (NVIDIA GeForce RTX 3080)                                 |
| gpu.driver         | Driver version (580.159.03)                                     |
| gpu.cuda           | CUDA version (13.0)                                             |
| gpu.util_pct       | GPU utilization %                                               |
| gpu.mem_util_pct   | Memory controller utilization %                                 |
| gpu.mem_total_mb   | Total memory (20480)                                            |
| gpu.mem_used_mb    | Memory used (18459/17639)                                       |
| gpu.mem_free_mb    | Memory free                                                     |
| gpu.temp_c         | Temperature (54/49°C)                                           |
| gpu.power_w        | Current power (141.5/137.3W)                                    |
| gpu.power_limit_w  | Power limit (280W)                                              |
| gpu.fan_pct        | Fan speed                                                       |
| gpu.clock_mhz      | Graphics clock                                                  |
| gpu.mem_clock_mhz  | Memory clock                                                    |
| gpu.pcie_gen       | PCIe generation (3)                                             |
| gpu.pcie_width     | PCIe width (16/4)                                               |
| gpu.pstate         | Performance state (P2)                                          |
| gpu.temp_mem_c     | Memory temperature °C (HBM cards have values, consumer cards [N/A]→null) |
| gpu.ecc_corrected  | ECC corrected errors cumulative (volatile, [N/A]→null)          |
| gpu.ecc_uncorrected | ECC uncorrected errors cumulative (volatile, >0 frontend red flash) |
| gpu.throttle       | Throttling reason bitmask (0=normal; Not Active/decimal/0x hex all parsed) |
| gpu.cuda           | CUDA version (13.0)                                             |
| gpu.apps[]         | Processes using the card (pid/name/memory)                      |

### 7.7 CPU (S: /proc/stat, /proc/cpuinfo)

| Field | Description |
|---|---|
| cpu.model | Intel Core i5-8500 |
| cpu.cores | Core count (6, no hyperthreading) |
| cpu.usage_pct | Total utilization (C delta) |
| cpu.per_core_pct[] | Per-core utilization (C delta, 6 values) |
| cpu.load1/load5/load15 | Load (1.09/0.75/0.58) |
| cpu.mhz | Clock speed (3000) |

### 7.8 Memory (S: /proc/meminfo)

| Field | Description |
|---|---|
| mem.total_mb | Total (31Gi) |
| mem.used_mb | Used (5.8Gi) |
| mem.free_mb | Free |
| mem.buff_cache_mb | buff/cache (25Gi) |
| mem.available_mb | Available |
| mem.swap_total_mb | Swap total (8Gi) |
| mem.swap_used_mb | Swap used |

### 7.9 Disk (S: df, /proc/diskstats)

| Field | Description |
|---|---|
| disk.mounts[] | Mount point list (/) |
| disk.size_gb/used_gb/avail_gb/use_pct | Capacity (1.2T/801G/347G/70%) |
| disk.read_mb_s/write_mb_s | Read/write rates (C delta) |

### 7.10 Network (S: /proc/net/dev)

| Field | Description |
|---|---|
| net.ifaces[] | NIC list (excluding lo) |
| net.rx_mb_s/tx_mb_s | Rates (C delta) |
| net.rx_total/tx_total | Cumulative |

### 7.11 System (S)

| Field | Description |
|---|---|
| sys.hostname | ai |
| sys.kernel | 6.8.0-124-generic |
| sys.os | OS version (/etc/os-release) |
| sys.uptime_s | Uptime |
| sys.procs | Total process count |

### 7.12 Events (C)

| Field | Description |
|---|---|
| event.ts | Timestamp |
| event.level | info/warn/error |
| event.type | task_start/prefill_start/task_end/model_change/llama_up/llama_down/ssh_down/ssh_up/llama_boot/alert |
| event.msg | Description (task_end includes total tokens/duration/average speed/MTP acceptance rate/context usage; prefill_start includes prompt total tokens; alert includes metric/value/threshold/level) |

### 7.13 Logs (L: journalctl -u llama-server)

| Field                                                                                               | Description (ai.lan live values)                                                                                     |
| ---------------------------------------------------| ---------------------------------------------------------|
| log.available                                                                                         | Whether log channel is available                                                         |
| log.state.phase                                                                                         | Current phase: prompt_processing / decoding / idle                                         |
| log.state.task_id                                                                                         | Current task ID (14847)                                                                    |
| log.state.n_decoded                                                                                         | Current task decoded count (2843)                                                          |
| log.state.tg_3s_tps                                                                                         | 3-second window real-time speed (28.46)                                                    |
| log.state.tg_tps                                                                                          | Task average speed (26.21)                                                                 |
| log.state.prompt_progress                                                                                         | Prompt progress 0-1 (0.92)                                                                 |
| log.state.prompt_speed_tps                                                                                         | Prompt speed (516.98)                                                                      |
| log.state.is_child                                                                                          | Whether sub-task (0)                                                                       |
| log.context.used                                                                                          | Context used (n_tokens, 201047)                                                            |
| log.context.total                                                                                         | Context total (262144)                                                                     |
| log.context.pct                                                                                           | Percentage (76.7)                                                                          |
| log.context.remaining                                                                                       | Remaining (61097)                                                                          |
| log.context.truncated                                                                                       | Whether truncated (0)                                                                      |
| log.mtp.acceptance                                                                                          | MTP acceptance rate (0.698)                                                                |
| log.mtp.accepted                                                                                            | Accepted count (1925)                                                                      |
| log.mtp.generated                                                                                           | Generated count (2757)                                                                     |
| log.mtp.mean_len                                                                                            | Average accepted length (3.09)                                                             |
| log.kv.f_keep                                                                                               | KV cache retention rate (1.000)                                                            |
| log.kv.f_sim_best                                                                                           | LCP similarity (0.988)                                                                     |
| log.kv.selection                                                                                            | Slot selection method (LCP/LRU)                                                            |
| log.graphs_reused                                                                                           | CUDA graph reuse count (15340)                                                             |
| log.task.prompt_ms/prompt_tokens/prompt_speed_tps | Task summary: prompt time/tokens/speed                                                     |
| log.task.eval_ms/decoded_tokens/gen_speed_tps                                       | Task summary: decode time/tokens/speed                                                     |
| log.task.total_ms/total_tokens                                                                                         | Task summary: total time/total tokens                                                      |
| log.boot.pid/started_at                                                                                         | Startup process and time (55646 / 17:20:41)                                                |
| log.boot.verbosity                                                                                              | Log level (3)                                                                              |
| log.boot.n_slots/n_ctx_slot/kv_unified                                                                                         | Slot count/context/kv_unified (1/262144/false)                                              |
| log.boot.mtp_draft                                                                                              | Whether MTP draft context created (true)                                                   |
| log.boot.kv_cache_upgrade                                                                                         | KV cache type upgrade (turbo3→q8_0, auto-asymmetric GQA 6:1)                               |
| log.boot.listening                                                                                              | Listening address (http://0.0.0.0:8080)                                                    |
| log.boot.warnings[]                                                                                             | Warning list (CORS fully open, port change notification, etc.)                             |

## 8. Threshold Rules (default values, overridable per host)

| Metric | Yellow (warn) | Red (danger) |
|---|---|---|
| GPU Utilization | ≥80% | ≥90% |
| GPU Memory | ≥85% | ≥95% |
| GPU Temperature | ≥75°C | ≥85°C |
| GPU Power | ≥85% of limit | ≥95% of limit |
| CPU Utilization | ≥80% | ≥90% |
| Memory Usage | ≥85% | ≥95% |
| Disk Usage | ≥80% | ≥90% |
| Context Usage | ≥80% | ≥90% |
| MTP Acceptance Rate | <80% | <65% |
| llama Offline | — | Red |
| SSH Disconnected | — | Yellow |

## 9. Out of Scope

- Alert push notifications (email/IM/webhook) — subsequent iteration
- Historical data persistence (SQLite/InfluxDB) — subsequent iteration
- Multi-user authentication and authorization — internal LAN tool, not done for now
- llama-server remote control (start/stop/restart/change parameters) — read-only monitoring only
- Windows client / mobile adaptation — desktop browsers only
- Non-llama service monitoring (detailed monitoring of docker, etc.)

## 10. Subsequent Iterations (Not in This Phase)

1. Alert push: webhook / ServerChan / Bark
2. Historical persistence + long-term trends (7d/30d)
3. llama-server --metrics integration (if the instance restarts with that flag, switch to more precise data source)
4. SSH key authentication (replace password)
5. Docker/K8s packaging
6. Multiple models/instances on the same host (multiple llama-server processes)
7. Token cost estimation (speed × unit price)

## 11. Appendix A: Live Environment (ai.lan, 2026-08-28)

- Hostname ai, kernel 6.8.0-124-generic (Ubuntu), IPv6 2408:8266:401:2eca::fa6
- CPU: Intel Core i5-8500, 6 cores (no hyperthreading), 3.00GHz
- Memory: 31Gi (buff/cache 25Gi), swap 8Gi
- GPU: 2× NVIDIA GeForce RTX 3080 (20GB each), driver 580.159.03
  - GPU0: PCIe gen3 x16; GPU1: PCIe gen3 x4
- Disk: /dev/nvme0n1p3 1.2T, 70% used (801G/347G)
- llama-server: systemd unit llama-server.service, binary /home/wx/llama-cpp-turboquant-new/build/bin/llama-server
  - Full command line:
    ```
    -m /share/AI/LLM/unsloth/Qwen3.8-27B-Q6_K.gguf
    --mmproj /share/AI/LLM/unsloth/Qwen3.8-27B-mmproj-BF16.gguf
    --n-gpu-layers all --flash-attn enabled -ts 24,20 -b 1024 -ub 512 -np 1
    --ctx-size 262144 --kv-offload --cache-type-k turbo3 --cache-type-v turbo3
    --fit off --threads 6 --threads-batch 6 --threads-http 1
    --temperature 0.3 --top-p 0.9 --top-k 40
    --spec-type draft-mtp --spec-draft-n-max 3
    --port 8080 --host 0.0.0.0
    ```
- Available API endpoints: /health, /slots, /props, /v1/models (verified); /metrics not enabled (501)
- Model: Qwen3.8-27B-Q6_K.gguf (22.87GB, 27.32B parameters, n_embd 5120, n_vocab 248320) + mmproj (889MB, vision+video)
- Logs (journalctl -u llama-server, verified key lines):
  - During generation (every 3s): `slot print_timing: id 0 | task 14847 | n_decoded = 2843, tg = 26.21 t/s, tg_3s = 28.46 t/s`
  - During prefill (every ~2s): `slot print_timing: id 0 | task 8050 | prompt processing, n_tokens = 2048, progress = 0.92, t = 3.96 s / 516.98 tokens per second`
  - Task-end summary: `prompt eval time = 55930.81 ms / 23069 tokens (412.46 t/s)` + `eval time = 225389.07 ms / 5930 tokens (26.31 t/s)` + `total time = 281319.88 ms / 28999 tokens` + `graphs reused = 14433` + `draft acceptance = 0.68784 (3995 accepted / 5808 generated), mean len = 3.06`
  - Task end: `slot release: id 0 | task 12885 | stop processing: n_tokens = 193477, truncated = 0`
  - New task: `slot get_availabl: id 0 | task -1 | selected slot by LCP similarity, f_sim_best = 0.988 (> 0.100 thold), f_keep = 1.000`
  - Task start: `slot launch_slot_: id 0 | task 11793 | processing task, is_child = 0`
  - Startup: `loading model '...'`, `n_slots = 1, n_ctx_slot = 262144, kv_unified = 'false'`, `creating MTP draft context`, `auto-asymmetric: GQA ratio 6:1 — upgrading K from turbo3 to q8_0`, `listening on http://0.0.0.0:8080`
- Key log observations (2026-08-28):
  - Context grows across tasks: 130453 → 135504 → 146806 → 154804 → 159681 → 161641 → 164477 → 193477 → 201047 (reached 76.7% of 262144, long conversation accumulation, truncation risk exists)
  - MTP acceptance rate declines as context grows: 0.931 → 0.850 → 0.702 → 0.688 → 0.698 (mean len 3.79 → 3.06)
  - Prompt speed: cold start ~1100 t/s (no cache), hot ~500 t/s (KV cache hit)
