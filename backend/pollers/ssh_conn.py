"""SSH 连接管理器（SshConnection）。

- paramiko 为阻塞库，所有阻塞调用通过 run_in_executor 放入线程池，避免阻塞事件循环。
- 持久连接 + keepalive；断线指数退避重连（1s→2s→4s→...→30s 封顶）。
- 断线/重连产生 ssh_down / ssh_up 事件。
- 兼容 Python 3.9。
"""
import asyncio
import logging
import time
from typing import Optional

import paramiko

from ..config import SshCfg
from ..events import EventDetector

log = logging.getLogger("llamalens.ssh")

MAX_BACKOFF = 30.0


class SshConnection:
    def __init__(self, cfg: SshCfg, events: EventDetector, host_id: str):
        self.cfg = cfg
        self.events = events
        self.host_id = host_id
        self.client: Optional[paramiko.SSHClient] = None
        self.connected = False
        self._lock = asyncio.Lock()
        self._backoff = 1.0
        self._emitted_down = False
        self._fail_streak = 0
        self._cooldown_until = 0.0

    # ------------------------------------------------------------------
    def _connect_sync(self) -> paramiko.SSHClient:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        kwargs = dict(
            hostname=self.cfg.host,
            port=self.cfg.port,
            username=self.cfg.user,
            timeout=self.cfg.timeout,
            allow_agent=False,
            look_for_keys=False,
        )
        if self.cfg.key_path:
            kwargs["key_filename"] = self.cfg.key_path
        if self.cfg.password:
            kwargs["password"] = self.cfg.password
        client.connect(**kwargs)
        transport = client.get_transport()
        if transport:
            transport.set_keepalive(self.cfg.keepalive)
        return client

    def _is_alive(self) -> bool:
        try:
            if self.client is None:
                return False
            t = self.client.get_transport()
            return t is not None and t.is_active()
        except Exception:
            return False

    def _drop(self) -> None:
        if self.client is not None:
            try:
                self.client.close()
            except Exception:
                pass
        self.client = None
        was_connected = self.connected
        self.connected = False
        if was_connected and not self._emitted_down:
            self._emitted_down = True
            self.events.set_ssh_connected(time.time(), False)

    # ------------------------------------------------------------------
    def _cooldown(self) -> None:
        """失败后指数冷却（1s→2s→4s→...→30s 封顶），防远端过载时高频重连。"""
        self._fail_streak += 1
        self._cooldown_until = time.time() + min(30.0, 1.0 * (2 ** min(self._fail_streak, 5)))

    async def ensure_connected(self) -> bool:
        if time.time() < self._cooldown_until:
            return False
        async with self._lock:
            if self._is_alive():
                self._on_up()
                return True
            # 需要（重）连接
            try:
                client = await asyncio.get_running_loop().run_in_executor(
                    None, self._connect_sync)
                self.client = client
                self.connected = True
                self._backoff = 1.0
                self._fail_streak = 0
                self._on_up()
                log.info("[%s] SSH 已连接 %s@%s:%s", self.host_id,
                         self.cfg.user, self.cfg.host, self.cfg.port)
                return True
            except Exception as e:
                self._drop()
                log.warning("[%s] SSH 连接失败: %s: %s", self.host_id, type(e).__name__, e)
                self._cooldown()
                return False

    def _on_up(self):
        if self._emitted_down:
            self._emitted_down = False
            self.events.set_ssh_connected(time.time(), True)

    async def exec_command(self, cmd: str, timeout: Optional[float] = None) -> Optional[str]:
        """执行命令并返回 stdout。失败返回 None。"""
        if time.time() < self._cooldown_until:
            return None
        if not await self.ensure_connected():
            return None
        timeout = timeout or self.cfg.timeout

        def _run():
            import select
            stdin, stdout, stderr = self.client.exec_command(cmd, timeout=timeout)
            try:
                stdout.channel.settimeout(timeout)
            except Exception:
                pass
            try:
                stderr.channel.settimeout(timeout)
            except Exception:
                pass
            # Read both channels concurrently via select to avoid deadlock
            stdout_buf = ""
            stderr_buf = ""
            while True:
                channels = []
                if not stdout_buf:
                    channels.append(stdout)
                if not stderr_buf:
                    channels.append(stderr)
                if not channels:
                    break
                try:
                    readable, _, _ = select.select(channels, [], [], 0.5)
                except EOFError:
                    break
                except Exception:
                    break
                for ch in readable:
                    try:
                        data = ch.recv(4096)
                        if data:
                            if ch is stdout:
                                stdout_buf += data.decode("utf-8", "replace")
                            else:
                                stderr_buf += data.decode("utf-8", "replace")
                        else:
                            if ch is stdout:
                                stdout_buf = None  # EOF marker
                            else:
                                stderr_buf = None  # EOF marker
                    except EOFError:
                        if ch is stdout:
                            stdout_buf = ""  # EOF, stop reading
                        else:
                            stderr_buf = ""  # EOF, stop reading
                    except Exception:
                        if ch is stdout:
                            stdout_buf = ""
                        else:
                            stderr_buf = ""
                # If both channels hit EOF, we're done
                if stdout_buf == "" and stderr_buf == "":
                    break
            # Log non-empty stderr at DEBUG (e.g. "nvidia-smi: command not found" on non-GPU hosts)
            if stderr_buf:
                trimmed = stderr_buf[:2000]
                log.debug("[%s] SSH stderr: %s", self.host_id, trimmed)
            return stdout_buf

        try:
            out = await asyncio.get_running_loop().run_in_executor(None, _run)
            self._fail_streak = 0
            return out
        except Exception as e:
            log.warning("[%s] SSH 命令执行失败: %s: %s", self.host_id, type(e).__name__, e)
            self._drop()
            self._cooldown()
            return None

    def open_stream(self, cmd: str):
        """打开长驻流式通道（日志跟随用）。调用方须先 ensure_connected。返回 stdout。"""
        stdin, stdout, stderr = self.client.exec_command(cmd, timeout=None)
        return stdout

    async def close(self) -> None:
        async with self._lock:
            self._drop()
