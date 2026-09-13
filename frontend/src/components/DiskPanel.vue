<template>
  <div class="panel glass" :class="levelClass">
    <div class="panel-head">
      <span class="panel-title">{{ t('diskPanel.title') }}</span>
      <span class="mono dim small">{{ t('diskPanel.read') }} {{ readText }} MB/s · {{ t('diskPanel.write') }} {{ writeText }} MB/s</span>
    </div>
    <table class="tbl mono">
      <thead>
        <tr><th>{{ t('diskPanel.mount') }}</th><th class="num">{{ t('diskPanel.capacity') }}</th><th class="num">{{ t('diskPanel.used') }}</th><th class="num">{{ t('diskPanel.available') }}</th><th class="num">{{ t('diskPanel.usage_rate') }}</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="m in mounts" :key="m.mount">
          <td>{{ m.mount }}</td>
          <td class="num">{{ m.size_gb.toFixed(0) }}G</td>
          <td class="num">{{ m.used_gb.toFixed(0) }}G</td>
          <td class="num">{{ m.avail_gb.toFixed(0) }}G</td>
          <td class="num" :class="valClass(m.use_pct)">{{ m.use_pct.toFixed(0) }}%</td>
          <td class="barcell"><div class="bar"><i :class="barClass(m.use_pct)" :style="{ width: m.use_pct + '%' }"></i></div></td>
        </tr>
        <tr v-if="!mounts.length"><td colspan="6" class="faint">{{ t('diskPanel.no_data') }}</td></tr>
      </tbody>
    </table>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { t } from '../i18n'

const props = defineProps({
  disk: { type: Object, default: () => ({}) },
  alerts: { type: Array, default: () => [] }
})

const mounts = computed(() => props.disk.mounts || [])
const readText = computed(() => (props.disk.read_mb_s === null || props.disk.read_mb_s === undefined ? '—' : props.disk.read_mb_s.toFixed(1)))
const writeText = computed(() => (props.disk.write_mb_s === null || props.disk.write_mb_s === undefined ? '—' : props.disk.write_mb_s.toFixed(1)))

function valClass(v) {
  if (v >= 90) return 'lv-danger'
  if (v >= 80) return 'lv-warn'
  return ''
}
function barClass(v) {
  if (v >= 90) return 'danger'
  if (v >= 80) return 'warn'
  return ''
}

const level = computed(() => {
  let level = 'normal'
  for (const a of props.alerts || []) {
    if (!a.metric.startsWith('disk:')) continue
    if (a.level === 'danger') return 'danger'
    if (a.level === 'warn') level = 'warn'
  }
  return level
})
const levelClass = computed(() => (level.value === 'danger' ? 'card-danger' : level.value === 'warn' ? 'card-warn' : ''))
</script>

<style scoped>
.panel { padding: 12px 16px; }
.panel-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 8px; }
.panel-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }
.barcell { width: 120px; }
.barcell .bar { height: 5px; }
</style>
