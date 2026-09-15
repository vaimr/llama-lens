import { ref } from 'vue'
import en from './locales/en'
import ru from './locales/ru'
import zh from './locales/zh'

export const SUPPORTED = ['en', 'ru', 'zh']
export const LOCALES = { en, ru, zh }
const STORAGE_KEY = 'llamalens-lang'

function initialLocale() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (SUPPORTED.includes(stored)) return stored
  } catch (e) { /* privacy mode */ }
  try {
    const browser = navigator.language
    if (browser.startsWith('zh')) return 'zh'
    if (browser.startsWith('ru')) return 'ru'
  } catch (e) { /* privacy mode */ }
  return 'en'
}

export const locale = ref(initialLocale())

export function setLocale(l) {
  if (!SUPPORTED.includes(l)) return
  locale.value = l
  try {
    localStorage.setItem(STORAGE_KEY, l)
  } catch (e) { /* privacy mode */ }
}

/**
 * Dot-path translation lookup with interpolation.
 * - Reads `locale.value` on every call so Vue tracks the reactive dependency.
 * - Falls back to the EN dict when the key is missing in the active locale.
 * - Returns the raw key string when missing everywhere.
 */
export function t(key, params) {
  const current = LOCALES[locale.value] || en
  let value = current
  const parts = key.split('.')
  for (const part of parts) {
    if (value == null || typeof value !== 'object') { value = undefined; break }
    value = value[part]
  }
  if (value == null || typeof value !== 'string') {
    // Fall back to EN
    let fallback = en
    for (const part of parts) {
      if (fallback == null || typeof fallback !== 'object') { fallback = undefined; break }
      fallback = fallback[part]
    }
    if (fallback != null && typeof fallback === 'string') return interpolate(fallback, params)
    return key
  }
  return interpolate(value, params)
}

function interpolate(str, params) {
  if (!params) return str
  return str.replace(/\{(\w+)\}/g, (_, name) => {
    if (params[name] != null) return String(params[name])
    return `{${name}}`
  })
}

// ---------------------------------------------------------------------------
// Event message formatting (translates backend event type → localized message)
// ---------------------------------------------------------------------------

/**
 * Format a backend event into a localized display string.
 * @param {object} event - {type, data: {id, n_tokens, duration, avg_tps, mtp_acceptance, ctx_used, ...}}
 * @returns {string} Localized message
 */
export function formatEvent(event) {
  if (!event || !event.type) return ''
  const d = event.data || {}
  const type = event.type

  switch (type) {
    case 'llama_up': {
      const model = d.model || ''
      return model ? t('eventFeed.ev_llama_up_model', { model }) : t('eventFeed.ev_llama_up')
    }
    case 'llama_recovery':
    case 'llama_recovered': {
      const model = d.model || ''
      return model ? t('eventFeed.ev_llama_recovery', { model }) : t('eventFeed.ev_llama_recovery')
    }
    case 'llama_down':
      return t('eventFeed.ev_llama_down')
    case 'ssh_up':
      return t('eventFeed.ev_ssh_up')
    case 'ssh_down':
      return t('eventFeed.ev_ssh_down')
    case 'task_start': {
      const id = d.task_id != null ? d.task_id : '?'
      if (d.prompt_tokens != null && d.prompt_tokens > 0) {
        return t('eventFeed.ev_task_start_prompt', { id, n: d.prompt_tokens })
      }
      return t('eventFeed.ev_task_start', { id })
    }
    case 'task_end': {
      const id = d.task_id != null ? d.task_id : '?'
      if (d.total_tokens != null || d.duration_s != null || d.avg_tps != null) {
        const parts = []
        if (d.total_tokens != null) parts.push(`${d.total_tokens} tokens`)
        if (d.duration_s != null) parts.push(fmtDuration(d.duration_s))
        if (d.avg_tps != null) parts.push(`avg ${d.avg_tps.toFixed(1)} tok/s`)
        if (d.mtp_acceptance != null) parts.push(`MTP ${(d.mtp_acceptance * 100).toFixed(1)}%`)
        if (d.ctx_used != null) parts.push(`ctx ${d.ctx_used}`)
        return t('eventFeed.ev_task_end_stats', { id, stats: parts.join(' · ') })
      }
      return t('eventFeed.ev_task_end', { id })
    }
    case 'model_change': {
      const from = d.from || ''
      const to = d.to || ''
      return t('eventFeed.ev_model_change', { from, to })
    }
    case 'llama_boot': {
      const info = d.info || ''
      return t('eventFeed.ev_boot', { info })
    }
    case 'alert': {
      if (d.recovered) {
        return t('eventFeed.ev_alert_recovery', { name: d.metric || '' })
      }
      const tag = d.level === 'danger' ? t('gpuPanel.normal') : 'warning'
      const name = d.metric || ''
      return t('eventFeed.ev_alert_warn', { name, op: d.op || '≥', threshold: d.threshold ?? '', tag })
    }
    default:
      // For any unhandled event types, return a generic message
      return `${type}${d.note ? `: ${d.note}` : ''}`
  }
}

/**
 * Simple duration formatter (copied from utils to avoid circular deps)
 */
function fmtDuration(seconds) {
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  if (m < 60) return `${m}m${s.toString().padStart(2, '0')}s`
  const h = Math.floor(m / 60)
  const rm = m % 60
  return `${h}h${rm.toString().padStart(2, '0')}m`
}
