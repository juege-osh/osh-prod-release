<script setup>
import { computed } from 'vue'

const props = defineProps({
  health: Object,
  run: Object,
  progress: Number,
  statusLabel: String,
  busy: Boolean,
})

const activeSlot = computed(() => props.health?.deploy?.active_slot || 'green')
const prodHost = computed(() => props.health?.deploy?.prod_host || '149.88.92.159')
const deployRef = computed(() => props.health?.deploy?.backend_ref || 'release/20260708')
const progressLabel = computed(() => `${props.progress || 0}%`)
</script>

<template>
  <section class="command-overview panel">
    <div class="overview-head">
      <div>
        <p class="section-kicker">Production Command</p>
        <h1>OSH 发布作战台</h1>
        <p class="section-copy">
          以绿环境为首发验证面，围绕审批、增量数据、自动化测试、切流、回滚和蓝后同步建立完整发布闭环。
        </p>
      </div>
      <div class="slot-card">
        <span class="slot-label">当前生产槽位</span>
        <strong>{{ activeSlot === 'blue' ? 'Blue' : 'Green' }}</strong>
        <span class="slot-host">{{ prodHost }}</span>
      </div>
    </div>

    <div class="metric-grid overview-metrics">
      <div class="metric-card">
        <div class="metric-label">发布状态</div>
        <div class="metric-value">{{ statusLabel || '空闲' }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">当前进度</div>
        <div class="metric-value">{{ progressLabel }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">代码基线</div>
        <div class="metric-value small">{{ deployRef }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">执行态</div>
        <div class="metric-value">{{ busy ? 'Running' : 'Ready' }}</div>
      </div>
    </div>

    <div class="gate-strip">
      <span class="status-pill ok">双评审门禁</span>
      <span class="status-pill ok">绿先发布</span>
      <span class="status-pill warn">数据 diff 待接入</span>
      <span class="status-pill warn">组件回滚策略待接入</span>
      <span class="status-pill">蓝后同步保护</span>
    </div>
  </section>
</template>

<style scoped>
.command-overview {
  padding: 1.35rem;
}
.overview-head {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 1.25rem;
  align-items: stretch;
}
h1 {
  margin: 0;
  font-size: clamp(1.9rem, 4vw, 3.2rem);
  letter-spacing: -0.055em;
  line-height: 0.98;
}
.slot-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 140px;
  padding: 1rem;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(124, 156, 255, 0.16), rgba(255, 255, 255, 0.035));
}
.slot-label,
.slot-host {
  color: var(--muted);
  font-family: var(--mono);
  font-size: 0.72rem;
}
.slot-card strong {
  font-size: 2.1rem;
  letter-spacing: -0.05em;
}
.overview-metrics {
  margin-top: 1.1rem;
}
.metric-value.small {
  font-size: 0.83rem;
  word-break: break-word;
}
.gate-strip {
  display: flex;
  gap: 0.55rem;
  flex-wrap: wrap;
  margin-top: 1rem;
}
@media (max-width: 860px) {
  .overview-head {
    grid-template-columns: 1fr;
  }
}
</style>
