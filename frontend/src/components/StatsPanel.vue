<template>
  <div v-if="rows.length" class="stats-panel glass">
    <div class="panel-header">
      <span class="panel-title">{{ t('stats.title') }}</span>
      <span class="panel-hint">
        {{ t('stats.hint_since') }}
        <span v-if="windowSince" class="mono dim">{{ t('stats.window_since', { time: windowSince }) }}</span>
      </span>
      <button
        class="btn-reset-all"
        :class="{ confirming: confirmState === 'pending' }"
        :title="t('stats.reset_all')"
        @click="onResetAll"
      >
        {{ confirmState === 'pending' ? t('stats.reset_all_confirm') : t('stats.reset_all') }}
      </button>
    </div>

    <table class="tbl">
      <thead>
        <tr>
          <th>{{ t('stats.column_backend') }}</th>
          <th class="num">{{ t('stats.requests') }}</th>
          <th class="num" :title="t('stats.cached_note')">
            {{ t('stats.cached') }}
            <span class="info-dot" @click.stop="showTooltip = !showTooltip">?</span>
          </th>
          <th class="num">{{ t('stats.processed') }}</th>
          <th class="num">{{ t('stats.generated') }}</th>
          <th class="num" style="width: 36px"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.id">
          <td class="mono">{{ row.name }}</td>
          <td class="num mono">{{ fmtTokens(row.stats.requests) }}</td>
          <td class="num mono">{{ fmtTokens(row.stats.cached_tokens) }}</td>
          <td class="num mono">{{ fmtTokens(row.stats.processed_tokens) }}</td>
          <td class="num mono">{{ fmtTokens(row.stats.generated_tokens) }}</td>
          <td class="num">
            <button class="btn-reset" :title="t('stats.reset')" @click="onResetHost(row.id)">
              ↺
            </button>
          </td>
        </tr>
        <tr class="row-total">
          <td class="mono"><b>{{ t('stats.total') }}</b></td>
          <td class="num mono">{{ fmtTokens(total.requests) }}</td>
          <td class="num mono">{{ fmtTokens(total.cached_tokens) }}</td>
          <td class="num mono">{{ fmtTokens(total.processed_tokens) }}</td>
          <td class="num mono">{{ fmtTokens(total.generated_tokens) }}</td>
          <td></td>
        </tr>
      </tbody>
    </table>

    <div v-if="showTooltip" class="tooltip-box">{{ t('stats.cached_note') }}</div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { api } from '../api'
import { fmtTokens } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  hosts: { type: Array, required: true }
})

// ---- data ----
const confirmState = ref('idle') // idle | pending | done
const showTooltip = ref(false)
let confirmTimer = null
let tooltipTimer = null

const rows = computed(() =>
  props.hosts
    .filter((h) => h.stats && Object.keys(h.stats).length > 0)
    .map((h) => ({ id: h.id, name: h.name, stats: h.stats }))
)

const total = computed(() => {
  let r = 0, c = 0, p = 0, g = 0
  for (const h of props.hosts) {
    if (!h.stats) continue
    r += (h.stats.requests || 0)
    c += (h.stats.cached_tokens || 0)
    p += (h.stats.processed_tokens || 0)
    g += (h.stats.generated_tokens || 0)
  }
  return { requests: r, cached_tokens: c, processed_tokens: p, generated_tokens: g }
})

const windowSince = computed(() => {
  // Show the earliest window_started across all hosts, as a human time.
  let earliest = null
  for (const h of props.hosts) {
    if (h.stats && h.stats.window_started) {
      if (earliest === null || h.stats.window_started < earliest) earliest = h.stats.window_started
    }
  }
  if (!earliest) return null
  return new Date(earliest * 1000).toLocaleString([], {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
  })
})

// ---- reset ----
async function onResetAll() {
  if (confirmState.value === 'pending') {
    try {
      await api.resetAllStats()
    } catch { /* WS will update; swallow */ }
    confirmState.value = 'idle'
    return
  }
  confirmState.value = 'pending'
  confirmTimer = setTimeout(() => { confirmState.value = 'idle' }, 3000)
}

async function onResetHost(hostId) {
  try {
    await api.resetHostStats(hostId)
  } catch { /* WS will update; swallow */ }
}

// ---- tooltip auto-dismiss ----
function startTooltipDismiss() {
  tooltipTimer = setTimeout(() => { showTooltip.value = false }, 5000)
}

watch(showTooltip, (v) => {
  if (v) startTooltipDismiss()
  else clearTimeout(tooltipTimer)
})

onMounted(() => { clearTimeout(confirmTimer) })
onBeforeUnmount(() => { clearTimeout(confirmTimer); clearTimeout(tooltipTimer) })
</script>

<style scoped>
.stats-panel {
  padding: 14px 18px;
  margin-bottom: 4px;
}
.panel-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.panel-title {
  font-size: 12px;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  text-transform: uppercase;
  font-weight: 600;
}
.panel-hint {
  font-size: 11px;
  color: var(--text-faint);
}
.btn-reset-all {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-dim);
  background: transparent;
  border: 1px solid var(--card-border);
  border-radius: 5px;
  padding: 3px 10px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}
.btn-reset-all:hover { color: var(--text); border-color: var(--card-border-hover); }
.btn-reset-all.confirming { color: var(--amber); border-color: var(--amber); }

/* table */
.tbl { width: 100%; border-collapse: collapse; font-size: 12px; }
.tbl th {
  text-align: left;
  color: var(--text-faint);
  font-weight: 500;
  padding: 4px 8px;
  border-bottom: 1px solid rgba(143, 163, 200, 0.15);
  white-space: nowrap;
  position: relative;
}
.tbl td {
  padding: 5px 8px;
  border-bottom: 1px solid rgba(143, 163, 200, 0.07);
  white-space: nowrap;
  color: var(--text);
}
.tbl tr:last-child td { border-bottom: none; }
.tbl td.num, .tbl th.num { text-align: right; }

/* per-row reset */
.btn-reset {
  background: transparent;
  border: none;
  color: var(--text-faint);
  font-size: 14px;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  transition: color 0.15s, background 0.15s;
  line-height: 1;
}
.btn-reset:hover { color: var(--cyan); background: rgba(0, 229, 255, 0.08); }

/* total row */
.row-total td {
  font-weight: 600;
  border-top: 2px solid var(--card-border);
  border-bottom: none;
  padding-top: 8px;
  color: var(--text);
}

/* cached info dot */
.info-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--card-border);
  color: var(--text-faint);
  font-size: 9px;
  font-weight: 700;
  cursor: help;
  margin-left: 4px;
  vertical-align: middle;
}
.info-dot:hover { background: var(--cyan); color: #fff; }

/* tooltip */
.tooltip-box {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-dim);
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: 6px;
  padding: 6px 10px;
  line-height: 1.4;
}
</style>
