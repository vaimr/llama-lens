"""TDD: интеграция совокупной статистики в MonitorRegistry и REST-слой.

Запуск:  python -m pytest backend/tests -q
"""
import json

from fastapi import FastAPI
from fastapi.testclient import TestClient

from backend.api import router as api_router
from backend.config import (AppConfig, GlobalConfig, HostConfig, LlamaCfg,
                            SshCfg, LogCfg)
from backend.monitor import MonitorRegistry
from backend.stats import aggregate


def _host_cfg(hid):
    return HostConfig(
        id=hid, name=hid,
        llama=LlamaCfg(host="127.0.0.1", port=8080),
        ssh=SshCfg(host="127.0.0.1"),
        log=LogCfg(),
    )


def _registry(tmp_path, ids=("a", "b")):
    cfg = AppConfig(global_cfg=GlobalConfig(), hosts=[_host_cfg(i) for i in ids],
                    data_dir=str(tmp_path))
    return MonitorRegistry(cfg)


def _feed(m, task, decoded, prompt, processed):
    m.stats.observe([{
        "id": 0, "id_task": task, "is_processing": True,
        "n_decoded": decoded, "n_prompt_tokens": prompt,
        "n_prompt_tokens_processed": processed,
    }])


# ---------------------------------------------------------------------------
# Registry-уровень
# ---------------------------------------------------------------------------
def test_registry_snapshot_has_per_host_and_total(tmp_path):
    reg = _registry(tmp_path)
    _feed(reg.monitors["a"], task=1, decoded=30, prompt=100, processed=100)
    _feed(reg.monitors["b"], task=1, decoded=10, prompt=50, processed=50)
    snap = reg.stats_snapshot()
    assert snap["hosts"]["a"]["generated_tokens"] == 30
    assert snap["hosts"]["b"]["generated_tokens"] == 10
    assert snap["total"] == aggregate([snap["hosts"]["a"], snap["hosts"]["b"]])
    assert snap["total"]["generated_tokens"] == 40
    assert snap["total"]["requests"] == 2


def test_registry_reset_host_only_affects_one(tmp_path):
    reg = _registry(tmp_path)
    _feed(reg.monitors["a"], 1, 30, 100, 100)
    _feed(reg.monitors["b"], 1, 10, 50, 50)
    snap = reg.reset_host("a")
    assert snap["requests"] == 0 and snap["generated_tokens"] == 0
    assert reg.stats_snapshot()["hosts"]["b"]["generated_tokens"] == 10


def test_registry_reset_all_zeroes_everything(tmp_path):
    reg = _registry(tmp_path)
    _feed(reg.monitors["a"], 1, 30, 100, 100)
    _feed(reg.monitors["b"], 1, 10, 50, 50)
    out = reg.reset_all()
    assert out["total"] == {"requests": 0, "processed_tokens": 0,
                            "generated_tokens": 0, "cached_tokens": 0}


def test_registry_persists_and_reloads(tmp_path):
    reg = _registry(tmp_path)
    _feed(reg.monitors["a"], 1, 30, 100, 100)
    reg._save_stats_now()
    # перечитываем с диска новым реестром
    reg2 = _registry(tmp_path)
    assert reg2.monitors["a"].stats.generated_tokens == 30


def test_registry_reset_persists_immediately(tmp_path):
    reg = _registry(tmp_path)
    _feed(reg.monitors["a"], 1, 30, 100, 100)
    reg.reset_host("a")
    # на диске должен лежать ноль
    reg2 = _registry(tmp_path)
    assert reg2.monitors["a"].stats.generated_tokens == 0


def test_registry_without_hosts_no_crash(tmp_path):
    reg = _registry(tmp_path, ids=())
    assert reg.stats_snapshot()["total"]["requests"] == 0
    assert reg.reset_all()["total"]["requests"] == 0


# ---------------------------------------------------------------------------
# REST-слой (без запуска поллеров: подменяем registry в app.state)
# ---------------------------------------------------------------------------
class _FakeRegistry:
    def __init__(self):
        self.reset_calls = []

    def stats_snapshot(self):
        return {"hosts": {"a": {"requests": 1, "processed_tokens": 5,
                                "generated_tokens": 2, "cached_tokens": 0}},
                "total": {"requests": 1, "processed_tokens": 5,
                          "generated_tokens": 2, "cached_tokens": 0}}

    def reset_host(self, hid):
        self.reset_calls.append(("host", hid))
        return {"requests": 0, "processed_tokens": 0, "generated_tokens": 0,
                "cached_tokens": 0}

    def reset_all(self):
        self.reset_calls.append(("all", None))
        return {"hosts": {}, "total": {"requests": 0, "processed_tokens": 0,
                                       "generated_tokens": 0, "cached_tokens": 0}}


def _app_with(fake):
    app = FastAPI()
    app.state.registry = fake
    app.include_router(api_router)
    return TestClient(app)


def test_api_get_stats():
    client = _app_with(_FakeRegistry())
    r = client.get("/api/stats")
    assert r.status_code == 200
    assert r.json()["total"]["requests"] == 1


def test_api_reset_host():
    fake = _FakeRegistry()
    client = _app_with(fake)
    r = client.post("/api/hosts/a/stats/reset")
    assert r.status_code == 200
    assert r.json()["ok"] is True
    assert fake.reset_calls == [("host", "a")]


def test_api_reset_all():
    fake = _FakeRegistry()
    client = _app_with(fake)
    r = client.post("/api/stats/reset")
    assert r.status_code == 200
    assert fake.reset_calls == [("all", None)]


def test_api_reset_unknown_host_404():
    class _R(_FakeRegistry):
        def reset_host(self, hid):
            return None
    client = _app_with(_R())
    r = client.post("/api/hosts/ghost/stats/reset")
    assert r.status_code == 404
