<template>
  <header class="topbar">
    <router-link to="/" class="back">← {{ t('topBar.back') }}</router-link>
    <div class="host-info">
      <span class="dot" :class="dotClass"></span>
      <span class="hostname">{{ hostName }}</span>
      <span v-if="modelName" class="model mono dim" :title="modelTitle">· {{ modelName }}</span>
    </div>

    <div v-if="stats" class="stats">
      <span class="stat" :class="genLevel" :title="genTip">
        <span class="k">{{ t('topBar.speed') }}</span><span class="v mono">{{ genText }}</span>
      </span>
      <span class="stat" :class="mtpLevel" :title="mtpTip">
        <span class="k">MTP</span><span class="v mono">{{ mtpText }}</span>
      </span>
      <span class="stat" :class="ctxLevel" :title="ctxTip">
        <span class="k">{{ t('topBar.context') }}</span><span class="v mono">{{ ctxText }}</span>
      </span>
      <span v-for="g in gpuChips" :key="g.idx" class="stat" :class="g.level" :title="`GPU${g.idx} ${t('topBar.util_temp_power_mem')}`">
        <span class="k">GPU{{ g.idx }}</span><span class="v mono">{{ g.text }}</span>
      </span>
      <span class="stat" :class="memLevel" :title="memTip">
        <span class="k">{{ t('topBar.memory') }}</span><span class="v mono">{{ memText }}</span>
      </span>
      <span class="stat" :class="cpuLevel" :title="t('topBar.cpu_util')">
        <span class="k">CPU</span><span class="v mono">{{ cpuText }}</span>
      </span>
    </div>

    <div class="right">
      <span v-if="gpuTemp !== null" class="temp-badge" :class="tempLevel" :title="`${Math.round(gpuTemp)}°C`">
        <svg class="thermo" viewBox="0 0 16 24" width="14" height="22" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M8 2a4.5 4.5 0 0 0-4.5 4.5c0 2.8 2 5.5 3.5 7.2V18a1 1 0 1 0 2 0v-4.3c1.5-1.7 3.5-4.4 3.5-7.2A4.5 4.5 0 0 0 8 2z"/>
          <circle cx="8" cy="18" r="2"/>
        </svg>
        <span class="tv">{{ gpuTemp.toFixed(0) }}°</span>
      </span>
      <LanguageSwitcher />
      <ThemeSwitcher />
      <span v-if="!llamaOnline" class="badge danger">{{ t('topBar.llama_offline') }}</span>
      <span v-else-if="!sshOk" class="badge warn">{{ t('topBar.ssh_disconnected') }}</span>
      <span v-else class="badge ok">{{ t('topBar.online') }}</span>
      <LiveClock />

      <span class="mode-indicator" :class="modeDotClass" :title="t('topBar.data_channel')"></span>
      <select :value="mode" class="mode-select mono" @change="onModeChange">
        <option value="ws">{{ t('topBar.realtime') }}</option>
        <option value="1s">1s</option>
        <option value="2s">2s</option>
        <option value="5s">5s</option>
        <option value="paused">{{ t('topBar.paused') }}</option>
      </select>
      <span v-if="degraded" class="badge warn small">{{ t('topBar.ws_disconnected') }}</span>
    </div>
  </header>
</template>

<script setup>
import { computed } from 'vue'
import { fmtTokens, fmtGB, alertLevel } from '../utils'
import LiveClock from './LiveClock.vue'
import LanguageSwitcher from './LanguageSwitcher.vue'
import ThemeSwitcher from './ThemeSwitcher.vue'
import { t } from '../i18n'

const props = defineProps({
  hostName: { type: String, default: '' },
  modelName: { type: String, default: '' },
  modelTitle: { type: String, default: '' },
  llamaOnline: { type: Boolean, default: false },
  sshOk: { type: Boolean, default: false },
  stats: { type: Object, default: null },
  gpuTemp: { type: Number, default: null },
  mode: { type: String, default: 'ws' },
  degraded: { type: Boolean, default: false },
  connected: { type: Boolean, default: false }
})
const emit = defineEmits(['update:mode'])

const st = computed(() => props.stats || {})
const alerts = computed(() => st.value.alerts || [])
const num = (v) => (v === null || v === undefined || Number.isNaN(v) ? null : v)

const genText = computed(() => {
  const s = st.value
  if (!s.online) return '—'
  const p = num(s.prompt)
  const g = num(s.gen)
  const v = g > 0 ? g : p
  return v === null || v <= 0 ? '—' : `${v.toFixed(1)} t/s`
})
const genTip = computed(() => {
  const s = st.value
  if (!s.online) return 'llama offline'
  return `gen ${num(s.gen) || 0} t/s · prompt ${num(s.prompt) || 0} t/s · ${t('topBar.source')} ${s.speedSource || '—'}`
})
const genLevel = computed(() => (st.value.online ? 'normal' : 'off'))

const mtpText = computed(() => {
  const v = num(st.value.mtp)
  return v === null ? '—' : `${v.toFixed(1)}%`
})
const mtpTip = computed(() => (num(st.value.mtp) === null ? t('topBar.wait_task_end') : t('topBar.mtp_acceptance')))
const mtpLevel = computed(() => (num(st.value.mtp) === null ? 'off' : alertLevel(alerts.value, 'mtp')))

const ctxText = computed(() => {
  const s = st.value
  const r = num(s.ctxRemain)
  return r === null ? '—' : `${fmtTokens(r)}/${fmtTokens(s.ctxTotal)}`
})
const ctxTip = computed(() => {
  const s = st.value
  const r = num(s.ctxRemain)
  if (r === null) return t('topBar.wait_task_end')
  return `${t('topBar.remaining')} ${fmtTokens(r)} · ${t('topBar.used')} ${fmtTokens(s.ctxUsed)} (${num(s.ctxPct) === null ? '—' : s.ctxPct}%)`
})
const ctxLevel = computed(() => (num(st.value.ctxRemain) === null ? 'off' : alertLevel(alerts.value, 'ctx')))

const gpuChips = computed(() =>
  (st.value.gpus || []).map((g) => ({
    idx: g.idx,
    level: alertLevel(alerts.value, `gpu${g.idx}.`),
    text: [
      num(g.util) === null ? '—' : `${Math.round(g.util)}%`,
      num(g.temp) === null ? '—' : `${Math.round(g.temp)}°`,
      num(g.power) === null ? '—' : `${Math.round(g.power)}W`,
      num(g.memUsed) === null || !g.memTotal ? '—' : `${(g.memUsed / 1024).toFixed(1)}/${(g.memTotal / 1024).toFixed(0)}G`
    ].join(' ')
  }))
)

const memText = computed(() => {
  const s = st.value
  const u = num(s.memUsed)
  return u === null || !s.memTotal ? '—' : `${(u / 1024).toFixed(1)}/${(s.memTotal / 1024).toFixed(1)}G`
})
const memTip = computed(() => {
  const s = st.value
  const u = num(s.memUsed)
  return u === null || !s.memTotal ? t('topBar.no_data_ssh') : `${t('topBar.mem_used')} ${fmtGB(u)} / ${t('topBar.total')} ${fmtGB(s.memTotal)}`
})
const memLevel = computed(() => (num(st.value.memUsed) === null ? 'off' : alertLevel(alerts.value, 'mem')))

const cpuText = computed(() => {
  const v = num(st.value.cpu)
  return v === null ? '—' : `${Math.round(v)}%`
})
const cpuLevel = computed(() => (num(st.value.cpu) === null ? 'off' : alertLevel(alerts.value, 'cpu')))

const tempLevel = computed(() => {
  const t = props.gpuTemp
  if (t === null) return ''
  if (t >= 90) return 'danger'
  if (t >= 75) return 'warn'
  return ''
})

const dotClass = computed(() => {
  if (props.llamaOnline && props.sshOk) return 'online'
  if (props.sshOk) return 'warn'
  return 'offline'
})
const modeDotClass = computed(() => {
  if (props.mode === 'paused') return 'gray'
  if (props.mode === 'ws') return props.connected ? 'green' : 'amber'
  return 'amber'
})

function onModeChange(e) {
  emit('update:mode', e.target.value)
}
</script>

<style scoped>
.topbar {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 0 24px;
  border-bottom: 1px solid rgba(0, 229, 255, 0.12);
  background: rgba(10, 14, 23, 0.6);
  backdrop-filter: blur(12px);
  position: sticky;
  top: var(--chrome-top, 0px);
  z-index: 20;
}
.back { color: var(--text-dim); font-size: 13px; flex: none; }
.back:hover { color: var(--cyan); text-decoration: none; }
.host-info { display: flex; align-items: center; gap: 10px; min-width: 0; }
.hostname { font-size: 15px; font-weight: 600; }
.model { color: var(--text-dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.stats { display: flex; align-items: center; gap: 6px; min-width: 0; overflow-x: auto; scrollbar-width: none; }
.stats::-webkit-scrollbar { display: none; }
.stat {
  flex: none;
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 3px 9px;
  border: 1px solid var(--card-border);
  border-radius: 6px;
  background: rgba(16, 24, 40, 0.55);
  white-space: nowrap;
}
.stat .k { font-size: 10px; color: var(--text-faint); }
.stat .v { font-size: 12px; color: var(--text); }
.stat.warn .v { color: var(--amber); }
.stat.danger .v { color: var(--red); }
.stat.off .v { color: var(--text-faint); }
.right { margin-left: auto; display: flex; align-items: center; gap: 12px; }
.temp-badge {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 2px 8px; border-radius: 12px;
  background: rgba(16, 24, 40, 0.55);
  border: 1px solid var(--card-border);
  font-size: 12px; color: var(--cyan);
}
.temp-badge.warn { color: var(--amber); }
.temp-badge.danger { color: var(--red); }
.thermo { opacity: 0.8; }
.tv { font-weight: 600; }
.mode-indicator { width: 8px; height: 8px; border-radius: 50%; }
.mode-indicator.green { background: var(--green); box-shadow: 0 0 6px var(--green); }
.mode-indicator.amber { background: var(--amber); box-shadow: 0 0 6px var(--amber); }
.mode-indicator.gray { background: var(--text-faint); }
.mode-select {
  background: rgba(16, 24, 40, 0.9);
  color: var(--text);
  border: 1px solid var(--card-border);
  border-radius: 6px;
  font-size: 12px;
  padding: 4px 8px;
  outline: none;
  cursor: pointer;
}
.mode-select:hover { border-color: var(--card-border-hover); }
</style>
