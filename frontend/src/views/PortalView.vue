<template>
  <div class="portal">
    <BrandBar :hosts="hosts" :connected="connected" />
    <main class="main">
      <StatsPanel :hosts="hosts" />
      <div class="grid">
        <template v-if="hosts.length">
          <HostCard v-for="h in hosts" :key="h.id" :host="h" />
        </template>
        <div v-else class="glass placeholder" style="grid-column: 1 / -1; min-height: 300px">
          <span class="icon">◉</span>
          <span>{{ t('portal.no_hosts') }}</span>
          <span class="small">{{ t('portal.add_hosts') }}</span>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import BrandBar from '../components/BrandBar.vue'
import HostCard from '../components/HostCard.vue'
import StatsPanel from '../components/StatsPanel.vue'
import { usePortalStream } from '../stream'
import { t } from '../i18n'

const { hosts, connected } = usePortalStream()
</script>

<style scoped>
.main { display: flex; flex-direction: column; }
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(360px, 100%), 1fr));
  gap: 16px;
  padding: 20px 24px;
}
</style>
