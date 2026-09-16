"""llama-server HTTP 采集器（LlamaPoller）。

- /health + /slots 每 interval（默认 1s）轮询；/props + /v1/models 每 slow_interval（默认 30s）。
- 每 slot 速度差分（n_decoded / n_prompt_tokens_processed），任务开始/结束事件。
- 连续 3 次请求失败 → online=False + llama_down 事件；恢复 → llama_up。
- 模型路径变化 → model_change 事件。
- 兼容 Python 3.9。
"""
import asyncio
import logging
import time
from typing import Any, Dict, Optional

import httpx

from ..config import HostConfig
from ..events import EventDetector

log = logging.getLogger("llamalens.llama")

FAIL_OFFLINE = 3  # 连续失败次数 → 判定离线


def _base_url(host: str, port: int) -> str:
    h = host
    if ":" in h and not h.startswith("["):  # IPv6 字面量需要方括号
        h = "[%s]" % h
    return "http://%s:%d" % (h, port)


def _slot_decoded(slot: dict) -> Optional[int]:
    """slot 已解码 token 数。

    上游 llama.cpp：顶层 n_decoded 字段。
    部分自定义 build：无顶层 n_decoded，计数在 next_token[0].n_decoded；
    且该 build 的 n_prompt_tokens 已包含 decoded（即上下文已用总量）。
    """
    n = slot.get("n_decoded")
    if n is not None:
        try:
            return int(n)
        except (TypeError, ValueError):
            return None
    nt = slot.get("next_token")
    if isinstance(nt, list) and nt and isinstance(nt[0], dict):
        n = nt[0].get("n_decoded")
        if n is not None:
            try:
                return int(n)
            except (TypeError, ValueError):
                return None
    return None


class _SlotState:
    __slots__ = ("prev_task_id", "prev_decoded", "prev_prompt_processed",
                 "prev_ts", "task_start_ts", "gen_speed", "prompt_speed")

    def __init__(self):
        self.prev_task_id: Optional[int] = None
        self.prev_decoded = 0
        self.prev_prompt_processed = 0
        self.prev_ts: Optional[float] = None
        self.task_start_ts: Optional[float] = None
        self.gen_speed = 0.0
        self.prompt_speed = 0.0


class LlamaPoller:
    """每主机一个 HTTP 采集器。state 供 HostMonitor.snapshot 读取。"""

    def __init__(self, cfg: HostConfig, events: EventDetector, stats=None):
        self.cfg = cfg
        self.events = events
        self.stats = stats  # Optional[HostStats]: совокупная статистика (дельты /slots)
        # URL prefix: path "v1/llama" → "/v1/llama", empty → ""
        raw_path = cfg.llama.path
        if raw_path:
            self._prefix = "/" + raw_path
        else:
            self._prefix = ""
        self.state: Dict[str, Any] = {
            "online": False,
            "model": {},
            "slots": [],
            "ctx": {"used": None, "total": 0},
            "gen_speed_tps": 0.0,
            "prompt_speed_tps": 0.0,
        }
        self._slot_states: Dict[int, _SlotState] = {}
        self._fail_count = 0
        self._client: Optional[httpx.AsyncClient] = None
        self._stopped = False
        self._model_path: Optional[str] = None

    # ------------------------------------------------------------------
    async def start(self) -> None:
        host = self.cfg.llama.host
        port = self.cfg.llama.port
        log.info("[%s] LlamaPoller 启动 %s:%s%s（%.1fs / 慢 %.1fs）",
                 self.cfg.id, host, port, self._prefix,
                 self.cfg.llama.interval, self.cfg.llama.slow_interval)
        self._client = httpx.AsyncClient(
            base_url=_base_url(host, port),
            timeout=self.cfg.llama.timeout,
        )
        last_slow = 0.0
        try:
            while not self._stopped:
                try:
                    now = time.time()
                    await self._poll_fast(now)
                    if now - last_slow >= self.cfg.llama.slow_interval:
                        last_slow = now
                        await self._poll_slow(now)
                except Exception:
                    # 单周期异常（如 /slots 返回畸形数据）不能杀死整个 poller 任务，
                    # 否则该主机的 llama 采集将永久停止且无重启机制（同 SshPoller/LogPoller）
                    log.exception("[%s] llama 采集周期异常", self.cfg.id)
                await asyncio.sleep(self.cfg.llama.interval)
        finally:
            await self._client.aclose()

    def stop(self) -> None:
        self._stopped = True

    # ------------------------------------------------------------------
    async def _poll_fast(self, now: float) -> None:
        try:
            # /slots 即可判定在线（模型未加载 503 / 服务宕机连接失败），无需再发 /health
            resp = await self._client.get(self._prefix + "/slots")
            resp.raise_for_status()
            data = resp.json()
        except Exception as e:
            self._on_fail(now, e)
            return

        self._fail_count = 0
        if not self.state["online"]:
            self.state["online"] = True
            self.events.set_llama_online(now, True, (self.state["model"] or {}).get("name", ""))

        # 新版 llama.cpp 返回裸数组 [ {...} ]，旧版返回 {"slots": [...]}
        if isinstance(data, list):
            slots = data
        elif isinstance(data, dict):
            slots = data.get("slots")
            if slots is None and "default" in data:
                slots = [data["default"]]
        else:
            slots = None
        if not isinstance(slots, list):
            slots = []
        self._process_slots(slots, now)

    def _on_fail(self, now: float, err: Exception) -> None:
        self._fail_count += 1
        if self._fail_count == 1 or self._fail_count % 30 == 0:
            log.warning("[%s] llama 请求失败(%d): %s", self.cfg.id, self._fail_count, err)
        if self._fail_count >= FAIL_OFFLINE and self.state["online"]:
            self.state["online"] = False
            self.state["gen_speed_tps"] = 0.0
            self.state["prompt_speed_tps"] = 0.0
            for s in self.state["slots"]:
                s["gen_speed_tps"] = 0.0
                s["prompt_speed_tps"] = 0.0
            self.events.set_llama_online(now, False)

    # ------------------------------------------------------------------
    def _process_slots(self, slots: list, now: float) -> None:
        out = []
        total_gen = 0.0
        total_prompt = 0.0
        agg_used = None
        agg_total = 0
        for slot in slots:
            if not isinstance(slot, dict):
                continue
            sid = slot.get("id", 0)
            st = self._slot_states.get(sid)
            if st is None:
                st = _SlotState()
                self._slot_states[sid] = st
            self._diff_slot(st, slot, now)

            n_ctx = slot.get("n_ctx") or 0
            # 上游 build：n_prompt_tokens 仅 prompt，需加 n_decoded；
            # 自定义 build（无顶层 n_decoded）：n_prompt_tokens 已含 decoded
            if slot.get("n_decoded") is not None:
                ctx_used = (slot.get("n_prompt_tokens") or 0) + (slot.get("n_decoded") or 0)
            else:
                ctx_used = slot.get("n_prompt_tokens") or 0
            params = slot.get("params") or {}
            spec = (params.get("speculative") or {}).get("types") or []
            item = dict(slot)
            item["n_decoded"] = _slot_decoded(slot)
            item["gen_speed_tps"] = round(st.gen_speed, 2)
            item["prompt_speed_tps"] = round(st.prompt_speed, 2)
            item["ctx_used"] = ctx_used
            item["ctx_pct"] = round(ctx_used / n_ctx * 100.0, 2) if n_ctx else None
            item["speculative"] = bool(spec)
            out.append(item)
            if agg_used is None or ctx_used > agg_used:
                agg_used = ctx_used
            if n_ctx > agg_total:
                agg_total = n_ctx
            if slot.get("is_processing"):
                total_gen = max(total_gen, st.gen_speed)
                total_prompt = max(total_prompt, st.prompt_speed)
        self.state["slots"] = out
        self.state["gen_speed_tps"] = round(total_gen, 2)
        self.state["prompt_speed_tps"] = round(total_prompt, 2)
        self.state["ctx"] = {"used": agg_used, "total": agg_total}
        # Совокупная статистика (аналог llama-swap): дельты по всем слотам.
        if self.stats is not None:
            try:
                self.stats.observe(out, now)
            except Exception:
                log.exception("[%s] stats.observe  упала", self.cfg.id)

    def _diff_slot(self, st: _SlotState, slot: dict, now: float) -> None:
        """按架构文档 §4.1 的差分算法更新单 slot 速度与任务事件。"""
        task_id = slot.get("id_task")
        n_decoded = _slot_decoded(slot)
        n_prompt_processed = slot.get("n_prompt_tokens_processed") or 0
        dt = (now - st.prev_ts) if st.prev_ts is not None else 0.0

        if slot.get("is_processing"):
            if st.prev_task_id is None:
                st.task_start_ts = now
                self.events.set_task_running(now, True, task_id, slot.get("n_prompt_tokens"))
            if task_id != st.prev_task_id or (
                    n_decoded is not None and n_decoded < st.prev_decoded):
                # 新任务（或任务边界）：重置基线，本周期速度 = 0
                st.prev_task_id = task_id
                st.prev_decoded = n_decoded if n_decoded is not None else 0
                st.prev_prompt_processed = n_prompt_processed
                st.gen_speed = 0.0
                st.prompt_speed = 0.0
            else:
                if dt > 0:
                    if n_decoded is not None:
                        st.gen_speed = max(0.0, (n_decoded - st.prev_decoded) / dt)
                    st.prompt_speed = max(
                        0.0, (n_prompt_processed - st.prev_prompt_processed) / dt)
                if n_decoded is not None:
                    st.prev_decoded = n_decoded
                st.prev_prompt_processed = n_prompt_processed
        else:
            if st.prev_task_id is not None:
                duration = (now - st.task_start_ts) if st.task_start_ts else None
                avg = (st.prev_decoded / duration) if (duration and duration > 0) else None
                self.events.task_end_with_stats(
                    now, st.prev_task_id, st.prev_decoded, duration, avg)
                st.prev_task_id = None
                st.task_start_ts = None
            st.gen_speed = 0.0
            st.prompt_speed = 0.0
        st.prev_ts = now

    # ------------------------------------------------------------------
    async def _poll_slow(self, now: float) -> None:
        model = dict(self.state["model"] or {})
        try:
            resp = await self._client.get(self._prefix + "/props")
            resp.raise_for_status()
            props = resp.json()
            model.update(self._parse_props(props))
        except Exception as e:
            log.debug("[%s] /props 失败: %s", self.cfg.id, e)
        try:
            resp = await self._client.get(self._prefix + "/v1/models")
            resp.raise_for_status()
            data = (resp.json() or {}).get("data") or []
            if data:
                model.update(self._parse_v1_model(data[0]))
        except Exception as e:
            log.debug("[%s] /v1/models 失败: %s", self.cfg.id, e)

        if model:
            self.state["model"] = model
            path = model.get("path")
            if path and path != self._model_path:
                if self._model_path is not None:
                    self.events.set_model(now, path)
                self._model_path = path

    @staticmethod
    def _parse_props(props: dict) -> Dict[str, Any]:
        model = {}
        path = props.get("model_path") or ""
        model["path"] = path
        model["name"] = props.get("model_alias") or (path.rsplit("/", 1)[-1] if path else "")
        for k in ("ftype", "n_embd", "n_vocab", "n_ctx", "n_ctx_train", "vocab_type", "owned_by"):
            if props.get(k) is not None:
                model[k] = props[k]
        # mmproj — llama.cpp /props 可能包含 mmproj_path
        mmproj = props.get("mmproj_path")
        if mmproj:
            model["mmproj_path"] = mmproj
        modalities = props.get("modalities") or []
        model["modalities"] = {
            "vision": "image" in modalities,
            "video": "video" in modalities,
            "audio": "audio" in modalities,
        }
        caps = props.get("capabilities") or []
        model["capabilities"] = caps
        return model

    @staticmethod
    def _parse_v1_model(m: dict) -> Dict[str, Any]:
        model = {}
        # 定制 build（llama-cpp-turboquant）把元数据放在 data[0].meta 下，
        # 顶层字段为 null；先读顶层，再回退 meta（架构文档 §5.1 数据模型）。
        meta = m.get("meta") or {}
        for k in ("n_params", "n_embd", "n_vocab", "size", "ftype",
                  "vocab_type", "n_ctx", "n_ctx_train"):
            v = m.get(k)
            if v is None:
                v = meta.get(k)
            if v is not None:
                model[k] = v
        if model.get("size") is not None:
            model["file_size"] = model["size"]
        if m.get("owned_by"):
            model.setdefault("owned_by", m["owned_by"])
        return model
