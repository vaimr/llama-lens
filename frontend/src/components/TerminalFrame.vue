<template>
  <div class="tframe">
    <template v-if="term">
      <!-- 终端窗口标题栏（macOS 风格：标题显示运行中的命令，无多余 meta） -->
      <div class="tbar">
        <div class="tlights">
          <span class="tl red"><em>×</em></span>
          <span class="tl yellow"><em>−</em></span>
          <span class="tl green"><em>+</em></span>
        </div>
        <div class="ttitle">
          <span>{{ titleBase }}</span><span v-if="totalSpeed > 0" class="tspeed"> · ⚡ {{ totalSpeed.toFixed(1) }} tok/s</span>
        </div>
      </div>
      <!-- 会话首行：提示符 + 命令（命令已执行，光标不在此处） -->
      <div class="tprompt">
        <span class="pu">user@llamalens</span><span class="pd">:~$</span><span class="pc">{{ cmd }}</span>
      </div>
    </template>

    <slot />

    <!-- 命令执行完毕后的新提示符（仅门户：hosts 命令已完成；watch 长驻命令无新提示符、光标隐藏） -->
    <div v-if="term && isPortal" class="tprompt next">
      <span class="pu">user@llamalens</span><span class="pd">:~$</span><span class="cursor">▊</span>
    </div>

    <!-- tmux 风格底部状态栏（左：窗口列表，右：主机 + 时间） -->
    <div v-if="term" class="tstatus">
      <span class="seg">0: llamalens*</span>
      <span class="seg2">1: logs</span>
      <span class="tstatus-right"><span>user@ai.lan</span><span>{{ clock }}</span></span>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { themeState } from '../theme'
import { totalSpeed } from '../speed'
import { t } from '../i18n'

const route = useRoute()
const term = computed(() => themeState.id === 'terminal')

// 命令随路由变化：门户 = 主机列表，详情 = 单主机 watch
const cmd = computed(() =>
  route.path === '/'
    ? ' llamalens hosts --all'
    : ` llamalens watch --host ${route.params.id || ''} --interval 1s`
)

const isPortal = computed(() => route.path === '/')
// 标题栏：门户回到 user@host:~；详情页显示运行中的 watch 命令（macOS 终端行为）
// 有生成速度时追加 ⚡ X tok/s（ANSI 绿），让“标题”直接可见实时速度
const titleBase = computed(() =>
  isPortal.value
    ? 'user@llamalens: ~'
    : `llamalens watch --host ${route.params.id || ''} --interval 1s`
)

const clock = ref('')
let timer = null
function tick() {
  const d = new Date()
  const p = (n) => String(n).padStart(2, '0')
  clock.value = `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
onMounted(() => {
  tick()
  timer = setInterval(tick, 1000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.tframe { min-height: 100%; }

/* ---------------- 标题栏 ---------------- */
.tbar {
  position: sticky;
  top: 0;
  z-index: 40;
  height: 38px;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  padding: 0 14px;
  background: #18181c;
  border-bottom: 1px solid #000;
  font-size: 12px;
}
.tlights { display: flex; gap: 8px; }
.tl {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  position: relative;
  flex: none;
}
.tl em {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-style: normal;
  font-size: 9px;
  line-height: 1;
  color: rgba(0, 0, 0, 0.55);
  opacity: 0;
}
.tlights:hover em { opacity: 1; }
.tl.red { background: #ff5f57; box-shadow: inset 0 0 0 0.5px #e0443e; }
.tl.yellow { background: #febc2e; box-shadow: inset 0 0 0 0.5px #d89e24; }
.tl.green { background: #28c840; box-shadow: inset 0 0 0 0.5px #1eae31; }
.ttitle { text-align: center; color: #787880; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; padding: 0 12px; }
.ttitle .tspeed { color: #98c379; font-weight: 700; }

/* ---------------- 提示符行 ---------------- */
.tprompt {
  padding: 10px 24px 0;
  font-size: 13px;
  line-height: 1.5;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pu { color: #98c379; font-weight: 700; }
.pd { color: #c8c8cc; }
.pc { color: #c8c8cc; }
.tprompt.next { padding: 2px 24px 10px; }
.cursor {
  color: #98c379;
  margin-left: 2px;
  animation: termCursor 1.1s steps(1) infinite;
}
@keyframes termCursor {
  0%, 55% { opacity: 1; }
  56%, 100% { opacity: 0; }
}

/* ---------------- tmux 状态栏 ---------------- */
.tstatus {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 40;
  height: 24px;
  display: flex;
  align-items: center;
  background: #18181c;
  border-top: 1px solid #000;
  font-size: 11px;
  font-family: ui-monospace, 'SF Mono', 'JetBrains Mono', Menlo, Consolas, monospace;
}
.seg {
  background: #98c379;
  color: #0e0e10;
  padding: 0 10px;
  height: 100%;
  display: flex;
  align-items: center;
  font-weight: 700;
  flex: none;
}
.seg2 { color: var(--text-dim); padding: 0 10px; flex: none; }
.tstatus-right {
  margin-left: auto;
  display: flex;
  gap: 12px;
  color: var(--text-dim);
  padding: 0 12px;
  flex: none;
}
</style>
