"""Совокупная статистика использования бекенда (аналог llama-swap).

Накапливает четыре счётчика по дельтам из /slots (работает на любой сборке
llama.cpp, включая кастомные, и не требует /metrics):

- ``requests``            — число запросов (задач) к бекенду;
- ``processed_tokens``     — обработанные токены промпта (prefill);
- ``generated_tokens``     — сгенерированные (decoded) токены;
- ``cached_tokens``        — токены промпта, обслуженные из KV-кэша (best-effort).

Счётчики считаются «с последнего сброса»: кнопка в UI обнуляет их и помечает
время окна. Дельты устойчивы к рестарту llama-server — счётчики слота
обнуляются, базовая линия сбрасывается, в минус не уходим. Персист в JSON
переживает перезапуск llama-lens (иное поведение было бы сюрпризом для
пользователя, который сбросил счётчики под задачу).

``cached_tokens`` — best-effort и никогда не приписывает ложное: считаем
``n_prompt_tokens - n_prompt_tokens_processed`` на момент завершения задачи
(сколько токенов промпта НЕ пересчитывались). Если сборка всегда доводит
``processed`` до ``total``, кэш прочтётся нулём.

Ключевой компромисс: подсчёт по опросу /slots с шагом 1s может пропустить
короткий запрос, целиком уместившийся между двумя опросами. Для целевого
сценария (сброс → выполнить работу → считать итоги) это приемлемо.

Совместимо с Python 3.9 (без 3.10+ синтаксиса).
"""
import json
import logging
import os
import tempfile
import time
from typing import Any, Dict, List, Optional

log = logging.getLogger("llamalens.stats")

# Версия формата персиста — на случай будущих миграций.
SCHEMA_VERSION = 1


def _as_int(v: Any) -> int:
    """Безопасно привести значение к int (None/мусор → 0)."""
    try:
        return int(v)
    except (TypeError, ValueError):
        return 0


def _slot_decoded(slot: Dict[str, Any]) -> int:
    """Уже раскодированные токены слота (с фолбэком в next_token[0].n_decoded)."""
    n = slot.get("n_decoded")
    if n is None:
        nt = slot.get("next_token")
        if isinstance(nt, list) and nt and isinstance(nt[0], dict):
            n = nt[0].get("n_decoded")
    return _as_int(n)


class _SlotTracker:
    """Внутреннее состояние одного слота для дельт текущего окна."""

    __slots__ = ("task_id", "prev_decoded", "prev_processed",
                 "task_prompt_total", "task_processed")

    def __init__(self) -> None:
        self.task_id: Optional[int] = None
        self.prev_decoded = 0
        self.prev_processed = 0
        self.task_prompt_total = 0
        self.task_processed = 0


class HostStats:
    """Счётчики одной хост-бэкенда + дельта-трекинг по слотам."""

    def __init__(self, host_id: str) -> None:
        self.host_id = host_id
        self.requests = 0
        self.processed_tokens = 0
        self.generated_tokens = 0
        self.cached_tokens = 0
        # Время начала текущего окна (после сброса или первого наблюдения).
        self.window_started: Optional[float] = None
        self.last_reset: Optional[float] = None
        self.last_activity: Optional[float] = None
        self._slots: Dict[int, _SlotTracker] = {}
        self._dirty = False

    # ------------------------------------------------------------------
    def observe(self, slots: List[Dict[str, Any]], now: Optional[float] = None) -> bool:
        """Скормить один срез /slots. Возвращает True, если счётчики изменились."""
        if now is None:
            now = time.time()
        changed = False
        for slot in slots:
            if not isinstance(slot, dict):
                continue
            if self._observe_slot(slot, now):
                changed = True
        if changed:
            self._dirty = True
        return changed

    def _observe_slot(self, slot: Dict[str, Any], now: float) -> bool:
        sid = _as_int(slot.get("id", 0))
        st = self._slots.get(sid)
        if st is None:
            st = _SlotTracker()
            self._slots[sid] = st

        raw_task = slot.get("id_task")
        tid = raw_task if isinstance(raw_task, int) and raw_task >= 0 else None
        decoded = _slot_decoded(slot)
        prompt_total = _as_int(slot.get("n_prompt_tokens"))
        processed = _as_int(slot.get("n_prompt_tokens_processed"))

        changed = False

        # Новый запрос: id_task сменился (или появился) — фиксируем прошлую
        # задачу (её кэш) и открываем новую базовую линию.
        if tid is not None and st.task_id != tid:
            if self._finalize_task(st, now):
                changed = True
            st.task_id = tid
            st.prev_decoded = 0
            st.prev_processed = 0
            st.task_prompt_total = prompt_total
            st.task_processed = 0
            self.requests += 1
            self.last_activity = now
            changed = True
        elif tid is None and st.task_id is not None:
            # Задача завершилась (слот простаивает).
            if self._finalize_task(st, now):
                changed = True
            st.task_id = None
            st.prev_decoded = 0
            st.prev_processed = 0
            changed = True

        # Накапливаем дельты внутри текущей задачи.
        if tid is not None:
            if decoded > st.prev_decoded:
                self.generated_tokens += decoded - st.prev_decoded
                st.prev_decoded = decoded
                self.last_activity = now
                changed = True
            if processed > st.prev_processed:
                d = processed - st.prev_processed
                self.processed_tokens += d
                st.task_processed += d
                st.prev_processed = processed
                self.last_activity = now
                changed = True
            if prompt_total > st.task_prompt_total:
                st.task_prompt_total = prompt_total
                changed = True

        return changed

    def _finalize_task(self, st: _SlotTracker, now: float) -> bool:
        """Завершить текущую задачу: учесть кэш. Возвращает True если что-то учтено."""
        if st.task_id is None:
            return False
        cached = st.task_prompt_total - st.task_processed
        st.task_id = None  # защита от повторного финала
        if cached > 0:
            self.cached_tokens += cached
            self.last_activity = now
            return True
        return False

    # ------------------------------------------------------------------
    def reset(self, now: Optional[float] = None) -> None:
        """Обнулить счётчики и начать новое окно."""
        if now is None:
            now = time.time()
        self.requests = 0
        self.processed_tokens = 0
        self.generated_tokens = 0
        self.cached_tokens = 0
        self.window_started = now
        self.last_reset = now
        self.last_activity = None
        # Сбрасываем дельта-трекинг, чтобы следующий срез не дал всплеска.
        for st in self._slots.values():
            st.task_id = None
            st.prev_decoded = 0
            st.prev_processed = 0
            st.task_prompt_total = 0
            st.task_processed = 0
        self._dirty = True

    def snapshot(self) -> Dict[str, Any]:
        """Снимок для API/WS/фронтенда."""
        return {
            "requests": self.requests,
            "processed_tokens": self.processed_tokens,
            "generated_tokens": self.generated_tokens,
            "cached_tokens": self.cached_tokens,
            "window_started": self.window_started,
            "last_reset": self.last_reset,
            "last_activity": self.last_activity,
            "source": "slots_delta",
        }

    # ------------------------------------------------------------------
    # Персист
    # ------------------------------------------------------------------
    def to_dict(self) -> Dict[str, Any]:
        return {
            "requests": self.requests,
            "processed_tokens": self.processed_tokens,
            "generated_tokens": self.generated_tokens,
            "cached_tokens": self.cached_tokens,
            "window_started": self.window_started,
            "last_reset": self.last_reset,
            "last_activity": self.last_activity,
        }

    def load_dict(self, data: Dict[str, Any]) -> None:
        """Восстановить счётчики из персиста (дельта-трекинг начинается заново)."""
        self.requests = _as_int(data.get("requests"))
        self.processed_tokens = _as_int(data.get("processed_tokens"))
        self.generated_tokens = _as_int(data.get("generated_tokens"))
        self.cached_tokens = _as_int(data.get("cached_tokens"))
        ws = data.get("window_started")
        self.window_started = float(ws) if ws is not None else None
        lr = data.get("last_reset")
        self.last_reset = float(lr) if lr is not None else None
        la = data.get("last_activity")
        self.last_activity = float(la) if la is not None else None

    def pop_dirty(self) -> bool:
        d = self._dirty
        self._dirty = False
        return d


def aggregate(stats_list: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Суммарная статистика по нескольким снимкам HostStats.snapshot()."""
    total = {
        "requests": 0,
        "processed_tokens": 0,
        "generated_tokens": 0,
        "cached_tokens": 0,
    }
    for s in stats_list:
        if not isinstance(s, dict):
            continue
        for k in total:
            total[k] += _as_int(s.get(k))
    return total


class StatsStore:
    """Атомарный JSON-персист счётчиков {host_id: {...}}.

    Пустой путь → персист выключен (только память, удобно для тестов).
    Повреждённый файл → старт с нуля, файл будет перезаписан.
    """

    def __init__(self, path: Optional[str]) -> None:
        self.path = path

    @property
    def enabled(self) -> bool:
        return bool(self.path)

    def load(self) -> Dict[str, Dict[str, Any]]:
        if not self.path or not os.path.exists(self.path):
            return {}
        try:
            with open(self.path, "r", encoding="utf-8") as f:
                raw = json.load(f)
        except (ValueError, OSError) as e:
            log.warning("stats: не удалось прочитать %s (%s) — старт с нуля", self.path, e)
            return {}
        if not isinstance(raw, dict):
            return {}
        hosts = raw.get("hosts")
        if not isinstance(hosts, dict):
            return {}
        out: Dict[str, Dict[str, Any]] = {}
        for hid, data in hosts.items():
            if isinstance(data, dict):
                out[str(hid)] = data
        return out

    def save(self, data: Dict[str, Dict[str, Any]]) -> bool:
        """Атомарная запись (temp + rename). Возвращает True при успехе."""
        if not self.path:
            return False
        payload = {"version": SCHEMA_VERSION, "saved_at": time.time(), "hosts": data}
        try:
            d = os.path.dirname(self.path)
            if d:
                os.makedirs(d, exist_ok=True)
            fd, tmp = tempfile.mkstemp(dir=d or ".", prefix=".stats-", suffix=".tmp")
            try:
                with os.fdopen(fd, "w", encoding="utf-8") as f:
                    json.dump(payload, f, ensure_ascii=False)
                os.replace(tmp, self.path)
            except Exception:
                try:
                    os.unlink(tmp)
                except OSError:
                    pass
                raise
            return True
        except (OSError, ValueError) as e:
            log.warning("stats: не удалось сохранить %s (%s)", self.path, e)
            return False
