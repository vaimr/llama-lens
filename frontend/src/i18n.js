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
