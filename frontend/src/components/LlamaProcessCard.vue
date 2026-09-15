<template>
  <div class="proc glass">
    <div class="panel-head">
      <span class="panel-title">{{ t('llamaProcess.title') }}</span>
      <span v-if="!found" class="badge warn">{{ t('llamaProcess.not_found') }}</span>
    </div>

    <template v-if="found">
      <div v-for="(p, idx) in processList" :key="p.pid" :class="['proc-card', { multi: processList.length > 1 }]">
        <div v-if="processList.length > 1" class="proc-label">Process {{ idx + 1 }}</div>
        <div class="metrics mono">
          <div class="m"><span class="v">{{ p.pid }}</span><span class="k">PID</span></div>
          <div class="m"><span class="v" :class="cpuLevelClass(p)">{{ cpuRtText(p) }}</span><span class="k">{{ t('llamaProcess.realtime_cpu') }}</span></div>
          <div class="m"><span class="v">{{ cpuLifeText(p) }}</span><span class="k">{{ t('llamaProcess.cumulative_cpu') }}</span></div>
          <div class="m"><span class="v">{{ rssText(p) }}</span><span class="k">RSS</span></div>
          <div class="m"><span class="v">{{ vszText(p) }}</span><span class="k">VSZ</span></div>
          <div class="m"><span class="v">{{ threadsVal(p) ?? '—' }}</span><span class="k">{{ t('llamaProcess.threads') }}</span></div>
          <div class="m"><span class="v">{{ elapsedVal(p) || '—' }}</span><span class="k">{{ t('llamaProcess.uptime') }}</span></div>
        </div>

        <div v-if="service && service.active" class="service mono">
          <span class="k">{{ t('llamaProcess.service') }}</span><span class="v">{{ service.unit }} · {{ service.active }}</span>
          <span class="k">{{ t('llamaProcess.started_at') }}</span><span class="v">{{ service.since || '—' }}</span>
          <span class="k">{{ t('llamaProcess.cumulative_cpu') }}</span><span class="v">{{ service.cpu_total || '—' }}</span>
          <span class="k">{{ t('llamaProcess.memory') }}</span><span class="v">{{ service.memory || '—' }}<span v-if="service.memory_peak" class="faint"> ({{ t('llamaProcess.peak') }} {{ service.memory_peak }})</span></span>
          <span class="k">Tasks</span><span class="v">{{ service.tasks || '—' }}</span>
        </div>

        <div class="collapse-head" :class="{ open: p._cmdOpen }" @click="p._cmdOpen = !p._cmdOpen">
          <span class="arrow">▸</span> {{ t('llamaProcess.full_command_line') }}
        </div>
        <div class="collapse-body" :class="{ open: p._cmdOpen }">
          <pre class="cmdline mono">{{ p.cmdline || '—' }}</pre>
        </div>

        <div v-if="flagRows(p).length" class="collapse-head" :class="{ open: p._flagOpen }" @click="p._flagOpen = !p._flagOpen">
          <span class="arrow">▸</span> {{ t('llamaProcess.params_table') }}（{{ flagRows(p).length }}）
        </div>
        <div class="collapse-body" :class="{ open: p._flagOpen }">
          <div class="flags">
            <div v-for="[k, v] in flagRows(p)" :key="k" class="flag">
              <span class="fk mono">{{ k }}</span>
              <span class="fv mono">{{ v }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="placeholder"><span class="icon">⌁</span>{{ t('llamaProcess.not_found_alt') }}</div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { fmtBytes } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  process: { type: [Object, Array], default: () => ({}) },
  service: { type: Object, default: () => ({}) },
  // Модель текущего llama-server для фильтрации процессов
  modelPath: { type: String, default: '' }
})

// Сохраняем состояние раскрытия по PID
const _openState = ref({}) // { [pid]: { _cmdOpen: bool, _flagOpen: bool } }

function getOpenState(pid) {
  if (!_openState.value[pid]) {
    _openState.value[pid] = { _cmdOpen: false, _flagOpen: false }
  }
  return _openState.value[pid]
}

// 支持单进程对象或进程数组（新版 CLI Go → []ProcInfo，旧版 backend SSH → 单对象）
const processList = computed(() => {
  const raw = props.process
  let list = []
  if (Array.isArray(raw)) list = raw
  else if (raw && raw.list) list = raw.list
  else if (raw && raw.found !== undefined) list = [raw]

  // Фильтруем: если modelPath задан, показываем только процесс с этой моделью
  if (props.modelPath) {
    list = list.filter(p => {
      const cmd = (p.cmdline || '').toLowerCase()
      const mp = props.modelPath.toLowerCase()
      // Проверяем --model или -m параметр
      const m = cmd.match(/(?:--model\s+|-m\s+)(\S+)/i)
      if (!m) return false
      return m[1].toLowerCase() === mp
    })
    // Если ни один процесс не совпал — показываем все с меткой "non-match"
    if (!list.length) list = [] // можно раскомментировать для отладки: list = raw.list
  }

  // Восстанавливаем состояние раскрытия по PID
  for (const p of list) {
    if (p.pid != null) {
      const s = getOpenState(p.pid)
      if (p._cmdOpen === undefined) p._cmdOpen = s._cmdOpen
      if (p._flagOpen === undefined) p._flagOpen = s._flagOpen
    }
  }
  return list
})

const found = computed(() => processList.value.length > 0)

const cpuRtText = (p) => {
  const v = p.cpu_pct_realtime ?? null
  return v === null ? '—' : v.toFixed(1) + '%'
}
const cpuLifeText = (p) => {
  const v = p.cpu_pct_lifetime
  return v === null || v === undefined ? '—' : v.toFixed(1) + '%'
}
const rssText = (p) => (p.rss_mb ? fmtBytes(p.rss_mb * 1024 * 1024) : '—')
const vszText = (p) => (p.vsz_mb ? fmtBytes(p.vsz_mb * 1024 * 1024) : '—')
const threadsVal = (p) => p.threads
const elapsedVal = (p) => p.elapsed
const flagRows = (p) => Object.entries(p.flags || {})
const cpuLevelClass = (p) => {
  const v = p.cpu_pct_realtime
  if (v === null) return ''
  if (v >= 900) return 'lv-danger'
  if (v >= 800) return 'lv-warn'
  return ''
}
</script>

<style scoped>
.proc { padding: 12px 16px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.panel-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }
.proc-card { margin-bottom: 8px; }
.proc-card.multi { border-left: 3px solid var(--accent); padding-left: 8px; }
.proc-label { font-size: 10px; color: var(--text-dim); margin-bottom: 4px; text-transform: uppercase; letter-spacing: 0.5px; }
.metrics { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px 12px; margin-bottom: 10px; }
.m { display: flex; flex-direction: column; gap: 2px; }
.m .v { font-size: 16px; font-weight: 700; color: var(--text); }
.m .k { font-size: 10px; color: var(--text-faint); }
.service {
  display: grid;
  grid-template-columns: 52px 1fr;
  column-gap: 8px;
  row-gap: 3px;
  border-top: 1px solid rgba(143, 163, 200, 0.1);
  padding-top: 8px;
  margin-bottom: 6px;
  font-size: 11px;
}
.service .k { color: var(--text-faint); }
.service .v { color: var(--text); word-break: break-all; }
.cmdline {
  background: rgba(5, 8, 14, 0.6);
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 11px;
  color: var(--text-dim);
  white-space: pre-wrap;
  word-break: break-all;
  margin: 4px 0 8px;
  max-height: 140px;
  overflow-y: auto;
}
.flags { display: grid; grid-template-columns: repeat(3, 1fr); gap: 2px 18px; margin: 4px 0 8px; }
.flag { display: flex; justify-content: space-between; gap: 8px; font-size: 11px; padding: 1px 0; }
.fk { color: var(--text-dim); }
.fv { color: var(--text); text-align: right; word-break: break-all; }
</style>
