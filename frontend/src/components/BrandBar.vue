<template>
  <header class="brandbar">
    <div class="brand">
      <span class="logo">◉</span>
      <span class="name">{{ t('brand.name') }}</span>
      <span class="sub">{{ t('brand.sub') }}</span>
    </div>
    <div class="stats">
      <span class="stat">
        <span class="k">{{ t('brandBar.hosts') }}</span><span class="v mono">{{ hostsTotal }}</span>
      </span>
      <span class="stat">
        <span class="k">{{ t('brandBar.online') }}</span><span class="v mono lv-green">{{ onlineCount }}</span>
      </span>
      <span v-if="totalSpeed > 0" class="stat">
        <span class="k">{{ t('brandBar.speed') }}</span><span class="v mono lv-cyan">{{ totalSpeed.toFixed(1) }}</span><span class="unit">t/s</span>
      </span>
      <span v-if="showPrefill" class="stat">
        <span class="k">{{ t('brandBar.prefill') }}</span><span class="v mono lv-amber">{{ totalPrefill.toFixed(1) }}</span><span class="unit">t/s</span>
      </span>
      <span class="conn" :class="connected ? 'ok' : 'bad'">{{ connected ? t('brandBar.ws_realtime') : t('brandBar.polling') }}</span>
      <LanguageSwitcher />
      <ThemeSwitcher />
      <span class="clock-sep"></span>
      <LiveClock />
    </div>
  </header>
</template>

<script setup>
import { computed, watch, ref } from 'vue'
import LiveClock from './LiveClock.vue'
import LanguageSwitcher from './LanguageSwitcher.vue'
import ThemeSwitcher from './ThemeSwitcher.vue'
import { totalSpeed as globalSpeed } from '../speed'
import { t } from '../i18n'

const props = defineProps({
  hosts: { type: Array, default: () => [] },
  connected: { type: Boolean, default: false }
})

const hostsTotal = computed(() => props.hosts.length)
const onlineCount = computed(() => props.hosts.filter((h) => h.online).length)
const totalSpeed = computed(() => props.hosts.reduce((s, h) => s + (h.gen_speed_tps || 0), 0))

// --- Prefill last-non-zero hold ---
const prefillMap = ref(new Map())

watch(() => props.hosts, (hosts) => {
  const map = new Map(prefillMap.value)
  const ids = new Set()
  for (const h of hosts) {
    ids.add(h.id)
    const ps = h.prompt_speed_tps
    if (ps !== null && ps !== undefined && ps > 0) {
      map.set(h.id, ps)
    } else if (!h.online) {
      map.delete(h.id)
    }
  }
  // hosts vanished from the payload entirely
  for (const k of map.keys()) if (!ids.has(k)) map.delete(k)
  prefillMap.value = map
}, { immediate: true })

const totalPrefill = computed(() => {
  let sum = 0
  for (const v of prefillMap.value.values()) {
    sum += v
  }
  return sum
})
const showPrefill = computed(() => prefillMap.value.size > 0 && totalPrefill.value > 0)

// 门户级聚合速度同步到全局：浏览器标签页标题（App.vue）与
// Terminal 窗口标题栏（TerminalFrame.vue）据此展示，任意视图均可见
watch(totalSpeed, (v) => { globalSpeed.value = v }, { immediate: true })
</script>

<style scoped>
.brandbar {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  border-bottom: 1px solid rgba(0, 229, 255, 0.12);
  background: rgba(10, 14, 23, 0.6);
  backdrop-filter: blur(12px);
  position: sticky;
  top: var(--chrome-top, 0px);
  z-index: 20;
}
.brand { display: flex; align-items: baseline; gap: 10px; }
.logo {
  color: var(--cyan);
  font-size: 20px;
  text-shadow: 0 0 12px rgba(0, 229, 255, 0.8);
  align-self: center;
}
.name {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 1px;
  background: linear-gradient(90deg, #00e5ff, #00ff9d);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.sub { color: var(--text-faint); font-size: 11px; }
.stats { display: flex; align-items: center; gap: 6px; }
.stat {
  display: inline-flex;
  align-items: baseline;
  gap: 5px;
  padding: 3px 9px;
  border: 1px solid var(--card-border);
  border-radius: 6px;
  background: rgba(16, 24, 40, 0.55);
  white-space: nowrap;
}
.stat .k { font-size: 10px; color: var(--text-dim); font-weight: 500; }
.stat .v { font-size: 12px; color: var(--text); font-weight: 600; }
.stat .unit { font-size: 10px; color: var(--text-faint); }
.lv-green { color: var(--green) !important; }
.lv-cyan { color: var(--cyan) !important; }
.lv-amber { color: var(--amber) !important; }
.conn { padding: 2px 8px; border-radius: 10px; font-size: 11px; }
.conn.ok { color: var(--green); border: 1px solid rgba(0, 255, 157, 0.35); }
.conn.bad { color: var(--amber); border: 1px solid rgba(255, 197, 61, 0.35); }
.clock-sep { width: 1px; height: 16px; background: rgba(143, 163, 200, 0.25); }
</style>
