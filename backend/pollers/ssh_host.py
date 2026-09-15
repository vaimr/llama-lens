"""SSH 只读采集器（SshPoller）。

- 每 2s 一条批量只读命令（一次网络往返），解析 GPU/CPU/内存/磁盘/网络/进程/服务/模型。
- 静态信息（hostname/内核/OS/CPU 型号/核数）首次连接采集一次。
- 差分指标（CPU/进程 CPU/磁盘/网络速率）由 DiffEngine 基于连续两次采样计算。
- 全部只读，不修改被监控主机任何配置。兼容 Python 3.9。
"""
import asyncio
import logging
import json
import re
import time
from typing import Optional, Dict, Any, List

from ..config import HostConfig
from ..diff import DiffEngine
from ..events import EventDetector
from ..store import RingBuffer
from .ssh_conn import SshConnection

log = logging.getLogger("llamalens.sshpoller")

# ---------------------------------------------------------------------------
# 批量命令模板（占位符：{process_name} {systemd_unit} {df_mounts}）
# ---------------------------------------------------------------------------

BATCH_CMD = r"""
echo ==GPU==
nvidia-smi --query-gpu=index,name,driver_version,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,power.limit,fan.speed,clocks.current.graphics,clocks.current.memory,pcie.link.gen.current,pcie.link.width.current,pstate,temperature.memory,ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total,clocks_throttle_reasons.active --format=csv,noheader,nounits
echo ==APPS==
nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits
echo ==AMDGPU==
if [ -n "$(command -v amd-smi)" ]; then amd-smi metric --json; elif [ -n "$(command -v rocm-smi)" ]; then rocm-smi --showallinfo --json; fi
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
awk '$3 ~ /^(sd|vd|nvme)/ && $3 !~ /p[0-9]+$/ && $3 !~ /^[sv]d[a-z]+[0-9]+$/ {{print}}' /proc/diskstats
echo ==DF==
df -B1 --output=source,target,size,used,avail,pcent {df_mounts}
echo ==PROC==
# Собираем ВСЕ llama-server процессы (не только первый)
for pid in $(pgrep -x "{process_name}"); do
  IFS=$(awk '{{print $14, $15, $23}}' /proc/$pid/stat)
  VmRSS=$(grep '^VmRSS:' /proc/$pid/status | awk '{{print $2}}')
  VmSize=$(grep '^VmSize:' /proc/$pid/status | awk '{{print $2}}')
  Threads=$(grep '^Threads:' /proc/$pid/status | awk '{{print $2}}')
  ps_out=$(ps -o pcpu=,pmem=,etime= -p $pid)
  cmdline=$(tr '\0' ' ' < /proc/$pid/cmdline | tr '\n' ' ')
  echo "PROC_PID:$pid"
  echo "PROC_STATS:$IFS"
  echo "PROC_RSS:$VmRSS"
  echo "PROC_VSZ:$VmSize"
  echo "PROC_THREADS:$Threads"
  echo "PROC_PS:$ps_out"
  echo "PROC_CMDLINE:$cmdline"
done
echo "PROC_END:"
echo ==PS==
ps -eo pid,comm,pcpu,pmem,rss --no-headers
echo ==PSTICKS==
awk 'FNR==1 {{ n=split(FILENAME, p, "/"); pid=p[n-1]; i=index($0, ") "); if (i > 0) {{ s=substr($0, i+2); m=split(s, a, " "); if (m >= 13) print pid, a[12], a[13] }} }}' /proc/[0-9]*/stat
echo ==PROCS==
ls /proc | grep -c '^[0-9]'
echo ==SERVICE==
systemctl show {systemd_unit} -p Description,ActiveState,SubState,ExecMainStartTimestamp,CPUUsageNSec,MemoryCurrent,MemoryPeak,NTasks
echo ==MODELS==
# Собираем модель и mmproj для ВСЕХ llama-server процессов
for pid in $(pgrep -x "{process_name}"); do
  # Читаем cmdline как единую строку с разделителем пробел
  cmdline_raw=$(tr '\0' ' ' < /proc/$pid/cmdline)
  
  # Извлекаем model path (ищем --model или -m и берём следующее слово)
  model=""
  mmproj=""
  
  # Используем awk для надёжного парсинга без Perl regex
  model=$(echo "$cmdline_raw" | awk '{{
    for (i=1; i<=NF; i++) {{
      if ($i == "--model" || $i == "-m") {{
        print $(i+1)
        exit
      }}
    }}
  }}')
  
  mmproj=$(echo "$cmdline_raw" | awk '{{
    for (i=1; i<=NF; i++) {{
      if ($i == "--mmproj") {{
        print $(i+1)
        exit
      }}
    }}
  }}')
  
  if [ -n "$model" ]; then
    echo "MODEL:$pid:$model"
  fi
  if [ -n "$mmproj" ]; then
    echo "MMPROJ:$pid:$mmproj"
  fi
done
echo ==END==
"""

STATIC_CMD = r"""
echo ==HOSTNAME==
hostname
echo ==KERNEL==
uname -r
echo ==OS==
grep '^PRETTY_NAME=' /etc/os-release | cut -d= -f2- | tr -d '"'
echo ==CPUMODEL==
grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | sed 's/^ *//'
echo ==CORES==
grep -c '^processor' /proc/cpuinfo
echo ==MHZ==
grep -m1 'cpu MHz' /proc/cpuinfo | awk '{print $4}'
echo ==NVSMI==
command -v nvidia-smi || echo NOT_FOUND
echo ==CUDA==
# CUDA 版本静态（驱动不升级不变）：一次性采集，避免每 2s 渲染整张 nvidia-smi 表
nvidia-smi | grep -m1 'CUDA Version'
echo ==AMDSMI==
command -v amd-smi || command -v rocm-smi || echo NOT_FOUND
echo ==AMDSTATIC==
if [ -n "$(command -v amd-smi)" ]; then amd-smi static --json; elif [ -n "$(command -v rocm-smi)" ]; then rocm-smi --showallinfo --json; fi
echo ==END==
"""


def _f(x, default=None):
    """安全转 float。"""
    try:
        return float(x)
    except (ValueError, TypeError):
        return default


def _i(x, default=None):
    """安全转 int。"""
    try:
        return int(float(x))
    except (ValueError, TypeError):
        return default


# ---------------------------------------------------------------------------
# 分段解析
# ---------------------------------------------------------------------------

def split_sections(output: str) -> Dict[str, str]:
    """把 ==SECTION== 分隔的输出切成 {section: content}。"""
    sections: Dict[str, str] = {}
    current = None
    buf: List[str] = []
    for line in output.splitlines():
        s = line.strip()
        if s.startswith("==") and s.endswith("==") and len(s) > 4:
            if current is not None:
                sections[current] = "\n".join(buf).strip("\n")
            current = s[2:-2]
            buf = []
        elif current is not None:
            buf.append(line)
    if current is not None:
        sections[current] = "\n".join(buf).strip("\n")
    return sections


def _parse_throttle(raw) -> int:
    """clocks_throttle_reasons.active：'Not Active'/十进制/0x 十六进制 → 位掩码 int（0=无降频）。"""
    s = (raw or "").strip()
    if not s or s.upper().startswith("NOT"):
        return 0
    try:
        return int(s, 16) if s.lower().startswith("0x") else int(s)
    except ValueError:
        return 0


RE_SMI_CUDA = re.compile(r"CUDA Version:?\s*(\d+\.\d+)")


def parse_smi_cuda(section: str) -> Optional[str]:
    """nvidia-smi 表头行（第 3 行）→ CUDA 版本（如 '13.0'），无匹配 None。

    cuda_version 查询字段部分驱动不支持（580.x 报 not a valid field，整条查询失败），
    改从表头行解析。
    """
    m = RE_SMI_CUDA.search(section or "")
    return m.group(1) if m else None


def parse_gpu(section: str) -> List[Dict[str, Any]]:
    gpus = []
    for line in section.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = [p.strip() for p in line.split(",")]
        if len(parts) < 21:
            continue
        fan = _f(parts[11])
        gpus.append({
            "index": _i(parts[0], 0),
            "name": parts[1],
            "driver": parts[2],
            "mem_total_mb": _i(parts[3]),
            "mem_used_mb": _i(parts[4]),
            "mem_free_mb": _i(parts[5]),
            "util_pct": _f(parts[6], 0.0),
            "mem_util_pct": _f(parts[7], 0.0),
            "temp_c": _f(parts[8]),
            "power_w": _f(parts[9]),
            "power_limit_w": _f(parts[10]),
            "fan_pct": None if (fan is None or fan < 0) else fan,
            "clock_mhz": _i(parts[12]),
            "mem_clock_mhz": _i(parts[13]),
            "pcie_gen": _i(parts[14]),
            "pcie_width": _i(parts[15]),
            "pstate": parts[16],
            "temp_mem_c": _f(parts[17]),
            "ecc_corrected": _i(parts[18]),
            "ecc_uncorrected": _i(parts[19]),
            "throttle": _parse_throttle(parts[20]),
            "apps": [],
        })
    return gpus


def parse_apps(section: str) -> List[Dict[str, Any]]:
    apps = []
    for line in section.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = [p.strip() for p in line.split(",")]
        if len(parts) < 3:
            continue
        pid = _i(parts[0])
        mem = _i(parts[-1])
        name = ",".join(parts[1:-1]).strip()
        if name:
            name = name.rsplit("/", 1)[-1]
        apps.append({"pid": pid, "name": name, "mem_mb": mem})
    return apps


def _safe_num(v, default=None):
    """Convert a value (int/float/str/list/dict) to float, never crash.

    Handles: bare numbers, strings with units ("123.4 MHz"),
    lists (take first element), "N/A" → default,
    dicts like {"value": 512, "unit": "MB"} → extract "value",
    percent strings like "42%" → strip % and parse.
    """
    if v is None:
        return default
    if isinstance(v, dict):
        # {"value": 512, "unit": "MB"} → recurse on "value"
        if "value" in v:
            return _safe_num(v["value"], default)
        return default
    if isinstance(v, (int, float)):
        return float(v) if not isinstance(v, bool) else default
    if isinstance(v, list):
        return _safe_num(v[0], default) if v else default
    s = str(v).strip()
    if not s or s.upper() in ("N/A", "NA", "NULL", "NONE", "UNKNOWN", "NOT FOUND"):
        return default
    # Strip trailing % (e.g. "42%" → "42")
    if s.endswith("%"):
        s = s[:-1]
    # Strip common unit suffixes: "123.4 MHz" → 123.4
    s = s.split()[0]
    try:
        return float(s)
    except (ValueError, TypeError):
        return default


def _safe_int(v, default=0):
    """Convert a value to int via float, never crash."""
    f = _safe_num(v, None)
    if f is None:
        return default
    return int(f)


def _pick(d: dict, *keys, default=None):
    """Try multiple keys in order, return first non-None value."""
    if not isinstance(d, dict):
        return default
    for k in keys:
        if k in d:
            return d[k]
    return default


def _deep_pick(d: dict, dotted: str, default=None):
    """Walk a dotted path like 'temperature.edge.value' via .get().
    Returns default on any miss.
    """
    if not isinstance(d, dict):
        return default
    keys = dotted.split(".")
    cur = d
    for k in keys:
        if isinstance(cur, dict) and k in cur:
            cur = cur[k]
        else:
            return default
    return cur if cur is not None else default


def parse_amd_gpu(section: str, static_map: Optional[Dict[int, Dict[str, Any]]] = None) -> List[Dict[str, Any]]:
    """Parse amd-smi metric --json output.

    Tolerant container discovery, never raise.  Mirrors parse_gpu keys
    so the frontend panel renders identically.

    Args:
        section: raw JSON string from amd-smi metric --json.
        static_map: {gpu_index: static_entry} from amd-smi static --json,
                    used to resolve GPU names (metric JSON has no name).
    """
    gpus: List[Dict[str, Any]] = []
    if not section.strip():
        return gpus
    try:
        root = json.loads(section)
    except (json.JSONDecodeError, TypeError):
        return gpus
    if not isinstance(root, dict):
        return gpus

    # --- Container discovery ---
    candidates = []
    for key in ("gpu_data", "gpu_static_data", "gpu", "gpus", "GPUTOP"):
        val = root.get(key)
        if isinstance(val, list):
            candidates = val
            break
    if not candidates:
        # Fallback: iterate top-level values, collect lists of dicts
        for val in root.values():
            if isinstance(val, list) and val and isinstance(val[0], dict):
                # Skip version keys
                sample = val[0]
                if not any(k in sample for k in ("rocm_smi_version", "amdsmi_version", "version")):
                    candidates = val
                    break

    for idx, gpu in enumerate(candidates):
        if not isinstance(gpu, dict):
            continue
        try:
            static_entry = static_map.get(idx) if static_map else None
            entry = _extract_amd_gpu_entry(gpu, static=static_entry, fallback_idx=idx)
            gpus.append(entry)
        except Exception:
            continue  # skip unparseable entries

    return gpus


def _extract_amd_gpu_entry(gpu: dict, static: Optional[Dict[str, Any]] = None, fallback_idx: int = 0) -> Dict[str, Any]:
    """Extract a single GPU entry from amd-smi metric JSON.

    Uses static data (from amd-smi static --json) to resolve GPU name
    since metric JSON has no name field.

    Args:
        gpu: one entry from the metric JSON gpu_data list.
        static: corresponding entry from static JSON (gpu_static_data or gpu_data).
        fallback_idx: positional index used for fallback name.
    """
    # --- index ---
    idx = _safe_int(_pick(gpu, "gpu", "gpu_id", "index", "card_index", "gpu_index"), fallback_idx)

    # --- name (prefer static, fallback to gpu entry, then positional) ---
    name = str(
        _pick(
            static or {},
            "asic.market_name",
            "product_name",
            "name",
            "card_series",
            "drm_device_name",
            "device_name",
            "board.product_name",
        )
        or ""
    ).strip()
    if not name:
        name = str(
            _pick(
                gpu,
                "asic.market_name",
                "product_name",
                "name",
                "card_series",
                "drm_device_name",
                "device_name",
            )
            or ""
        ).strip()
    if not name:
        name = "AMD GPU %d" % fallback_idx

    # --- driver ---
    driver = ""
    if static:
        drv = _pick(static, "driver")
        if isinstance(drv, dict):
            ver = drv.get("version", "")
            driver = "%s %s" % (str(drv.get("name", "")).strip(), str(ver).strip()) if ver else str(drv.get("name", "")).strip()
        else:
            driver = str(drv or "").strip()
    if not driver:
        driver = str(
            _pick(gpu, "driver_version", "hardware.hwdriverversion", "asic.driver_version")
            or ""
        ).strip()

    # --- memory (MB) ---
    # Primary: amd-smi 26.2.2 metric format
    mem_total_mb = _deep_pick(gpu, "mem_usage.total_vram.value", None)
    mem_used_mb = _deep_pick(gpu, "mem_usage.used_vram.value", None)
    mem_free_mb = _deep_pick(gpu, "mem_usage.free_vram.value", None)

    if mem_free_mb is None and mem_total_mb is not None and mem_used_mb is not None:
        mem_free_mb = mem_total_mb - mem_used_mb

    # Fallbacks (rocm-smi legacy formats)
    if mem_total_mb is None:
        vram = _pick(gpu, "vram_usage")
        if isinstance(vram, dict):
            mem_total_mb = _safe_num(vram.get("vram_total"), None) / 1048576.0 if _safe_num(vram.get("vram_total"), None) is not None else None
            mem_used_mb = _safe_num(vram.get("vram_mem"), None) / 1048576.0 if _safe_num(vram.get("vram_mem"), None) is not None else None
        else:
            hw = _pick(gpu, "hardware")
            if isinstance(hw, dict):
                mem_total_mb = _safe_num(hw.get("hwmemtotal"), None) / 1024.0 if _safe_num(hw.get("hwmemtotal"), None) is not None else None
                mem_used_mb = _safe_num(hw.get("hwmemused"), None) / 1024.0 if _safe_num(hw.get("hwmemused"), None) is not None else None
            else:
                mem_total_mb = _safe_num(gpu.get("mem_total"), None)
                mem_used_mb = _safe_num(gpu.get("mem_used"), None)

    if mem_total_mb is None:
        mem_total_mb = 0.0
    if mem_used_mb is None:
        mem_used_mb = 0.0
    if mem_free_mb is None:
        mem_free_mb = mem_total_mb - mem_used_mb

    # --- util_pct ---
    # amd-smi metric: "usage" is a string like "N/A" or "42" or "42%"
    util = _deep_pick(gpu, "usage", None)
    if util is None:
        util = _pick(gpu, "gpu_use", "gpu_activity_percent.gpu", "utilization.gpu", "usage.gpu")
    if isinstance(util, str) and "%" in util:
        util_str = util.strip().replace("%", "").strip()
        util_pct = _safe_num(util_str, 0.0)
    else:
        util_pct = _safe_num(util, 0.0)

    # --- temp_c ---
    temp_c = _deep_pick(gpu, "temperature.edge.value", None)
    if temp_c is None:
        temp_c = _pick(
            gpu,
            "temperature.sensortemp",
            "temperature.edge",
            "Temperature (Sensor edge) (C)",
            "sensor_edge_temp",
        )
    temp_c = _safe_num(temp_c)

    # --- power_w ---
    power_w = _deep_pick(gpu, "power.socket_power", None)
    if power_w is None:
        power_w = _pick(gpu, "power.current_socket_power", "power.manchip.totalpower")
        if power_w is not None:
            power_w = _safe_num(power_w)
            # rocm-smi legacy: mW
            if "manchip" in str(power_w):
                power_w = power_w / 1000.0 if power_w is not None else None
    if power_w is not None:
        # "N/A" string → None
        pw_raw = _deep_pick(gpu, "power.socket_power", _pick(gpu, "power.current_socket_power", ""))
        if isinstance(pw_raw, str) and "N/A" in pw_raw.upper():
            power_w = None

    # --- power_limit_w ---
    ppl = _deep_pick(gpu, "power.pptlimit", None)
    if ppl is None:
        ppl = _pick(gpu, "power.pptlimit")
    power_limit_w = _safe_num(ppl)
    if power_limit_w is not None:
        ppl_str = str(ppl) if ppl is not None else ""
        if "N/A" in ppl_str.upper():
            power_limit_w = None

    # --- fan_pct ---
    fan_pct = _deep_pick(gpu, "fan.speed", None)
    if fan_pct is None:
        fan_pct = _pick(gpu, "fan_speed")
    fan_pct = _safe_num(fan_pct)

    # --- clock_mhz ---
    # amd-smi metric: clock.gfx_0.clk or clock.gfx_N.clk (string "2665MHz")
    clock_mhz = None
    clk = _deep_pick(gpu, "clock", None)
    if isinstance(clk, dict):
        # Try gfx_0, gfx_1, ... dynamically
        for k in sorted(clk.keys()):
            if k.startswith("gfx_"):
                sub = clk[k]
                if isinstance(sub, dict):
                    v = _safe_num(sub.get("clk"), None)
                else:
                    v = _safe_num(sub, None)
                if v is not None and str(v) != "0.0":
                    clock_mhz = v
                    break
    if clock_mhz is None:
        clock_mhz = _pick(gpu, "sclk", "Average Graphics Clock (MHz)")
    if clock_mhz is None:
        clk2 = _pick(gpu, "clocks_current_clk")
        if isinstance(clk2, dict):
            clock_mhz = _safe_num(clk2.get("gfxclk"), None)
        else:
            clock_mhz = _safe_num(clk2, None)
    if isinstance(clock_mhz, str) and "N/A" in str(clock_mhz).upper():
        clock_mhz = None

    # --- mem_clock_mhz ---
    mem_clock_mhz = _deep_pick(gpu, "clock.mem_0.clk", None)
    if mem_clock_mhz is None:
        # Try mem_1, mem_2 dynamically
        if isinstance(clk, dict):
            for k in sorted(clk.keys()):
                if k.startswith("mem_"):
                    sub = clk[k]
                    if isinstance(sub, dict):
                        v = _safe_num(sub.get("clk"), None)
                    else:
                        v = _safe_num(sub, None)
                    if v is not None:
                        mem_clock_mhz = v
                        break
    if mem_clock_mhz is None:
        mem_clock_mhz = _pick(gpu, "mem_clock_mhz", "clocks_current_clk.memclk", "mclk", "Average Memory Clock (MHz)")
    mem_clock_mhz = _safe_num(mem_clock_mhz)

    # --- pcie_gen / pcie_width ---
    pcie_gen = _deep_pick(gpu, "pcie.speed", None)
    if pcie_gen is None:
        pcie_gen = _pick(gpu, "pcie_gen", "PCIe Speed (GT/s)")
    pcie_gen = _safe_int(pcie_gen)

    pcie_width = _deep_pick(gpu, "pcie.width", None)
    if pcie_width is None:
        pcie_width = _pick(gpu, "pcie_width", "PCIe Width (Lanes)")
    pcie_width = _safe_int(pcie_width)

    # --- pstate ---
    pstate = str(_deep_pick(gpu, "perf_level", _pick(gpu, "pstate", "power_state") or "")).strip()

    # --- throttle ---
    throttle_raw = _pick(gpu, "throttle_reason", "throttle_reasons")
    throttle = _parse_throttle(str(throttle_raw) if throttle_raw is not None else "")

    return {
        "index": idx,
        "name": name,
        "driver": driver,
        "mem_total_mb": int(mem_total_mb) if mem_total_mb is not None else 0,
        "mem_used_mb": int(mem_used_mb) if mem_used_mb is not None else 0,
        "mem_free_mb": int(mem_free_mb) if mem_free_mb is not None else 0,
        "util_pct": util_pct,
        "mem_util_pct": None,  # not available in AMD JSON
        "temp_c": temp_c,
        "power_w": power_w,
        "power_limit_w": power_limit_w,
        "fan_pct": fan_pct,
        "clock_mhz": clock_mhz,
        "mem_clock_mhz": mem_clock_mhz,
        "pcie_gen": pcie_gen,
        "pcie_width": pcie_width,
        "pstate": pstate,
        "temp_mem_c": None,
        "ecc_corrected": 0,
        "ecc_uncorrected": 0,
        "throttle": throttle,
        "apps": [],
    }


def parse_stat(section: str) -> Dict[str, Any]:
    """解析 /proc/stat。返回 {total, idle, cores: {i: (total, idle)}}。"""
    result = {"total": 0, "idle": 0, "cores": {}}
    for line in section.splitlines():
        if not line.startswith("cpu"):
            continue
        fields = line.split()
        name = fields[0]
        nums = fields[1:]
        if len(nums) < 4:
            continue
        total = sum(_i(x, 0) for x in nums)
        idle = _i(nums[3], 0) + (_i(nums[4], 0) if len(nums) > 4 else 0)
        if name == "cpu":
            result["total"] = total
            result["idle"] = idle
        elif name.startswith("cpu"):
            try:
                idx = int(name[3:])
                result["cores"][idx] = (total, idle)
            except ValueError:
                pass
    return result


def parse_meminfo(section: str) -> Dict[str, Any]:
    kv = {}
    for line in section.splitlines():
        if ":" in line:
            k, v = line.split(":", 1)
            v = v.strip()
            kv[k.strip()] = _i(v.split()[0] if v else 0, 0)
    total = kv.get("MemTotal", 0)
    free = kv.get("MemFree", 0)
    avail = kv.get("MemAvailable", 0)
    buff_cache = kv.get("Buffers", 0) + kv.get("Cached", 0)
    swap_total = kv.get("SwapTotal", 0)
    swap_free = kv.get("SwapFree", 0)
    return {
        "total_mb": total // 1024,
        "used_mb": (total - free - buff_cache) // 1024,
        "free_mb": free // 1024,
        "buff_cache_mb": buff_cache // 1024,
        "available_mb": avail // 1024,
        "swap_total_mb": swap_total // 1024,
        "swap_used_mb": (swap_total - swap_free) // 1024,
    }


def parse_loadavg(section: str) -> List[float]:
    fields = section.split()
    return [_f(x, 0.0) for x in fields[:3]]


def parse_uptime(section: str) -> Optional[float]:
    fields = section.split()
    return _f(fields[0]) if fields else None


# 虚拟接口前缀（docker 网桥/veth 等，避免流量重复计数）
_NET_EXCLUDE_PREFIXES = ("docker", "br-", "veth", "virbr", "kube", "cni", "cali", "fla", "tun", "tap")


def is_real_iface(name: str) -> bool:
    return not any(name.startswith(pfx) for pfx in _NET_EXCLUDE_PREFIXES)


def parse_netdev(section: str) -> List[Dict[str, Any]]:
    ifaces = []
    for line in section.splitlines():
        if ":" not in line:
            continue
        name, rest = line.split(":", 1)
        name = name.strip()
        if name == "lo" or not is_real_iface(name):
            continue
        fields = rest.split()
        if len(fields) < 9:
            continue
        ifaces.append({
            "name": name,
            "rx_bytes": _i(fields[0], 0),
            "tx_bytes": _i(fields[8], 0),
        })
    return ifaces


def parse_diskstats(section: str) -> Dict[str, int]:
    """汇总所有块设备的扇区读/写（近似总速率）。"""
    sectors_read = 0
    sectors_written = 0
    for line in section.splitlines():
        fields = line.split()
        if len(fields) < 10:
            continue
        sectors_read += _i(fields[5], 0)
        sectors_written += _i(fields[9], 0)
    return {"sectors_read": sectors_read, "sectors_written": sectors_written}


def parse_df(section: str) -> List[Dict[str, Any]]:
    mounts = []
    seen = set()
    lines = [l for l in section.splitlines() if l.strip()]
    for line in lines[1:]:  # 跳过表头
        fields = line.split()
        if len(fields) < 6:
            continue
        source = fields[0]
        if source in seen:
            continue
        seen.add(source)
        mounts.append({
            "mount": fields[1],
            "size_gb": _i(fields[2], 0) / (1024 ** 3),
            "used_gb": _i(fields[3], 0) / (1024 ** 3),
            "avail_gb": _i(fields[4], 0) / (1024 ** 3),
            "use_pct": _f(fields[5].rstrip("%"), 0.0),
        })
    return mounts



def parse_proc(section: str, diff: DiffEngine, ts: float, host_id: str) -> Dict[str, Any]:
    """解析 ==PROC== 段，返回所有 llama-server 进程列表。

    新版格式：
      PROC_PID:12345
      PROC_STATS:utime stime vsize
      PROC_RSS:rss_kb
      PROC_VSZ:vsz_kb
      PROC_THREADS:n
      PROC_PS:pcpu pmem etime
      PROC_CMDLINE:full cmdline
      PROC_PID:67890
      ...
    """
    procs = []
    lines = [l for l in section.splitlines() if l.strip()]
    if not lines or not lines[0].startswith("PROC_PID:"):
        # 旧格式兼容：单进程 P:PID
        if not lines or not lines[0].startswith("P:"):
            return {"found": False, "list": []}
        pid = _i(lines[0][2:])
        proc = _parse_single_proc(pid, lines[1:], diff, ts, host_id)
        return {"found": True, "list": [proc]}

    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith("PROC_PID:"):
            pid = _i(line[9:])
            proc_lines = []
            i += 1
            # 收集下一条 PROC_PID: 或 PROC_END: 之前的行
            while i < len(lines):
                if lines[i].startswith("PROC_PID:") or (lines[i].startswith("PROC_END:") and not lines[i].startswith("PROC_PID:")):
                    break
                proc_lines.append(lines[i])
                i += 1
            proc = _parse_single_proc(pid, proc_lines, diff, ts, host_id)
            procs.append(proc)
        else:
            i += 1
    return {"found": len(procs) > 0, "list": procs}


def _parse_single_proc(pid: int, proc_lines: List[str], diff: DiffEngine, ts: float, host_id: str) -> Dict[str, Any]:
    """解析单个进程的 PROC_* 行。"""
    proc = {"found": True, "pid": pid}
    idx = 0
    while idx < len(proc_lines):
        line = proc_lines[idx]
        if line.startswith("PROC_STATS:"):
            f = line[11:].split()
            if len(f) >= 3:
                utime = _i(f[0], 0)
                stime = _i(f[1], 0)
                vsize = _i(f[2], 0)
                proc["vsz_mb"] = vsize // (1024 * 1024)
                proc["cpu_pct_realtime"] = diff.process_cpu_pct(
                    "proc:%s:%s" % (host_id, pid), ts, utime + stime)
        elif line.startswith("PROC_RSS:"):
            proc["rss_mb"] = _i(line[11:], 0) // 1024
        elif line.startswith("PROC_VSZ:"):
            proc["vsz_mb"] = _i(line[11:], 0) // (1024 * 1024)
        elif line.startswith("PROC_THREADS:"):
            proc["threads"] = _i(line[15:], 0)
        elif line.startswith("PROC_PS:"):
            f = line[10:].split()
            if len(f) >= 3:
                proc["cpu_pct_lifetime"] = _f(f[0])
                proc["mem_pct"] = _f(f[1])
                proc["elapsed"] = f[2]
        elif line.startswith("PROC_CMDLINE:"):
            proc["cmdline"] = line[13:].strip()
        idx += 1
    return proc


def parse_ps_ticks(section: str) -> Dict[int, int]:
    """解析 ==PSTICKS== 段（每行 `pid utime stime`，来自 /proc/<pid>/stat，时钟滴答）。

    相比 ps 的 time 列（1s 粒度），/proc 的 utime/stime 为 10ms 粒度，
    2s 采样窗口下进程 CPU% 不再被量化成 0/50/100/150 的台阶值。
    """
    ticks: Dict[int, int] = {}
    for line in section.splitlines():
        f = line.split()
        if len(f) < 3:
            continue
        try:
            pid = int(f[0])
            utime = int(f[1])
            stime = int(f[2])
        except ValueError:
            continue
        ticks[pid] = utime + stime
    return ticks


def parse_ps(section: str) -> List[Dict[str, Any]]:
    """解析 `ps -eo pid,comm,pcpu,pmem,rss`。

    comm 可能含空格：pid 取首列，pcpu/pmem/rss 取末 3 列，中间为进程名。
    进程 CPU 滴答不在此处（ps time 列仅 1s 粒度），由 ==PSTICKS== 段按 PID 关联。
    """
    procs = []
    for line in section.splitlines():
        f = line.split()
        if len(f) < 5:
            continue
        try:
            pid = int(f[0])
        except ValueError:
            continue
        procs.append({
            "pid": pid,
            "name": " ".join(f[1:-3]),
            "cpu_pct_lifetime": _f(f[-3], 0.0),
            "mem_pct": _f(f[-2], 0.0),
            "rss_mb": _i(f[-1], 0) // 1024,
        })
    return procs


def parse_service(section: str) -> Dict[str, Any]:
    kv = {}
    for line in section.splitlines():
        if "=" in line:
            k, v = line.split("=", 1)
            kv[k.strip()] = v.strip()
    svc = {}
    svc["description"] = kv.get("Description", "")
    active = kv.get("ActiveState", "")
    sub = kv.get("SubState", "")
    svc["active"] = "%s (%s)" % (active, sub) if active else ""
    svc["since"] = kv.get("ExecMainStartTimestamp", "")
    cpu_ns = _i(kv.get("CPUUsageNSec"), 0)
    svc["cpu_total"] = _fmt_seconds(cpu_ns / 1e9) if cpu_ns else ""
    mem_cur = _i(kv.get("MemoryCurrent"), 0)
    mem_peak = _i(kv.get("MemoryPeak"), 0)
    svc["memory"] = _fmt_bytes(mem_cur) if mem_cur else ""
    svc["memory_peak"] = _fmt_bytes(mem_peak) if mem_peak else ""
    svc["tasks"] = _i(kv.get("NTasks"), 0)
    return svc


def parse_models(section: str) -> Dict[str, Any]:
    """返回 {path: size_bytes} 和 per-pid model/mmproj 信息。

    新版格式：
      MODEL:pid:model_path
      MMPROJ:pid:mmproj_path
    旧格式（ls -l 输出）：
      -rw-r--r-- 1 user group 12345678 /path/to/model
    """
    result = {"path_sizes": {}, "models": {}, "mmproj": {}}
    for line in section.splitlines():
        line = line.strip()
        if not line:
            continue
        # 新版多进程格式
        if line.startswith("MODEL:"):
            parts = line.split(":", 2)
            if len(parts) == 3:
                pid = int(parts[1])
                result["models"][pid] = parts[2]
        elif line.startswith("MMPROJ:"):
            parts = line.split(":", 2)
            if len(parts) == 3:
                pid = int(parts[1])
                result["mmproj"][pid] = parts[2]
        # 旧格式 ls -l 输出
        elif line.startswith("-"):
            f = line.split()
            if len(f) >= 5:
                result["path_sizes"][f[-1]] = _i(f[4], 0)
    return result


def _fmt_seconds(total_s: float) -> str:
    m = int(total_s // 60)
    s = total_s % 60
    return "%dmin %.3fs" % (m, s)


def _fmt_bytes(n: int) -> str:
    if n >= 1024 ** 3:
        return "%.1fG" % (n / 1024 ** 3)
    if n >= 1024 ** 2:
        return "%.1fM" % (n / 1024 ** 2)
    return "%dB" % n



# ---------------------------------------------------------------------------
# SshPoller
# ---------------------------------------------------------------------------

class SshPoller:
    """每主机一个 SSH 采集器：静态信息一次 + 周期批量采集。"""

    def __init__(self, host_cfg: HostConfig, ssh: SshConnection, diff: DiffEngine,
                 ring: RingBuffer, events: EventDetector):
        self.cfg = host_cfg
        self.ssh = ssh
        self.diff = diff
        self.ring = ring
        self.events = events
        self.host_id = host_cfg.id
        # 共享输出（HostMonitor.snapshot 读取）
        self.metrics: Dict[str, Any] = {
            "reachable": False,
            "sys": {}, "cpu": {}, "mem": {}, "disk": {}, "net": {},
            "gpus": [], "process": {"found": False}, "service": {},
            "top": {"cpu": [], "mem": []},
        }
        self._static_done = False
        self._cuda_ver: Optional[str] = None
        self._last_ts: Optional[float] = None
        self._last_gpu_warn = 0.0
        self._amd_static: Dict[int, Dict[str, Any]] = {}
        self._stopped = False

    def _build_batch_cmd(self) -> str:
        mounts = self.cfg.disk_mounts or ["/"]
        return BATCH_CMD.format(
            process_name=self.cfg.process_name,
            systemd_unit=self.cfg.systemd_unit,
            df_mounts=" ".join(mounts),
        )

    async def _collect_static(self) -> None:
        out = await self.ssh.exec_command(STATIC_CMD)
        if out is None:
            return
        sec = split_sections(out)
        sysinfo = self.metrics["sys"]
        sysinfo["hostname"] = sec.get("HOSTNAME", "").strip()
        sysinfo["kernel"] = sec.get("KERNEL", "").strip()
        sysinfo["os"] = sec.get("OS", "").strip()
        cpu = self.metrics["cpu"]
        cpu["model"] = sec.get("CPUMODEL", "").strip()
        cpu["cores"] = _i(sec.get("CORES", "").strip(), 0)
        cpu["mhz"] = _f(sec.get("MHZ", "").strip())
        self._cuda_ver = parse_smi_cuda(sec.get("CUDA", ""))
        nvsmi = sec.get("NVSMI", "").strip() or "?"
        if nvsmi == "NOT_FOUND" or not nvsmi:
            log.warning("[%s] nvidia-smi 不存在或不在 PATH: %r — GPU 数据不可用", self.host_id, nvsmi)
        else:
            log.info("[%s] nvidia-smi: %s（CUDA %s）", self.host_id, nvsmi, self._cuda_ver or "не определён")
        amdsmi = sec.get("AMDSMI", "").strip() or "?"
        if amdsmi == "NOT_FOUND" or not amdsmi:
            log.info("[%s] amd-smi/rocm-smi 不存在 — AMD GPU 数据不可用", self.host_id)
        else:
            log.info("[%s] amd-smi/rocm-smi: %s", self.host_id, amdsmi)
        # Parse AMD static data (gpu names, driver, VRAM size)
        self._amd_static = {}
        amdst = sec.get("AMDSTATIC", "")
        if amdst.strip():
            try:
                root = json.loads(amdst)
                if isinstance(root, dict):
                    for k in ("gpu_data", "gpu_static_data", "gpu", "gpus"):
                        val = root.get(k)
                        if isinstance(val, list):
                            for e in val:
                                if isinstance(e, dict):
                                    self._amd_static[_safe_int(_pick(e, "gpu", "gpu_id", "index"), -1)] = e
                            break
            except Exception:
                self._amd_static = {}
        self._static_done = True
        log.info("[%s] 静态信息: %s", self.host_id, sysinfo)

    async def start(self) -> None:
        log.info("[%s] SshPoller 启动（间隔 %.1fs）", self.host_id, self.cfg.ssh.interval)
        while not self._stopped:
            try:
                if not self._static_done:
                    await self._collect_static()
                await self._cycle()
            except Exception:
                # 单周期异常（命令构造/解析错误）不能杀死整个 poller 任务，
                # 否则主机将永久停在"SSH 断开"且无任何日志痕迹
                log.exception("[%s] 采集周期异常", self.host_id)
                self.metrics["reachable"] = False
            await asyncio.sleep(self.cfg.ssh.interval)

    def stop(self) -> None:
        self._stopped = True

    async def _cycle(self) -> None:
        ts = time.time()
        out = await self.ssh.exec_command(self._build_batch_cmd())
        if out is None:
            self.metrics["reachable"] = False
            return
        self.metrics["reachable"] = True
        sec = split_sections(out)
        self._parse_cycle(sec, ts)

    def _parse_cycle(self, sec: Dict[str, str], ts: float) -> None:
        m = self.metrics
        # GPU + APPS
        gpus = parse_gpu(sec.get("GPU", "")) or parse_amd_gpu(sec.get("AMDGPU", ""), self._amd_static)
        if not gpus:
            now = time.time()
            if now - self._last_gpu_warn > 300:
                log.warning("[%s] GPU 数据为空（无 nvidia-smi / amd-smi 输出，stderr 细节见 DEBUG）", self.host_id)
                self._last_gpu_warn = now
        else:
            self._last_gpu_warn = 0.0
        apps = parse_apps(sec.get("APPS", ""))
        cuda_ver = self._cuda_ver
        for g in gpus:
            g["apps"] = apps
            g["cuda"] = cuda_ver
        m["gpus"] = gpus

        # CPU（整机 + 每核差分）
        stat = parse_stat(sec.get("STAT", ""))
        cpu = m["cpu"]
        cpu["usage_pct"] = self.diff.cpu_pct("cpu:%s" % self.host_id, ts, stat["total"], stat["idle"])
        per_core = []
        for i in sorted(stat["cores"].keys()):
            total, idle = stat["cores"][i]
            per_core.append(self.diff.cpu_pct("cpu:%s:%d" % (self.host_id, i), ts, total, idle))
        cpu["per_core_pct"] = per_core
        cpu["load"] = parse_loadavg(sec.get("LOAD", ""))

        # 内存
        m["mem"] = parse_meminfo(sec.get("MEM", ""))

        # 磁盘（df + diskstats 差分）
        diskio = parse_diskstats(sec.get("DISKIO", ""))
        m["disk"] = {
            "mounts": parse_df(sec.get("DF", "")),
            "read_mb_s": (self.diff.bytes_rate("diskr:%s" % self.host_id, ts, diskio["sectors_read"]) or 0) * 512 / 1024 / 1024,
            "write_mb_s": (self.diff.bytes_rate("diskw:%s" % self.host_id, ts, diskio["sectors_written"]) or 0) * 512 / 1024 / 1024,
        }

        # 网络（差分）
        ifaces = parse_netdev(sec.get("NET", ""))
        net_out = []
        for itf in ifaces:
            rx_rate = self.diff.bytes_rate("netrx:%s:%s" % (self.host_id, itf["name"]), ts, itf["rx_bytes"])
            tx_rate = self.diff.bytes_rate("nettx:%s:%s" % (self.host_id, itf["name"]), ts, itf["tx_bytes"])
            net_out.append({
                "name": itf["name"],
                "rx_mb_s": (rx_rate or 0) / 1024 / 1024,
                "tx_mb_s": (tx_rate or 0) / 1024 / 1024,
                "rx_total_mb": itf["rx_bytes"] / 1024 / 1024,
                "tx_total_mb": itf["tx_bytes"] / 1024 / 1024,
            })
        m["net"] = {"ifaces": net_out}

        # 进程（多进程支持）
        m["process"] = parse_proc(sec.get("PROC", ""), self.diff, ts, self.host_id)

        # Top 进程（实时 CPU% = Δ(utime+stime)/Δt；Top CPU 按实时排序，Top 内存按 RSS 排序）
        procs = parse_ps(sec.get("PS", ""))
        ps_ticks = parse_ps_ticks(sec.get("PSTICKS", ""))
        ps_rows = []
        for p in procs:
            cpu_ticks = ps_ticks.get(p["pid"], 0)
            rt = self.diff.process_cpu_pct(
                "ps:%s:%d" % (self.host_id, p["pid"]), ts, cpu_ticks)
            ps_rows.append({
                "pid": p["pid"],
                "name": p["name"],
                "cpu_pct": rt if rt is not None else 0.0,
                "mem_pct": p["mem_pct"],
                "rss_mb": p["rss_mb"],
                "_ticks": cpu_ticks,
            })
        self.diff.prune("ps:%s:" % self.host_id, {p["pid"] for p in procs})
        # Top CPU：实时 CPU% 并列时（如系统空闲全 0.0）按累计 CPU 时间兜底，
        # 避免列表退化为 Top 内存的副本（RSS 序）看起来"数据是假的"
        top_cpu = sorted(ps_rows, key=lambda r: (-r["cpu_pct"], -r["_ticks"], -r["rss_mb"]))[:8]
        top_mem = sorted(ps_rows, key=lambda r: (-r["rss_mb"], -r["cpu_pct"]))[:8]
        for r in top_cpu + top_mem:
            r.pop("_ticks", None)
        m["top"] = {"cpu": top_cpu, "mem": top_mem}

        # 系统（uptime + procs，合并静态信息）
        m["sys"]["uptime_s"] = parse_uptime(sec.get("UPTIME", ""))
        m["sys"]["procs"] = _i(sec.get("PROCS", "").strip(), 0)

        # 服务
        m["service"] = parse_service(sec.get("SERVICE", ""))
        m["service"]["unit"] = self.cfg.systemd_unit

        # 模型文件体积 + per-pid model/mmproj
        model_info = parse_models(sec.get("MODELS", ""))
        m["_model_sizes"] = model_info.get("path_sizes", {})
        m["_model_paths"] = model_info.get("models", {})
        m["_mmproj_paths"] = model_info.get("mmproj", {})

        self._push_ring(ts)

    def _push_ring(self, ts: float) -> None:
        m = self.metrics
        cpu = m.get("cpu", {})
        mem = m.get("mem", {})
        disk = m.get("disk", {})
        net = m.get("net", {})
        proc = m.get("process", {})
        self.ring.push("cpu", ts, cpu.get("usage_pct"))
        load = cpu.get("load") or []
        for i, name in enumerate(("load_1", "load_5", "load_15")):
            self.ring.push(name, ts, load[i] if i < len(load) else None)
        self.ring.push("mem_used", ts, mem.get("used_mb"))
        self.ring.push("mem_buff_cache", ts, mem.get("buff_cache_mb"))
        self.ring.push("swap_used", ts, mem.get("swap_used_mb"))
        self.ring.push("disk_read", ts, disk.get("read_mb_s"))
        self.ring.push("disk_write", ts, disk.get("write_mb_s"))
        ifaces = net.get("ifaces", [])
        self.ring.push("net_rx", ts, sum(i.get("rx_mb_s", 0) for i in ifaces))
        self.ring.push("net_tx", ts, sum(i.get("tx_mb_s", 0) for i in ifaces))
        self.ring.push("proc_cpu", ts, proc.get("cpu_pct_realtime"))
        for g in m.get("gpus", []):
            idx = g.get("index", 0)
            self.ring.push("gpu_util_%d" % idx, ts, g.get("util_pct"))
            self.ring.push("gpu_mem_%d" % idx, ts, g.get("mem_used_mb"))
            self.ring.push("gpu_temp_%d" % idx, ts, g.get("temp_c"))
            self.ring.push("gpu_power_%d" % idx, ts, g.get("power_w"))
        self._last_ts = ts
