<script setup>
import { computed, ref, watch } from 'vue'
import { NButton, NScrollbar } from 'naive-ui'
import { CopyOutline } from '@vicons/ionicons5'
import { NIcon } from 'naive-ui'

const props = defineProps({
  lines: { type: Array, default: () => [] },
})

const scroller = ref(null)
const text = computed(() => (props.lines.length ? props.lines.join('\n') : '等待任务开始…'))

watch(
  () => props.lines.length,
  () => {
    requestAnimationFrame(() => {
      const el = scroller.value?.$el?.querySelector('.n-scrollbar-container')
      if (el) el.scrollTop = el.scrollHeight
    })
  }
)

async function copyLog() {
  await navigator.clipboard.writeText(text.value)
}
</script>

<template>
  <div class="terminal glass">
    <div class="term-head">
      <span>实时日志</span>
      <NButton size="tiny" quaternary @click="copyLog">
        <template #icon><NIcon :component="CopyOutline" /></template>
        复制
      </NButton>
    </div>
    <NScrollbar ref="scroller" style="max-height: 420px">
      <pre class="log-body">{{ text }}</pre>
    </NScrollbar>
  </div>
</template>

<style scoped>
.terminal {
  padding: 1rem 1.25rem;
  height: 100%;
}
.term-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--muted);
}
.log-body {
  margin: 0;
  padding: 1rem;
  background: rgba(0, 0, 0, 0.45);
  border-radius: 10px;
  font-family: var(--mono);
  font-size: 0.78rem;
  line-height: 1.55;
  color: #b8c5d6;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
