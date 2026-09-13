<template>
  <select :value="locale" @change="onChange">
    <option value="en">EN</option>
    <option value="ru">РУ</option>
  </select>
</template>

<script setup>
import { locale, setLocale } from '../i18n'

function onChange(e) {
  setLocale(e.target.value)
}
</script>

<style scoped>
select {
  background: rgba(16, 24, 40, 0.9);
  color: var(--text);
  border: 1px solid var(--card-border);
  border-radius: 6px;
  font-size: 12px;
  padding: 4px 8px;
  outline: none;
  cursor: pointer;
}
select:hover { border-color: var(--card-border-hover); }
</style>
