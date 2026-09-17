<template>
  <div class="slots">
    <div v-for="slot in slots" :key="slot.id" class="slot glass">
      <div class="panel-head">
        <span class="panel-title">Slot {{ slot.id + 1 }}</span>
        <span class="badge" :class="slot.is_processing ? 'ok' : 'dim'">
          {{ slot.is_processing ? t('slotTable.processing') : t('slotTable.idle') }}
        </span>
      </div>

      <div class="grid2">
        <div class="kv"><span class="k">{{ t('slotTable.task_id') }}</span><span class="v mono">#{{ slot.id_task ?? '—' }}</span></div>
        <div class="kv"><span class="k">n_ctx</span><span class="v mono">{{ fmtNum(slot.n_ctx) }}</span></div>
        <div class="kv"><span class="k">Prompt tokens</span><span class="v mono">{{ fmtNum(slot.n_prompt_tokens) }}</span></div>
        <div class="kv"><span class="k">{{ t('slotTable.processed_cached') }}</span><span class="v mono">{{ fmtNum(slot.n_prompt_tokens_processed) }} / {{ fmtNum(slot.n_prompt_tokens_cache) }}</span></div>
        <div class="kv"><span class="k">{{ t('slotTable.decoded') }}</span><span class="v mono">{{ fmtNum(slot.n_decoded) }}</span></div>
        <div class="kv"><span class="k">{{ t('slotTable.remaining') }}</span><span class="v mono">{{ fmtNum(slot.n_remain) }}</span></div>
        <div class="kv">
          <span class="k">{{ t('slotTable.context_usage') }}</span>
          <span class="v mono" :class="ctxClass">{{ ctxText }}</span>
        </div>
        <div class="kv"><span class="k">{{ t('slotTable.speculative_decoding') }}</span><span class="v mono">{{ slot.speculative ? t('slotTable.enabled') : '—' }}</span></div>
      </div>

      <div class="kv">
        <span class="k">{{ t('slotTable.speed') }}</span>
        <span class="v mono">
          <span style="color: var(--cyan)">gen {{ (slot.gen_speed_tps || 0).toFixed(1) }} t/s</span>
          <span class="faint"> · </span>
          <span style="color: var(--green)">prompt {{ (slot.prompt_speed_tps || 0).toFixed(1) }} t/s</span>
        </span>
      </div>

      <div class="collapse-head" :class="{ open: openSlots.has(slot.id) }" @click="toggle(slot.id)">
        <span class="arrow">▸</span> {{ t('slotTable.sampling_params') }}（{{ paramRows.length }}）
      </div>
      <div class="collapse-body" :class="{ open: openSlots.has(slot.id) }">
        <div class="params">
          <div v-for="[k, v] in paramRows" :key="k" class="flag">
            <span class="fk mono">{{ k }}</span>
            <span class="fv mono">{{ v }}</span>
          </div>
        </div>
      </div>
    </div>
    <div v-if="!slots.length" class="glass placeholder"><span class="icon">⌁</span>{{ t('slotTable.no_data') }}</div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { fmtNum } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  slots: { type: Array, default: () => [] }
})

const openSlots = ref(new Set())

function toggle(id) {
  const s = new Set(openSlots.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  openSlots.value = s
}

const ctxText = computed(() => {
  const slot = props.slots[0] || {}
  if (slot.ctx_pct === null || slot.ctx_pct === undefined) return '—'
  return `${fmtNum(slot.ctx_used)} (${slot.ctx_pct.toFixed(1)}%)`
})
const ctxClass = computed(() => {
  const slot = props.slots[0] || {}
  const p = slot.ctx_pct
  if (p === null || p === undefined) return ''
  if (p >= 90) return 'lv-danger'
  if (p >= 80) return 'lv-warn'
  return ''
})

const paramRows = computed(() => {
  const slot = props.slots[0] || {}
  const p = slot.params || {}
  const rows = []
  for (const [k, v] of Object.entries(p)) {
    if (v === null || v === undefined || v === '' || v === false) continue
    if (k === 'speculative' || k === 'timings_per_token' || k === 'post_sampling_probs') continue
    if (typeof v === 'object') {
      for (const [k2, v2] of Object.entries(v)) {
        if (v2 === null || v2 === undefined || v2 === '' || v2 === false) continue
        rows.push([`${k}.${k2}`, v2])
      }
    } else {
      rows.push([k, v])
    }
  }
  return rows
})
</script>

<style scoped>
.slots { display: grid; grid-template-columns: repeat(auto-fit, minmax(420px, 1fr)); gap: 12px; }
.slot { padding: 12px 16px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.panel-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }
.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 0 18px; margin-bottom: 4px; }
.params { display: grid; grid-template-columns: repeat(2, 1fr); gap: 2px 18px; margin: 4px 0 8px; }
.flag { display: flex; justify-content: space-between; gap: 8px; font-size: 11px; padding: 1px 0; }
.fk { color: var(--text-dim); }
.fv { color: var(--text); text-align: right; word-break: break-all; }
</style>
