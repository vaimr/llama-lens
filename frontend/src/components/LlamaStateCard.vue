<template>
  <div class="state-card glass" :class="cardClass">
    <div class="head">
      <span class="phase" :class="phaseClass">{{ phaseText }}</span>
      <span v-if="logAvailable" class="badge ok small">{{ t('llamaState.log_live') }}</span>
      <span v-else class="badge dim small">{{ t('llamaState.log_unavailable') }}</span>
      <span v-if="elapsedText" class="elapsed mono small">{{ t('llamaState.running') }} {{ elapsedText }}</span>
    </div>

    <template v-if="logAvailable">
      <div class="kv" v-if="state.task_id !== null && state.task_id !== undefined">
        <span class="k">{{ t('llamaState.task_id') }}</span>
        <span class="v mono">#{{ state.task_id }}<span v-if="state.is_child" class="dim"> · {{ t('llamaState.child_task') }}</span><span v-if="slotInfo" class="dim"> · {{ t('llamaState.slot') }} {{ slotInfo }}</span></span>
      </div>

      <template v-if="phase === 'prompt_processing'">
        <div class="progress-wrap">
          <div class="bar"><i class="green" :style="{ width: (state.prompt_progress || 0) * 100 + '%' }"></i></div>
          <span class="mono small dim">{{ ((state.prompt_progress || 0) * 100).toFixed(0) }}%</span>
        </div>
        <div class="stat3">
          <div class="stat">
            <span class="s-label">{{ t('llamaState.prompt_speed') }}</span>
            <span class="s-val mono green">{{ state.prompt_speed_tps === null ? '—' : state.prompt_speed_tps.toFixed(1) }}</span>
            <span class="s-unit">t/s</span>
          </div>
          <div class="stat">
            <span class="s-label">{{ t('llamaState.prompt_processed') }}</span>
            <span class="s-val mono">{{ promptProcessed === null ? '—' : fmtNum(promptProcessed) }}</span>
            <span class="s-unit">/ {{ promptTotal ? fmtNum(promptTotal) : '—' }}</span>
          </div>
          <div class="stat">
            <span class="s-label">{{ t('llamaState.elapsed') }}</span>
            <span class="s-val mono">{{ state.prompt_elapsed_s === null ? '—' : state.prompt_elapsed_s.toFixed(1) }}</span>
            <span class="s-unit">s</span>
          </div>
        </div>
        <div class="kv-grid">
          <div class="kv" v-if="promptTotal">
            <span class="k">{{ t('llamaState.prompt_total') }}</span><span class="v mono">{{ fmtNum(promptTotal) }} tokens</span>
          </div>
          <div class="kv" v-if="prefillEta !== null">
            <span class="k">{{ t('llamaState.eta_remaining') }}</span><span class="v mono" style="color: var(--green)">{{ fmtDuration(prefillEta) }}</span>
          </div>
          <div class="kv" v-if="ctx.total">
            <span class="k">{{ t('llamaState.context_usage') }}</span>
            <span class="v mono">{{ fmtNum(ctx.used) }} / {{ fmtNum(ctx.total) }} <span class="dim">({{ ctx.pct }}%)</span></span>
          </div>
          <div class="kv" v-if="cacheHit !== null">
            <span class="k">{{ t('llamaState.kv_cache_hit') }}</span><span class="v mono" style="color: var(--green)">{{ (cacheHit * 100).toFixed(1) }}%</span>
          </div>
        </div>
      </template>

      <template v-else-if="phase === 'decoding'">
        <div class="stat3">
          <div class="stat">
            <span class="s-label">{{ t('llamaState.decoded') }}</span>
            <span class="s-val mono cyan">{{ fmtNum(state.n_decoded) }}</span>
            <span class="s-unit">tokens</span>
          </div>
          <div class="stat">
            <span class="s-label">{{ t('llamaState.realtime_speed') }}</span>
            <span class="s-val mono cyan">{{ state.tg_3s_tps === null ? '—' : state.tg_3s_tps.toFixed(2) }}</span>
            <span class="s-unit">t/s</span>
          </div>
          <div class="stat">
            <span class="s-label">{{ t('llamaState.avg_speed') }}</span>
            <span class="s-val mono dim">{{ state.tg_tps === null ? '—' : state.tg_tps.toFixed(2) }}</span>
            <span class="s-unit">t/s</span>
          </div>
        </div>
        <div class="progress-wrap" v-if="genProgress !== null">
          <div class="bar"><i :style="{ width: genProgress * 100 + '%' }"></i></div>
          <span class="mono small dim">{{ (genProgress * 100).toFixed(0) }}%</span>
        </div>
        <div class="kv" v-if="genEta !== null">
          <span class="k">{{ t('llamaState.eta_completion') }}</span><span class="v mono" style="color: var(--cyan)">{{ fmtDuration(genEta) }}</span>
        </div>
        <div class="kv-grid">
          <div class="kv" v-if="ctx.total">
            <span class="k">{{ t('llamaState.context_usage') }}</span>
            <span class="v mono">{{ fmtNum(ctx.used) }} / {{ fmtNum(ctx.total) }} <span class="dim">({{ ctx.pct }}%)</span></span>
          </div>
          <div class="kv" v-if="mtp.acceptance !== null && mtp.acceptance !== undefined">
            <span class="k">{{ t('llamaState.mtp_acceptance_rate') }}</span>
            <span class="v mono" :class="mtpClass">{{ (mtp.acceptance * 100).toFixed(1) }}%<span v-if="mtp.accepted !== null && mtp.accepted !== undefined" class="dim"> · {{ mtp.accepted }}/{{ mtp.generated }}</span></span>
          </div>
          <div class="kv">
            <span class="k">{{ t('llamaState.remaining_tokens') }}</span><span class="v mono">{{ nRemainText }}</span>
          </div>
          <div class="kv" v-if="promptTotal">
            <span class="k">{{ t('llamaState.prompt_total') }}</span><span class="v mono">{{ fmtNum(promptTotal) }} tokens</span>
          </div>
          <div class="kv" v-if="cacheHit !== null">
            <span class="k">{{ t('llamaState.cache_hit') }}</span><span class="v mono" style="color: var(--green)">{{ (cacheHit * 100).toFixed(1) }}%</span>
          </div>
          <div class="kv" v-if="graphsReused !== null">
            <span class="k">{{ t('llamaState.graphs_reused') }}</span><span class="v mono">{{ graphsReused }}</span>
          </div>
        </div>
      </template>

      <template v-else>
        <div class="idle-note">{{ t('llamaState.idle') }}</div>
        <div class="kv" v-if="ctx.total">
          <span class="k">{{ t('llamaState.context_usage') }}</span>
          <span class="v mono">{{ fmtNum(ctx.used) }} / {{ fmtNum(ctx.total) }} <span class="dim">({{ ctx.pct }}%)</span></span>
        </div>
        <div v-if="lastTask" class="last-task small dim">
          {{ t('llamaState.last_task') }} #{{ lastTask.task_id }}：{{ lastTask.total_tokens || lastTask.decoded_tokens || '—' }} tokens
          <template v-if="lastTask.gen_speed_tps"> · {{ t('llamaState.avg_speed') }} {{ lastTask.gen_speed_tps.toFixed(1) }} t/s</template>
          <template v-if="lastTask.mtp && lastTask.mtp.acceptance !== null"> · MTP {{ (lastTask.mtp.acceptance * 100).toFixed(1) }}%</template>
        </div>
        <div class="kv-grid" v-if="lastTask">
          <div class="kv" v-if="lastTask.total_ms">
            <span class="k">{{ t('llamaState.total_time') }}</span><span class="v mono">{{ fmtDuration(lastTask.total_ms / 1000) }}</span>
          </div>
          <div class="kv" v-if="lastTask.prompt_ms">
            <span class="k">{{ t('llamaState.prefill_time') }}</span>
            <span class="v mono">{{ fmtDuration(lastTask.prompt_ms / 1000) }}<template v-if="lastTask.prompt_speed_tps"> @ {{ lastTask.prompt_speed_tps.toFixed(0) }} t/s</template></span>
          </div>
          <div class="kv" v-if="lastTask.eval_ms">
            <span class="k">{{ t('llamaState.gen_time') }}</span><span class="v mono">{{ fmtDuration(lastTask.eval_ms / 1000) }}</span>
          </div>
          <div class="kv" v-if="lastTask.graphs_reused !== null && lastTask.graphs_reused !== undefined">
            <span class="k">{{ t('llamaState.graphs_reused') }}</span><span class="v mono">{{ lastTask.graphs_reused }}</span>
          </div>
        </div>
      </template>

      <div class="cfg-strip" v-if="cfgRows.length">
        <span v-for="r in cfgRows" :key="r[0]" class="cfg">
          <span class="cfg-k">{{ r[0] }}</span><span class="cfg-v mono">{{ r[1] }}</span>
        </span>
      </div>
    </template>

    <div v-else class="placeholder"><span class="icon">⌁</span>{{ t('llamaState.log_unavailable_api') }}</div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { fmtNum, fmtDuration } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  log: { type: Object, default: () => ({}) },
  online: { type: Boolean, default: false },
  slots: { type: Array, default: () => [] },
  flags: { type: Object, default: () => ({}) },
  now: { type: Number, default: 0 }
})

const logAvailable = computed(() => !!(props.log && props.log.available))
const state = computed(() => (props.log && props.log.state) || {})
const lastTask = computed(() => (props.log && props.log.last_task) || null)
const ctx = computed(() => (props.log && props.log.context) || {})
const activeSlot = computed(() => {
  const list = props.slots || []
  const tid = state.value.task_id
  return list.find((s) => s.is_processing && (tid === null || tid === undefined || s.id_task === tid))
    || list.find((s) => s.is_processing)
    || null
})
const promptTotal = computed(() => (activeSlot.value && activeSlot.value.n_prompt_tokens) || null)
const promptProcessed = computed(() => {
  const s = activeSlot.value
  const n = s && s.n_prompt_tokens_processed
  return n === null || n === undefined ? null : n
})
const cacheHit = computed(() => {
  const s = activeSlot.value
  if (!s || !s.n_prompt_tokens || s.n_prompt_tokens_cache === null || s.n_prompt_tokens_cache === undefined) return null
  return s.n_prompt_tokens_cache / s.n_prompt_tokens
})
// MTP 接受率（task_end 行更新，与总览仪表同源；<65 红 / 65-80 黄 / ≥80 绿）
const mtp = computed(() => (props.log && props.log.mtp) || {})
const mtpClass = computed(() => {
  const a = mtp.value.acceptance
  if (a === null || a === undefined) return ''
  if (a < 0.65) return 'lv-danger'
  if (a < 0.8) return 'lv-warn'
  return 'lv-green'
})
const graphsReused = computed(() => {
  const g = props.log && props.log.graphs_reused
  return g === null || g === undefined ? null : g
})
const slotInfo = computed(() => {
  const list = props.slots || []
  if (!list.length) return null
  const a = activeSlot.value
  const id = a && a.id !== null && a.id !== undefined ? a.id : '?'
  return `${id}/${list.length}`
})
const prefillEta = computed(() => {
  const st = state.value
  const p = st.prompt_progress
  const total = st.prompt_total_tokens
  const speed = st.prompt_speed_tps
  if (p === null || p === undefined || !total || !speed || speed <= 0) return null
  return Math.max(0, (1 - p) * total / speed)
})

const phase = computed(() => state.value.phase || 'idle')
const phaseText = computed(() => {
  if (!props.online) return t('llamaState.offline')
  if (!logAvailable.value) return t('llamaState.unknown')
  if (phase.value === 'prompt_processing') return t('llamaState.prompt_processing')
  if (phase.value === 'decoding') return t('llamaState.generating')
  return t('llamaState.idle')
})
const phaseClass = computed(() => {
  if (!props.online) return 'lv-danger'
  if (phase.value === 'decoding') return 'lv-cyan'
  if (phase.value === 'prompt_processing') return 'lv-green'
  return ''
})
const cardClass = computed(() => (phase.value === 'decoding' && props.online ? 'active' : ''))

// 解码进度 / ETA（n_remain > 0 表示设了上限；-1/0 为不限）
const nRemain = computed(() => {
  const s = activeSlot.value
  const n = s && s.n_remain
  return typeof n === 'number' && n > 0 ? n : null
})
const nRemainText = computed(() => (nRemain.value === null ? t('llamaState.unlimited') : fmtNum(nRemain.value)))
const genProgress = computed(() => {
  if (nRemain.value === null) return null
  const d = state.value.n_decoded || 0
  const total = d + nRemain.value
  return total > 0 ? Math.min(1, d / total) : null
})
const genEta = computed(() => {
  if (nRemain.value === null) return null
  const v = state.value.tg_3s_tps
  if (!v || v <= 0) return null
  return nRemain.value / v
})

// 任务已运行时长（随快照 ts 每秒跳动）
const elapsed = computed(() => {
  const st = state.value
  if (phase.value === 'idle') return null
  const t0 = st.started_at
  if (!t0 || !props.now) return null
  const e = props.now - t0
  return e >= 0 ? e : null
})
const elapsedText = computed(() => (elapsed.value === null ? '' : fmtDuration(elapsed.value)))

// 静态配置（llama-server 命令行解析）
const cfgRows = computed(() => {
  const f = props.flags || {}
  const out = []
  if (f.spec_type) out.push([t('llamaState.spec_decoding'), `${f.spec_type}×${f.spec_draft_n_max ?? '?'}`])
  if (f.cache_type_k || f.cache_type_v) out.push([t('llamaState.kv_cache'), `${f.cache_type_k || '—'} / ${f.cache_type_v || '—'}`])
  if (f.batch) out.push([t('llamaState.batch_size'), `${f.batch}${f.ubatch ? ' / ' + f.ubatch : ''}`])
  if (f.n_gpu_layers) out.push([t('llamaState.gpu_layers'), f.n_gpu_layers])
  return out
})
</script>

<style scoped>
.state-card { padding: 12px 16px; display: flex; flex-direction: column; gap: 4px; overflow-y: auto; }
.state-card.active { border-color: rgba(0, 229, 255, 0.35); box-shadow: 0 0 12px rgba(0, 229, 255, 0.15); }
.head { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
.phase { font-size: 16px; font-weight: 700; }
.lv-cyan { color: var(--cyan); text-shadow: 0 0 10px rgba(0, 229, 255, 0.5); }
.lv-green { color: var(--green); text-shadow: 0 0 10px rgba(0, 255, 157, 0.5); }
.elapsed { margin-left: auto; color: var(--cyan); }
.progress-wrap { display: flex; align-items: center; gap: 8px; padding: 4px 0; }
.progress-wrap .bar { flex: 1; }
.stat3 { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; padding: 4px 0; }
.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  padding: 8px 10px;
  background: rgba(143, 163, 200, 0.06);
  border: 1px solid rgba(143, 163, 200, 0.12);
  border-radius: 8px;
}
.s-label { font-size: 10px; color: var(--text-faint); letter-spacing: 1px; }
.s-val { font-size: 18px; font-weight: 700; line-height: 1.15; }
.s-val.cyan { color: var(--cyan); text-shadow: 0 0 10px rgba(0, 229, 255, 0.4); }
.s-val.green { color: var(--green); text-shadow: 0 0 10px rgba(0, 255, 157, 0.4); }
.s-val.dim { color: var(--text-dim); }
.s-unit { font-size: 10px; color: var(--text-faint); }
.kv-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 18px; }
.idle-note { color: var(--text-faint); font-size: 12px; padding: 6px 0; }
.last-task { line-height: 1.6; }
.cfg-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(143, 163, 200, 0.12);
}
.cfg { display: inline-flex; gap: 6px; font-size: 10px; }
.cfg-k { color: var(--text-faint); }
.cfg-v { color: var(--text-dim); }
</style>
