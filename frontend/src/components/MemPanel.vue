<template>
  <div class="panel glass" :class="levelClass">
    <div class="panel-head">
      <span class="panel-title">{{ t('memPanel.title') }}</span>
      <span class="mono dim small">{{ fmtNum(totalMb, 0) }} MB {{ t('memPanel.total') }}</span>
    </div>

    <div class="stack-bar">
      <i class="used" :style="{ width: usedPct + '%' }" :title="`${t('memPanel.used')} ${fmtNum(usedMb, 0)} MB`"></i>
      <i class="cache" :style="{ width: cachePct + '%' }" :title="`buff/cache ${fmtNum(cacheMb, 0)} MB`"></i>
    </div>
    <div class="legend small mono">
      <span><i class="sw used"></i>{{ t('memPanel.used') }} {{ fmtNum(usedMb, 0) }} MB</span>
      <span><i class="sw cache"></i>buff/cache {{ fmtNum(cacheMb, 0) }} MB</span>
      <span><i class="sw free"></i>{{ t('memPanel.available') }} {{ fmtNum(availMb, 0) }} MB</span>
    </div>

    <div class="kv-row">
      <div class="kv"><span class="k">{{ t('memPanel.usage_rate') }}</span><span class="v mono" :class="levelClass">{{ usedPctText }}</span></div>
      <div class="kv"><span class="k">Swap</span><span class="v mono">{{ swapText }}</span></div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { fmtNum } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  mem: { type: Object, default: () => ({}) },
  alerts: { type: Array, default: () => [] }
})

const totalMb = computed(() => props.mem.total_mb || 0)
const usedMb = computed(() => props.mem.used_mb ?? 0)
const cacheMb = computed(() => props.mem.buff_cache_mb ?? 0)
const availMb = computed(() => props.mem.available_mb ?? 0)

const usedPct = computed(() => (totalMb.value ? (usedMb.value / totalMb.value) * 100 : 0))
const cachePct = computed(() => (totalMb.value ? (cacheMb.value / totalMb.value) * 100 : 0))
const usedPctText = computed(() => (totalMb.value ? usedPct.value.toFixed(1) + '%' : '—'))
const swapText = computed(() => {
  const st = props.mem.swap_total_mb || 0
  const su = props.mem.swap_used_mb ?? 0
  return st ? `${fmtNum(su, 0)} / ${fmtNum(st, 0)} MB` : t('memPanel.no_swap')
})

const level = computed(() => {
  const a = (props.alerts || []).find((x) => x.metric === 'mem')
  return a ? a.level : 'normal'
})
const levelClass = computed(() => (level.value === 'danger' ? 'lv-danger' : level.value === 'warn' ? 'lv-warn' : ''))
</script>

<style scoped>
.panel { padding: 12px 16px; }
.panel-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 8px; }
.panel-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }
.stack-bar {
  display: flex;
  height: 10px;
  border-radius: 5px;
  overflow: hidden;
  background: rgba(143, 163, 200, 0.12);
  margin-bottom: 6px;
}
.stack-bar i { display: block; height: 100%; transition: width 0.3s ease; }
.stack-bar .used { background: var(--cyan); box-shadow: 0 0 6px rgba(0, 229, 255, 0.5); }
.stack-bar .cache { background: rgba(0, 255, 157, 0.55); }
.legend { display: flex; gap: 14px; color: var(--text-dim); margin-bottom: 8px; }
.legend .sw { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 4px; }
.legend .sw.used { background: var(--cyan); }
.legend .sw.cache { background: rgba(0, 255, 157, 0.55); }
.legend .sw.free { background: rgba(143, 163, 200, 0.25); }
.kv-row { display: grid; grid-template-columns: 1fr 1fr; gap: 0 18px; }
</style>
