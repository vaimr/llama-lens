<template>
  <TerminalFrame>
    <router-view />
  </TerminalFrame>
</template>

<script setup>
import { watch } from 'vue'
import TerminalFrame from './components/TerminalFrame.vue'
import { totalSpeed } from './speed'
import { t } from './i18n'

// 动态标题：有生成速度时，在浏览器标签页标题实时展示总 token 速度
// （全局监听，门户/详情页均生效；数据源见 BrandBar → speed.js）
const BASE_TITLE = t('app.title')
watch(totalSpeed, (speed) => {
  document.title = speed > 0 ? `⚡ ${speed.toFixed(1)} tok/s · ${BASE_TITLE}` : BASE_TITLE
}, { immediate: true })
</script>
