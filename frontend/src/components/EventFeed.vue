<template>
  <div class="feed glass" :class="{ fill }">
    <div class="feed-head">
      <span class="section-title" style="margin: 0">{{ t('eventFeed.title') }}</span>
      <span class="mono faint small">{{ events.length }} / 200</span>
    </div>
    <div ref="box" class="feed-box mono">
      <div
        v-for="(e, i) in events"
        :key="e.ts + '-' + i"
        class="feed-line"
        :class="{ 'event-new': i < 3 && events.length > 1, fade: i > 160 }"
      >
        <span class="ts">{{ fmtTimeShort(e.ts) }}</span>
        <span class="lv" :class="'lv-' + e.level">[{{ e.level.toUpperCase() }}]</span>
        <span class="msg">{{ formatEvent(e) }}</span>
      </div>
      <div v-if="!events.length" class="placeholder"><span class="icon">⌁</span>{{ t('eventFeed.no_events') }}</div>
    </div>
    <button v-if="hasNew && !atBottom" class="new-btn" @click="scrollToBottom">↓ {{ t('eventFeed.new_events') }}</button>
  </div>
</template>

<script setup>
import { ref, watch, nextTick, computed } from 'vue'
import { fmtTimeShort } from '../utils'
import { t, formatEvent } from '../i18n'

const props = defineProps({
  events: { type: Array, default: () => [] },
  fill: { type: Boolean, default: false } // 铺满父容器高度（任务区右卡）
})

const box = ref(null)
const atBottom = ref(true)
const hasNew = ref(false)

function isNearBottom() {
  const b = box.value
  if (!b) return true
  return b.scrollHeight - b.scrollTop - b.clientHeight < 40
}

function onScroll() {
  atBottom.value = isNearBottom()
  if (atBottom.value) hasNew.value = false
}

function scrollToBottom() {
  if (box.value) box.value.scrollTop = box.value.scrollHeight
  hasNew.value = false
  atBottom.value = true
}

watch(
  () => props.events.length,
  async (len, old) => {
    if (len > (old || 0)) {
      if (atBottom.value) {
        await nextTick()
        scrollToBottom()
      } else {
        hasNew.value = true
      }
    }
  }
)
</script>

<style scoped>
.feed { position: relative; display: flex; flex-direction: column; }
.feed.fill .feed-box { flex: 1; height: auto; min-height: 0; }
.feed-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px 8px;
  border-bottom: 1px solid rgba(143, 163, 200, 0.1);
}
.feed-box {
  height: 260px;
  overflow-y: auto;
  padding: 8px 14px;
  font-size: 12px;
  line-height: 1.7;
  background: rgba(5, 8, 14, 0.5);
  border-radius: 0 0 10px 10px;
}
.feed-line { display: flex; gap: 8px; white-space: nowrap; }
.feed-line.fade { opacity: 0.45; }
.ts { color: var(--text-faint); flex: none; }
.lv { flex: none; width: 62px; }
.lv.lv-info { color: var(--green); }
.lv.lv-warn { color: var(--amber); }
.lv.lv-error { color: var(--red); }
.msg { color: var(--text); overflow: hidden; text-overflow: ellipsis; }
.new-btn {
  position: absolute;
  bottom: 10px;
  right: 14px;
  background: rgba(0, 229, 255, 0.15);
  color: var(--cyan);
  border: 1px solid rgba(0, 229, 255, 0.4);
  border-radius: 14px;
  font-size: 11px;
  padding: 3px 12px;
  cursor: pointer;
}
.new-btn:hover { background: rgba(0, 229, 255, 0.25); }
</style>
