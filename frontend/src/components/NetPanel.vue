<template>
  <div class="panel glass">
    <div class="panel-head">
      <span class="panel-title">{{ t('netPanel.title') }}</span>
      <span class="mono dim small">{{ ifaces.length }} {{ t('netPanel.nics') }}</span>
    </div>
    <table class="tbl mono">
      <thead>
        <tr><th>{{ t('netPanel.interface') }}</th><th class="num">{{ t('netPanel.rx') }}</th><th class="num">{{ t('netPanel.tx') }}</th><th class="num">{{ t('netPanel.cumulative_down') }}</th><th class="num">{{ t('netPanel.cumulative_up') }}</th></tr>
      </thead>
      <tbody>
        <tr v-for="n in ifaces" :key="n.name">
          <td>{{ n.name }}</td>
          <td class="num" style="color: var(--cyan)">{{ n.rx_mb_s.toFixed(2) }} MB/s</td>
          <td class="num" style="color: var(--green)">{{ n.tx_mb_s.toFixed(2) }} MB/s</td>
          <td class="num dim">{{ fmtBytes(n.rx_total_mb * 1024 * 1024) }}</td>
          <td class="num dim">{{ fmtBytes(n.tx_total_mb * 1024 * 1024) }}</td>
        </tr>
        <tr v-if="!ifaces.length"><td colspan="5" class="faint">{{ t('netPanel.no_data') }}</td></tr>
      </tbody>
    </table>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { fmtBytes } from '../utils'
import { t } from '../i18n'

const props = defineProps({
  net: { type: Object, default: () => ({}) }
})
const ifaces = computed(() => props.net.ifaces || [])
</script>

<style scoped>
.panel { padding: 12px 16px; }
.panel-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 8px; }
.panel-title { font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }
</style>
