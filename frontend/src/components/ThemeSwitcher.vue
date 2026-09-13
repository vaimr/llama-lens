<template>
  <label class="theme-switch">
    <span class="ts-label">{{ t('themeSwitcher.label') }}</span>
    <select :value="themeState.id" @change="onChange">
      <option v-for="t in THEMES" :key="t.id" :value="t.id">{{ t.label }}</option>
    </select>
  </label>
</template>

<script setup>
import { THEMES, themeState, setTheme } from '../theme'
import { t } from '../i18n'

function onChange(e) {
  setTheme(e.target.value)
}
</script>

<style scoped>
.theme-switch { display: inline-flex; align-items: center; gap: 6px; flex: none; }
.ts-label { font-size: 11px; color: var(--text-faint); }
.theme-switch select {
  background: rgba(16, 24, 40, 0.9);
  color: var(--text);
  border: 1px solid var(--card-border);
  border-radius: 6px;
  font-size: 12px;
  padding: 4px 8px;
  outline: none;
  cursor: pointer;
}
.theme-switch select:hover { border-color: var(--card-border-hover); }
</style>
