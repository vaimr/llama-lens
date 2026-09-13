# llama-lens UI & Interaction Design Document

| Field | Content |
|---|---|
| Document Version | v1.0 |
| Date | 2026-08-28 |
| Upstream Documents | 01-Requirements.md, 02-Architecture-Design.md |

## 1. Visual Style Specification (High-Tech Dark)

### 1.1 Color Palette

| Purpose | Color Name | Hex | Usage |
|---|---|---|---|
| Background Base | Deep Space Blue | #0a0e17 | Page background |
| Background Gradient | — | #0a0e17 → #0d1526 | Radial/linear gradient + fine grid (1px line rgba(0,229,255,0.04), 40px grid) |
| Card Base | Glass Blue | rgba(16, 24, 40, 0.72) | backdrop-blur 12px |
| Card Border | — | rgba(0, 229, 255, 0.15) | 1px; 0.35 on hover |
| Primary Color | Neon Cyan | #00e5ff | Primary text, active states, main lines |
| Secondary Color | Fluorescent Green | #00ff9d | Online status, normal values, secondary lines |
| Warning | Amber | #ffc53d | Yellow threshold |
| Danger | Alert Red | #ff3b5c | Red threshold, offline |
| Primary Text | — | #e6f1ff | Headings, primary numbers |
| Secondary Text | — | #8fa3c8 | Labels, auxiliary info |
| Tertiary Text | — | #5a6b8c | Timestamps, placeholders |
| Chart Grid | — | rgba(143, 163, 200, 0.12) | — |

### 1.2 Fonts

- Numbers/Data: 'JetBrains Mono', 'Roboto Mono', monospace (tabular-nums)
- Chinese/UI: system-ui, 'PingFang SC', 'Microsoft YaHei'
- Hierarchy: card large numbers 24px / gauge center numbers 20px / body text 13px / auxiliary 11px

### 1.3 Effects & Animations

| Effect | Specification |
|---|---|
| Card | Glassmorphism: rgba background + backdrop-blur(12px) + 1px border + 10px border-radius; border brightens on hover + translateY(-1px) |
| Glow | Primary elements box-shadow: 0 0 12px rgba(0,229,255,0.35); red alerts: 0 0 16px rgba(255,59,92,0.5) |
| Status Dot | 8px circle; online: green + pulse (scale 1→1.6 opacity 1→0, 1.5s infinite); offline: solid red |
| Number Animation | Value change: 300ms count-up (rAF) |
| Charts | ECharts animationDuration 300, areaStyle gradient (primary color 0.25→0), line glow (shadowBlur 8) |
| Danger Pulse | Danger state: card border + numbers turn red, border glow 1.2s infinite pulse |
| Scanline | Subtle page-level scanline (2px gradient bar, 8s cycle, opacity 0.03) — optional, can be disabled |
| Event Stream | New events slide in from top (200ms), auto-scroll to bottom (if user is at bottom) |

## 2. Portal Page (/)

### 2.1 Layout

```
┌────────────────────────────────────────────────────────────────┐
│ ◉ llama-lens          Host 3 · Online 2 · Token Speed 42.1 tok/s    │  BrandBar 56px
├────────────────────────────────────────────────────────────────┤
│ ┌──────────────────────┐  ┌──────────────────────┐             │
│ │ ● AI Host (ai.lan)   │  │ ● Host2 (10.0.0.2)   │   Grid:     │
│ │ Qwen3.8-27B-Q6_K     │  │ Llama-3.1-8B         │   auto-fill │
│ │ 27.32B Parameters    │  │ 8.0B Parameters      │   minmax(360px,1fr)│
│ │                      │  │                      │             │
│ │ 42.3 tok/s  ~~~spark │  │ 128.5 tok/s ~~~spark │             │
│ │ GPU0 ▓▓▓▓▓▓▓░ 87%   │  │ GPU0 ▓▓▓░░░░░ 32%   │             │
│ │ GPU1 ▓▓▓▓▓▓▓▓ 92%   │  │ (single card)        │             │
│ │ CPU 12%  MEM 19%    │  │ CPU 45%  MEM 38%    │             │
│ └──────────────────────┘  └──────────────────────┘             │
└────────────────────────────────────────────────────────────────┘
```

### 2.2 HostCard Fields

| Area | Content | Data Source |
|---|---|---|
| Top | Status dot + hostname + id | /api/hosts |
| Model | Model name (short name) + parameter count | model.name / n_params |
| Speed | gen_speed_tps large number + 60s sparkline | gen_speed_tps + history |
| GPU | One row per card: utilization% + VRAM% | gpus[] |
| Bottom | CPU% / MEM% / Disk% | cpu_pct / mem_pct |
| Alerts | Red border + red badge (count) in top-right corner when danger alerts exist | alerts |

### 2.3 Interactions

- Card hover: border brightens, translateY(-2px)
- Click: navigate to /host/:id
- Refresh: WS /ws/portal 1s push
- Offline card: grayed out + red "offline" badge

## 3. Detail Page (/host/:id)

### 3.1 Overall Layout (1920×1080 baseline)

```
┌ TopBar (56px) ────────────────────────────────────────────────────────────┐
│ ← Portal | ai.lan · Qwen3.8-27B-Q6_K | Speed 53.3t/s | MTP 83.3% | Context 224k/262k │
│ GPU0 48% 58° 223W 16.9/20G | GPU1 … | Memory 10.1/31.3G | CPU 18% | ● Online | WS Live ▾ │
├────────────────────────────────────────────────────────────────────────────┤
│ Real-time Overview (4 cards, 170px) —— 3 bar cards + 1 gauge                                      │
│ ┌Token Generation Speed─┐ ┌Prefill Speed────────┐ ┌Context Usage────────┐ ┌MTP Acceptance Rate─────┐ │
│ │ 42.3 tok/s    │ │ 123.4 tok/s   │ │ 85,106        │ │  ◜ 69.8% ◝  │ │
│ │ [bar]          │ │ [bar]          │ │ [bar]          │ │ (gauge)       │ │
│ │ [sparkline]   │ │ [sparkline]   │ │ [sparkline]   │ │ [sparkline]  │ │
│ └───────────────┘ └───────────────┘ └───────────────┘ └──────────────┘ │
├────────────────────────────────────────────────────────────────────────────┤
│ GPU Area (aggregated per card, two cards side by side) —— above real-time task area (GPUs are more prominent)         │
│ ┌─ GPU 0 · RTX 3080 ──────────────┐ ┌─ GPU 1 · RTX 3080 ──────────────┐   │
│ │ Ring gauge 87%    VRAM bar       │ │ Ring gauge 92%    VRAM bar       │   │
│ │ 18.4/20 GiB (90%)               │ │ 17.6/20 GiB (86%)               │   │
│ │ 54°C  141.5W/280W  Fan 60%     │ │ 49°C  137.3W/280W  Fan 55%     │   │
│ │ Clock 1440MHz  VRAM 1188MHz       │ │ Clock 1425MHz  VRAM 1188MHz       │   │
│ │ PCIe gen3 x16 · P2 · Driver 580.159.03 │ PCIe gen3 x4 · P2 · Driver 580.159.03│  │
├────────────────────────────────────────────────────────────────────────────┤
│ Real-time Generation Task Area (2 cards, both fixed height 320px) —— Left: "what llama is doing now" / Right: event stream (scrollable) │
│ ┌Current Status (640px)───────────────────────┐ ┌Event Stream (1fr, fill)────────────┐  │
│ │ ● Generating (decoding) · Task #14847     │ │ 15:53:10 [INFO] llama online…  │  │
│ │ Decoded 2843 · Live 28.5 t/s (3s window)  │ │ 16:02:33 [INFO] Task started…    │  │
│ │ Task avg 26.2 t/s · MTP 83.3%        │ │ 16:41:07 [INFO] Task ended…    │  │
│ └───────────────────────────────────────┘ └────────────────────────────┘  │
│ │ Process: llama-server 18414MiB      │ │ Process: llama-server 17626MiB      │   │
│ └──────────────────────────────────┘ └──────────────────────────────────┘   │
├────────────────────────────────────────────────────────────────────────────┤
│ System Area (4 cards, 200px)                                                        │
│ ┌CPU──────────────┐ ┌Memory────────────┐ ┌Disk──────────────┐ ┌Network──────────┐  │
│ │ 12.3% (large)   │ │ 5.8/31 GiB      │ │ / 70%           │ │ eth0            │  │
│ │ 6 bars ×6       │ │ used bar          │ │ bar 801G/1.2T     │ │ rx 0.5MB/s    │  │
│ │ load 1.09/0.75/ │ │ buff/cache 25G    │ │ R 1.2 W 0.3       │ │ tx 0.2MB/s    │  │
│ │ 0.58  3.0GHz    │ │ swap 0/8Gi        │ │                   │ │ [mini chart]  │  │
│ └──────────────────┘ └───────────────┘ └───────────────┘ └──────────────┘  │
├────────────────────────────────────────────────────────────────────────────┤
│ Process Area (280px)                                                              │
│ ┌llama-server Process Card (2/3 width)──────────────┐ ┌Top CPU (1/3)──────┐         │
│ │ PID 31255 | Live CPU 45.2% | Cumulative 98.6%   │ │ llama-server 98.6% │        │
│ │ RSS 5.1G | VSZ 12G | Threads 10 | Uptime 02:28│ │ ...                │         │
│ │ Service unit·active / Started / CPU / Memory / Tasks│ │ (8 rows)          │         │
│ │ [command line fold ▾]                           │ ├───────────────────┤         │
│ │ -m ... --n-gpu-layers all -ts 24,20 ...  │ │ Top Memory (8 rows)   │         │
│ │ [parameter table: 20 parsed items]                   │ └───────────────────┘         │
│ └──────────────────────────────────────────┘                               │
├────────────────────────────────────────────────────────────────────────────┤
│ Model & Slot Area (240px)                                                      │
│ ┌Model Info (1/2)──────────────────┐ ┌Slot Details (1/2)──────────────────┐     │
│ │ Path /share/AI/.../Qwen3.8-... │ │ Slot 0 · Running · Task #5199    │     │
│ │ Q6_K · 27.32B · 22.87GB       │ │ prompt 19032 (processed 626/cache 0)  │     │
│ │ embd 5120 · vocab 248320      │ │ Decoded 795 · Remaining 31205         │     │
│ │ vision+video · ctx 262144     │ │ Context 7.6% [bar]                │     │
│ │ mmproj 889MB · llamacpp       │ │ Sampling: temp 0.3 top_k 40 top_p .9│     │
│ │ [full params table collapsible]  │ │ max_tokens 32000 · spec draft-mtp│    │
│ └────────────────────────────────┘ │ [full sampling params table collapsible]            │     │
│                                   └─────────────────────────────────┘     │
├────────────────────────────────────────────────────────────────────────────┤
│ Trend Area (12 charts, 3 groups, each chart 170px, window toggle 5m/15m/1h)                          │
│ [llama] ┌Token Generation Speed┐ ┌Prefill Speed─┐ ┌Context Usage (1s line+n_ctx ref line)┐ ┌MTP (staircase)┐ │
│ [GPU]   ┌Utilization────┐ ┌VRAM──────────┐ ┌Temperature────────┐ ┌Power──────────┐ │
│ [System]   ┌CPU─────────┐ ┌Memory────────┐ ┌Network (rx/tx)┐ ┌Avg Load (1/5/15)┐ │
└────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 TopBar

| Element | Description |
|---|---|
| Back button | ← Portal (router back to /) |
| Host identifier | name + model short name (basename, hover title shows full path) |
| Core metric chips | Single-line compact number chips (horizontal scroll when space is insufficient): speed (t/s, prompt-active priority display, tooltip contains gen/prompt/source), MTP acceptance rate %, context remaining/total (k), per GPU utilization/temperature/power/VRAM used/total, memory used/total, CPU %; colored by alerts (warn yellow / danger red), grayed when no data |
| Status badge | ● Online (green pulse) / llama offline (red) / SSH disconnected (yellow) |
| Refresh control | Dropdown: Live (WS) / 1s / 2s / 5s / Pause; current mode indicator (WS green dot / polling yellow dot / pause gray) |

### 3.3 Real-time Overview Area (3 bar cards + 1 gauge)

| Card | Shape | Data | Auxiliary (right of title row) | Bottom row | Color |
|---|---|---|---|---|---|
| Token Generation Speed | BarCard bar | llama.gen_speed_tps | Source log/API; when offline "Data as of HH:MM:SS" | Task avg speed (log.state.tg_tps) + 60s peak/val/avg | Grayed when offline |
| Prefill Speed | BarCard bar | Real-time prefill value; returns to 0 after stop (last prefill info in sub "last HH:MM:SS" and foot tokens/progress) | Same as above; during non-prefill shows "last HH:MM:SS" | During prefill: progress % + estimated remaining (ETA=(1-progress)×total÷speed), bar switches to progress bar (green, ticks 0/50/100%); non-prefill: total prompt tokens + progress % + 60s peak/val/avg (all-zero window hidden) | Grayed when offline |
| Context Usage | BarCard bar (0–ctx.total) | ctx.used raw token count (thousands separator, e.g. 85,106) | Percentage · remaining (32.5% · remaining 177k) | 60s peak/val/avg (k format) | alerts `ctx` level; truncated shows red "truncated" badge |
| MTP Acceptance Rate | GaugeCard gauge | mtp.acceptance × 100 | spec_type×spec_draft_n_max · accepted/generated · mean len | 60s peak/val/avg | Value-driven color (<65 red / 65-80 yellow / ≥80 green, arc/needle/anchor/center number/60s trend all follow) + alerts `mtp` level |

- Bar card shape: large number (24px count-up) + horizontal bar + 0/median/full ticks + 60s sparkline (1.2px constant line width) + bottom row 60s peak/val/avg
- Bar full scale: speed cards dynamic (60s peak × 1.1 rounded to 1/2/2.5/5×10^n, floor 10); context card fixed ctx.total (ticks 0/median/full in k format)
- Gauge shape: ECharts gauge (240° arc, 12px track, threshold color bands, major/minor ticks, thin needle + anchor), center large number 300ms count-up
- MTP color band reversed (0-65 red / 65-80 yellow / 80-100 green, low is bad); value-driven color provided by zoneColors (same source as band thresholds)
- No data (llama offline / no tasks): bars zero, number "—" grayed, trends hidden
- Grid: >1500px 4 columns; ≤1500px 2 columns; ≤640px 1 column

### 3.4 Real-time Generation Task Area (LlamaStateCard + EventFeed)

- Two-column layout (640px + 1fr), both cards fixed height 320px (`.task-row > * { height: 320px }`): left LlamaStateCard answers "what is llama doing now" (field specs in §3.5), content overflows with card-internal scroll (overflow-y: auto); right EventFeed (fill mode, see §3.11)
- Fixed height purpose: phase transitions (idle/prefill/generating) have different content heights, avoiding layout jumps
- ≤1500px falls back to 1 column (both cards still fixed 320px each)

### 3.5 LlamaStateCard (Current Status Card)

- Header: phase badge + live log badge + right-side "running for" (now - state.started_at, ticks every second with snapshot)
- Three-column large number stat grid (stat3, dynamic values):
  - Generating: decoded (incrementing) / live speed 3s (fluctuating) / task avg speed
  - Prompt processing: prompt speed / processed prompts (n_prompt_tokens_processed, incrementing) / elapsed time (prompt_elapsed_s)
- Progress bar: prefill = prompt progress; generating = n_decoded/(n_decoded+n_remain) (only shown when n_remain>0, i.e. upper limit is set)
- Generating: estimated completion ETA (n_remain ÷ tg_3s, only n_remain>0) + two-column kv (context usage / MTP acceptance rate (log.mtp, task_end updates, <65 red / 65-80 yellow / ≥80 green, with accepted/generated) / remaining tokens (n_remain, "-1" shows "unlimited") / total prompts / cache hits / Graphs reuse (log.graphs_reused))
- Prompt processing: progress bar + two-column kv (total prompts / estimated remaining ETA / context usage / KV cache hits)
- Idle: context usage + previous task summary (tokens/avg speed/MTP) + two-column kv (total elapsed / prefill elapsed@speed / generate elapsed / Graphs reuse)
- Bottom config strip (cfg-strip, from process.flags static config): Spec decoding (spec_type×spec_draft_n_max) / KV cache (cache_type_k/v) / batch size (batch/ubatch) / GPU layers
- Task #id, is_child, slot id/total (active slots / total slot count)
- Data: log.state.* (including started_at) / log.context (used/total/pct) / log.mtp (acceptance/accepted/generated) / log.graphs_reused / active slots (is_processing: id, n_prompt_tokens, n_prompt_tokens_cache → cache hit = cache/total, n_prompt_tokens_processed, n_remain) / process.flags
- MTP acceptance rate shown during generating is the previous task's value (log.mtp only updates at task_end, same source as overview gauge)

### 3.6 GPU Panel (one per card, GpuPanel)

| Area | Content | Thresholds |
|---|---|---|
| Header | GPU chip icon (cyan glow) + GPU {index} · {name} | — |
| Left | Ring gauge (88px compact): utilization % (0-100) | ≥80 yellow, ≥90 red |
| Top-right | VRAM bar: used/total GiB + % | ≥85 yellow, ≥95 red |
| Right | 3×4 compact grid (kv 11px): temperature / VRAM temp / power (draw/limit) / VRAM utilization / fan / clock (core/VRAM) / PCIe (gen x width) / P-State / clock-throttle status (bitmask → Chinese short names, 0=normal) / ECC (corrected/uncorrected) / CUDA version / driver version | Temp ≥75 yellow ≥85 red; power ≥85% yellow ≥95% red; ECC uncorrected >0 red; clock-throttle active yellow |
| Bottom | Occupying process list (pid name mem) | — |

### 3.7 System Area

- CpuPanel: whole-machine % large number + 6 vertical bars (per core, height=utilization, color cyan→red gradient) + load 1/5/15 + clock speed; thresholds ≥80 yellow ≥90 red
- MemPanel: used/total large number + three bars (used / buff_cache / available stacked) + swap; thresholds ≥85 yellow ≥95 red
- DiskPanel: one bar per mount point (mount, used/total, %) + read/write speed; thresholds ≥80 yellow ≥90 red
- NetPanel: per NIC rx/tx speed + 60s mini chart

### 3.8 Process Area

- LlamaProcessCard:
  - First row: 4-column metric grid (7 items, 4+3 layout): PID | live CPU% (large number) | cumulative CPU% | RSS | VSZ | thread count | uptime
  - Second row: systemd key-value list (label left, value right, 5 rows): service unit · active / start time / service-level CPU cumulative / service-level memory (including peak) / Tasks
  - Command line: monospace font, expanded by default (pre-wrap, clickable to collapse)
  - Parameter table: three-column grid (parameter name → value), 20+ items, collapsible (expanded by default)
- TopProcessTable: two tables (Top CPU / Top Memory), each 8 rows: pid name cpu% mem% rss (PID and numeric columns right-aligned, process name truncated with ellipsis on overflow, full name on hover)

### 3.9 Model & Slot Area

- ModelInfoCard: key-value grid (name/path/ftype/parameters/file size/mmproj/embedding dim/vocab/context/modalities/capabilities/owned_by)
- SlotTable:
  - Header: Slot {id} · {running/idle} · Task #{id_task}
  - prompt tokens: total / processed / cache (3 numbers)
  - decoded / remaining / context usage bar
  - Log-derived: prompt progress (log), task elapsed summary (prompt/decode/total elapsed and speed), MTP acceptance rate, KV retention f_keep
  - Sampling parameters table (collapsible, default shows temperature/top_k/top_p/min_p/max_tokens/chat_format/speculative.types, full 30+ items expandable)

### 3.10 Trend Area (TrendChart × 12, 3 groups)

12 charts divided into 3 groups (each group one row with 2-column thin labels, related metrics adjacent):

| Group | Chart | Series | Type |
|---|---|---|---|
| llama | Token Generation Speed | gen_speed (cyan) | Line + area gradient |
| llama | Prefill Speed | prompt_speed (green) | Line + area gradient |
| llama | Context Usage | ctx_used (cyan, 1s sampling) + n_ctx upper limit (red dashed reference line) | Line + reference line |
| llama | MTP Acceptance Rate | per-task acceptance rate (green, staircase line, y-axis 0-100%) | Staircase line |
| GPU | GPU Utilization | GPU0 (cyan), GPU1 (green) | Line |
| GPU | GPU VRAM | GPU0, GPU1 (GiB) | Line |
| GPU | GPU Temperature | GPU0, GPU1 (°C) | Line |
| GPU | GPU Power | GPU0, GPU1 (W) | Line |
| System | CPU | Whole machine (cyan) | Line + area |
| System | Memory | used (cyan) + buff_cache (blue, stacked) | Stacked area |
| System | Network | rx (cyan), tx (green) | Line |
| System | Avg Load | load_1 (cyan), load_5 (green), load_15 (yellow) | Line |

- Common: window toggle 5m/15m/1h (global toggle in top-right of trend area); y-axis auto-scale; tooltip shows all series values + timestamp; broken lines on nulls (connectNulls: false)

### 3.11 Event Stream (EventFeed)

- Position: right card of real-time generation task area (fill mode, fixed height 320px, feed-box flex:1 + overflow-y: auto, scrollbar appears when content overflows); ≤1500px falls back to 1 column, still fixed 320px
- Terminal style: monospace font, dark background, level-tag coloring [INFO] green / [WARN] yellow / [ERROR] red
- Event sources: backend EventDetector (primarily log lines): task start (task ID/prompt tokens), prefill start (total prompt tokens), task end (total tokens/elapsed/avg speed/MTP acceptance/context usage), threshold crossing (alert: metric value ≥/≤ threshold (warning/danger) / restored to normal, minimum 30s interval between two events for the same metric), boot (model loaded/boot info), online/offline
- Auto-scroll: follows new events when user is at bottom; shows "↓ New Events" button after scrolling up
- Retains 200 entries, older ones fade out

## 4. Component Inventory

| Component | File | Responsibility | Key Props/Data |
|---|---|---|---|
| BrandBar | components/BrandBar.vue | Portal top bar | hosts_total, online_count |
| HostCard | components/HostCard.vue | Portal host card | host snapshot summary |
| TopBar | components/TopBar.vue | Detail top bar | host, model, stats (speed/MTP/context/GPU/memory/CPU), status, refresh control |
| BarCard | components/BarCard.vue | Overview bar card (large number + bar + ticks + 60s trend + peak/val/avg) | title, value, unit, digits, level, barMax, spark[], sub, badge, foot, fmt, fmtCompact |
| GaugeCard | components/GaugeCard.vue | Overview gauge card (threshold color bands + ticks + needle + 60s trend + peak/val/avg) | title, value, unit, level, sub, spark[], foot, zones, zoneColors |
| LlamaStateCard | components/LlamaStateCard.vue | Current status card (phase/progress/speed/context/cache hits/ETA) | log.state, log.context, slots |
| GpuPanel | components/GpuPanel.vue | Single GPU panel | gpu object |
| CpuPanel | components/CpuPanel.vue | CPU panel | cpu object |
| MemPanel | components/MemPanel.vue | Memory panel | mem object |
| DiskPanel | components/DiskPanel.vue | Disk panel | disk object |
| NetPanel | components/NetPanel.vue | Network panel | net object |
| LlamaProcessCard | components/LlamaProcessCard.vue | Process card | process, service |
| TopProcessTable | components/TopProcessTable.vue | Top process table | rows, mode(cpu/mem) |
| ModelInfoCard | components/ModelInfoCard.vue | Model info | model |
| SlotTable | components/SlotTable.vue | Slot details | slot |
| TrendChart | components/TrendChart.vue | General trend chart | series[], window |
| EventFeed | components/EventFeed.vue | Event stream (task area right card, fill mode) | events[], fill |

(17 components total, this table is authoritative)

## 5. Interaction Specifications

| Interaction | Specification |
|---|---|
| Refresh control | TopBar dropdown: Live (WS) / 1s / 2s / 5s / Pause; mode indicator dot: WS green / polling yellow / pause gray; when paused, data requests stop, page retains last data + "paused" watermark |
| Window toggle | Global toggle in trend area 5m/15m/1h, all charts sync |
| Hover | Chart tooltip (all series + timestamp); card hover border brightens; progress bar hover shows precise value |
| Collapse | Command line, parameter table, sampling parameters table: click header to expand/collapse, 200ms height animation |
| Event stream | Auto-scroll + "new events" button (see 3.9) |
| Navigation | Portal card click → detail; detail ← back → portal (preserves scroll position) |
| Number animation | Value change 300ms count-up; large numbers font-variant-numeric: tabular-nums to prevent jitter |

## 6. Threshold Color-Change Specification

| Level | Number Color | Card Border | Additional Effects |
|---|---|---|---|
| normal | #e6f1ff | rgba(0,229,255,0.15) | — |
| warn (yellow) | #ffc53d | rgba(255,197,61,0.4) | — |
| danger (red) | #ff3b5c | rgba(255,59,92,0.6) + glow 0 0 16px rgba(255,59,92,0.5) | Border pulse 1.2s infinite; portal card top-right red badge |

- Evaluation is done in the backend (snapshot.alerts[]), frontend renders by level
- Multiple metrics exceeding thresholds simultaneously: each metric is independent; card takes the highest level

## 7. Degradation & Empty States

| State | Display |
|---|---|
| llama offline | TopBar red badge "llama offline"; overview area speed card "—" grayed + "data as of HH:MM:SS" (sub row); GPU/system area normal (SSH still available) |
| SSH disconnected | TopBar yellow badge "SSH disconnected"; GPU/system/process areas show "data unavailable (SSH disconnected)" placeholder (icon + text), retain last values grayed; real-time overview/generation task area normal (API data still available) |
| Logs unavailable (SSH disconnected / unit name mismatch) | Real-time generation task card shows "logs unavailable, using API data" grayed; speed falls back to /slots diff with data source labeled API |
| Both disconnected | Full-page red banner "host unreachable", all areas as placeholders |
| No GPU | GPU area shows "no NVIDIA GPU detected" |
| Single GPU | GPU area single card fills entire row |
| Multiple Slots | Slot area one card per slot (grid) |
| No process | Process card shows "no llama-server process found (may be running under a different name)" |
| First load | Skeleton screen (card outline shimmer), data populated after WS first frame arrives |

## 8. Responsive

- Baseline 1920×1080; content fills full width (no max-width restriction), no horizontal scroll at any width
- Detail page breakpoints:

| Breakpoint | Overview | Task | GPU | System | Model/Slot | Process | Trend |
|---|---|---|---|---|---|---|---|
| >1500px | 4 columns | 2 columns (640px+1fr) | 2 columns | 2 columns | 2 columns | 2:1 | 2 columns |
| ≤1500px | 2 columns | 1 column | 1 column | 2 columns | 1 column | 1 column | 2 columns |
| ≤1100px | 2 columns | 1 column | 1 column | 1 column | 1 column | 1 column | 1 column |
| ≤640px | 1 column | 1 column | 1 column | 1 column | 1 column | 1 column | 1 column |

- Portal page: auto-fill minmax(360px, 1fr), single column <360px
- Sparklines use vector-effect: non-scaling-stroke, constant line width when cards stretch (overview cards 1.2px / portal cards 1.5px)
