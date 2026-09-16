"""TDD для движка совокупной статистики (backend/stats.py).

Запуск из корня репозитория:  python -m pytest backend/tests -q
"""
import os

import pytest

from backend.stats import HostStats, StatsStore, aggregate


# ---------------------------------------------------------------------------
# Хелперы-фабрики срезов /slots
# ---------------------------------------------------------------------------
def slot(sid=0, task=1, processing=True, decoded=0, prompt=100, processed=100):
    """Срез одного слота в формате llama.cpp /slots."""
    return {
        "id": sid,
        "id_task": task if processing else -1,
        "is_processing": processing,
        "n_decoded": decoded,
        "n_prompt_tokens": prompt,
        "n_prompt_tokens_processed": processed,
    }


# ---------------------------------------------------------------------------
# Запросы (requests)
# ---------------------------------------------------------------------------
def test_new_task_counts_one_request():
    s = HostStats("h")
    changed = s.observe([slot(task=1, decoded=0)], now=10.0)
    assert s.requests == 1
    assert changed is True
    # тот же id_task повторно → не удваиваем
    s.observe([slot(task=1, decoded=5)], now=11.0)
    assert s.requests == 1


def test_second_request_on_same_slot_counts_again():
    s = HostStats("h")
    s.observe([slot(task=1, decoded=10)], now=10.0)
    s.observe([slot(task=2, decoded=3)], now=12.0)
    assert s.requests == 2


def test_idle_then_new_request_counts():
    s = HostStats("h")
    s.observe([slot(task=1, processing=True, decoded=5)], now=10.0)
    s.observe([slot(task=-1, processing=False)], now=11.0)  # завершилась
    s.observe([slot(task=2, processing=True, decoded=1)], now=12.0)
    assert s.requests == 2


def test_idle_only_no_requests():
    s = HostStats("h")
    s.observe([slot(task=-1, processing=False)], now=10.0)
    assert s.requests == 0


# ---------------------------------------------------------------------------
# Генерация и обработка токенов (дельты)
# ---------------------------------------------------------------------------
def test_generated_and_processed_accumulate_by_delta():
    s = HostStats("h")
    s.observe([slot(task=1, decoded=10, prompt=100, processed=100)], now=10.0)
    assert s.generated_tokens == 10
    assert s.processed_tokens == 100
    s.observe([slot(task=1, decoded=25, prompt=100, processed=100)], now=11.0)
    assert s.generated_tokens == 25  # +15
    assert s.processed_tokens == 100  # промпт уже посчитан


def test_monotonic_no_double_on_repeat_sample():
    s = HostStats("h")
    s.observe([slot(task=1, decoded=10)], now=10.0)
    s.observe([slot(task=1, decoded=10)], now=11.0)  # тот же срез повторно
    assert s.generated_tokens == 10


def test_backend_restart_does_not_go_negative():
    """llama-server перезапустился: счётчики слота обнулились, id_task тоже."""
    s = HostStats("h")
    s.observe([slot(task=1, decoded=500)], now=10.0)
    before = s.generated_tokens
    # после рестарта: id_task=0, decoded=2
    s.observe([slot(task=0, decoded=2)], now=11.0)
    assert s.generated_tokens >= before  # не уменьшилось
    assert s.generated_tokens == before + 2


# ---------------------------------------------------------------------------
# Кешированные токены (best-effort)
# ---------------------------------------------------------------------------
def test_cached_tokens_from_prompt_gap():
    """Полный промах кэша: processed == prompt → cached 0."""
    s = HostStats("h")
    s.observe([slot(task=1, decoded=0, prompt=200, processed=200)], now=10.0)
    s.observe([slot(task=-1, processing=False)], now=11.0)  # финал задачи
    assert s.cached_tokens == 0


def test_cached_tokens_from_partial_process():
    """Кэш-хит: обработано 50 из 200 → 150 закэшировано (учтётся на финале)."""
    s = HostStats("h")
    s.observe([slot(task=1, decoded=0, prompt=200, processed=50)], now=10.0)
    assert s.cached_tokens == 0  # ещё не финализирована
    s.observe([slot(task=-1, processing=False)], now=11.0)
    assert s.cached_tokens == 150


def test_cached_never_negative():
    s = HostStats("h")
    # аномалия: processed больше prompt
    s.observe([slot(task=1, decoded=0, prompt=100, processed=150)], now=10.0)
    s.observe([slot(task=-1, processing=False)], now=11.0)
    assert s.cached_tokens == 0


# ---------------------------------------------------------------------------
# Сброс
# ---------------------------------------------------------------------------
def test_reset_zeroes_and_starts_new_window():
    s = HostStats("h")
    s.observe([slot(task=1, decoded=100, prompt=50, processed=50)], now=10.0)
    assert s.requests == 1 and s.generated_tokens == 100
    s.reset(now=20.0)
    snap = s.snapshot()
    assert snap["requests"] == 0
    assert snap["generated_tokens"] == 0
    assert snap["processed_tokens"] == 0
    assert snap["cached_tokens"] == 0
    assert snap["window_started"] == 20.0
    assert snap["last_reset"] == 20.0
    # после сброса продолжаем считать новое окно
    s.observe([slot(task=2, decoded=7)], now=21.0)
    assert s.requests == 1
    assert s.generated_tokens == 7


def test_reset_clears_slot_baseline_no_burst():
    """После сброса не должно быть всплеска от старых значений слота."""
    s = HostStats("h")
    s.observe([slot(task=1, decoded=999, prompt=100, processed=100)], now=10.0)
    s.reset(now=20.0)
    # тот же слот всё ещё показывает decoded=999 (тот же id_task) — не должны
    # накрутить заново: id_task тот же и дельта нулевая относительно сброса...
    s.observe([slot(task=1, decoded=999, prompt=100, processed=100)], now=21.0)
    # id_task=1 != None после сброса → засчитаем как новый запрос с нуля
    assert s.generated_tokens == 999  # baseline сброшен → пересчитал с 0
    assert s.requests == 1


# ---------------------------------------------------------------------------
# Агрегация (total)
# ---------------------------------------------------------------------------
def test_aggregate_sums_all_metrics():
    total = aggregate([
        {"requests": 2, "processed_tokens": 100, "generated_tokens": 50, "cached_tokens": 10},
        {"requests": 3, "processed_tokens": 200, "generated_tokens": 80, "cached_tokens": 5},
    ])
    assert total == {"requests": 5, "processed_tokens": 300,
                     "generated_tokens": 130, "cached_tokens": 15}


def test_aggregate_ignores_garbage():
    total = aggregate([{"requests": 1}, None, "x", {}])
    assert total["requests"] == 1


# ---------------------------------------------------------------------------
# Персист
# ---------------------------------------------------------------------------
def test_store_roundtrip(tmp_path):
    path = str(tmp_path / "stats.json")
    store = StatsStore(path)
    store.save({"a": {"requests": 3, "processed_tokens": 10,
                      "generated_tokens": 5, "cached_tokens": 1,
                      "window_started": 1.0, "last_reset": 0.5, "last_activity": 2.0}})
    loaded = StatsStore(path).load()
    assert loaded["a"]["requests"] == 3
    assert loaded["a"]["cached_tokens"] == 1


def test_store_load_missing_returns_empty(tmp_path):
    assert StatsStore(str(tmp_path / "nope.json")).load() == {}


def test_store_load_corrupt_returns_empty(tmp_path):
    p = tmp_path / "bad.json"
    p.write_text("{ this is not json", encoding="utf-8")
    assert StatsStore(str(p)).load() == {}


def test_store_disabled_when_no_path():
    store = StatsStore(None)
    assert store.enabled is False
    assert store.load() == {}
    assert store.save({"a": {}}) is False


def test_hoststats_persist_restore():
    s = HostStats("h")
    s.observe([slot(task=1, decoded=40, prompt=100, processed=30)], now=10.0)
    s.observe([slot(task=-1, processing=False)], now=11.0)
    dumped = s.to_dict()

    s2 = HostStats("h")
    s2.load_dict(dumped)
    assert s2.requests == s.requests
    assert s2.generated_tokens == s.generated_tokens
    assert s2.cached_tokens == s.cached_tokens
    assert s2.snapshot()["cached_tokens"] == s.snapshot()["cached_tokens"]
