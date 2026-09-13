<template>
  <div class="barcard glass" :class="levelClass">
    <div class="barcard-head">
      <span class="barcard-title">{{ title }}</span>
      <span v-if="badge" class="badge danger small">{{ badge }}</span>
      <span v-if="sub" class="barcard-sub mono">{{ sub }}</span>
    </div>
    <div class="barcard-value">
      <span class="mono num" :class="levelClass">{{ text }}</span>
      <span v-if="unit" class="unit">{{ unit }}</span>
    </div>
    <div class="bar"><i :class="barClass" :style="{ width: barPct + '%', '--pct': barPct }"></i></div>
    <div class="barcard-scale mono">
      <span>0</span>
      <span>{{ midText }}</span>
      <span>{{ maxText }}</span>
    </div>
    <svg v-if="sparkD" class="barcard-spark" :viewBox="`0 0 ${sparkW} ${sparkH}`" preserveAspectRatio="none">
      <path :d="sparkD" fill="none" :stroke="sparkColor" stroke-width="1.2" vector-effect="non-scaling-stroke" />
    </svg>
    <div v-if="footText" class="barcard-foot mono">{{ footText }}</div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { sparkPath, useCountUp, fmtNum, niceMax } from '../utils'
import { chartTheme } from '../theme'
import { t } from '../i18n'

const props = defineProps({
  title: { type: String, required: true },
  value: { type: Number, default: null },
  unit: { type: String, default: '' },
  digits: { type: Number, default: 1 },
  level: { type: String, default: 'normal' }, // normal | warn | danger
  barMax: { type: Number, default: null }, // null = 动态（60s 峰值取整）
  progress: { type: Number, default: null }, // 0-1；设置后条位显示进度（预填充），刻度 0/50/100%
  spark: { type: Array, default: () => [] }, // [[ts, value], ...] 60s
  sub: { type: String, default: '' },
  badge: { type: String, default: '' },
  foot: { type: String, default: '' }, // 底行前缀；后接 60s 峰/谷/均
  fmt: { type: Function, default: null }, // 大数字格式化（缺省 toFixed(digits)）
  fmtCompact: { type: Function, default: null } // 刻度/峰谷均格式化（缺省整数）
})

const animated = useCountUp(computed(() => (typeof props.value === 'number' ? props.value : 0)))
const text = computed(() => {
  if (props.value === null || props.value === undefined) return '—'
  const n = Number(animated.value)
  if (!Number.isFinite(n)) return '—'
  return props.fmt ? props.fmt(n) : n.toFixed(props.digits)
})

const levelClass = computed(() => (props.level === 'danger' ? 'lv-danger' : props.level === 'warn' ? 'lv-warn' : ''))
const hasProgress = computed(() => props.progress !== null && props.progress !== undefined)
const barClass = computed(() => {
  if (hasProgress.value) return 'green'
  return props.level === 'danger' ? 'danger' : props.level === 'warn' ? 'warn' : ''
})

const barMaxVal = computed(() => {
  if (props.barMax) return props.barMax
  const vals = (props.spark || []).map((p) => p[1]).filter((v) => v !== null && v !== undefined)
  if (typeof props.value === 'number') vals.push(props.value)
  if (!vals.length) return 10
  return niceMax(Math.max(...vals) * 1.1)
})
const barPct = computed(() => {
  if (hasProgress.value) return Math.min(100, Math.max(0, props.progress * 100))
  if (props.value === null || props.value === undefined) return 0
  return Math.min(100, (props.value / barMaxVal.value) * 100)
})
const scaleDigits = computed(() => (barMaxVal.value < 10 ? 1 : 0))
const midText = computed(() => {
  if (hasProgress.value) return '50'
  return props.fmtCompact ? props.fmtCompact(barMaxVal.value / 2) : fmtNum(barMaxVal.value / 2, scaleDigits.value)
})
const maxText = computed(() => {
  if (hasProgress.value) return '100%'
  return props.fmtCompact ? props.fmtCompact(barMaxVal.value) : fmtNum(barMaxVal.value, scaleDigits.value)
})

const sparkW = 150
const sparkH = 20
const sparkD = computed(() => sparkPath(props.spark || [], sparkW, sparkH))
const sparkColor = computed(() => {
  const t = chartTheme()
  if (props.value === null || props.value === undefined) return t.faint
  if (props.level === 'danger') return t.red
  if (props.level === 'warn') return t.amber
  return t.cyan
})

const statsText = computed(() => {
  const vals = (props.spark || []).map((p) => p[1]).filter((v) => v !== null && v !== undefined)
  if (!vals.length) return ''
  const max = Math.max(...vals)
  const min = Math.min(...vals)
  const avg = vals.reduce((a, b) => a + b, 0) / vals.length
  if (max === 0) return '' // 全零窗口（如解码期 prompt）无统计意义
  const f = props.fmtCompact || ((v) => v.toFixed(0))
  return `${t('barCard.60s_stats')} ${f(max)} · ${t('barCard.peak')} ${f(min)} · ${t('barCard.avg')} ${f(avg)}`
})
const footText = computed(() => {
  const prefix = props.foot ? props.foot + ' · ' : ''
  return prefix + statsText.value
})
</script>

<style scoped>
.barcard { padding: 12px 16px; display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.barcard-head { display: flex; align-items: center; gap: 8px; }
.barcard-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; flex: none; }
.barcard-sub {
  font-size: 10px;
  color: var(--text-faint);
  margin-left: auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.barcard-value { display: flex; align-items: baseline; gap: 6px; }
.num { font-size: 24px; font-weight: 700; color: var(--text); line-height: 1.1; }
.lv-danger .num { text-shadow: 0 0 12px rgba(255, 59, 92, 0.5); }
.lv-warn .num { text-shadow: 0 0 12px rgba(255, 197, 61, 0.4); }
.unit { color: var(--text-dim); font-size: 11px; }
.barcard-scale {
  display: flex;
  justify-content: space-between;
  font-size: 9px;
  color: var(--text-faint);
  margin-top: -4px;
}
.barcard-spark { width: 100%; height: 20px; margin-top: auto; }
.barcard-foot {
  font-size: 10px;
  color: var(--text-faint);
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
