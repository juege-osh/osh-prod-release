<script setup>
const probes = [
  { name: 'MySQL', status: 'schema diff / data diff', tone: 'warn' },
  { name: 'Redis', status: 'namespace health', tone: 'ok' },
  { name: 'Elasticsearch', status: 'index count compare', tone: 'warn' },
  { name: 'Kafka', status: 'topic metadata', tone: 'warn' },
  { name: 'Nacos', status: 'config checksum', tone: 'ok' },
  { name: 'HTTP API', status: 'critical path probe', tone: 'ok' },
]
</script>

<template>
  <section class="quality-panel panel">
    <div class="section-head">
      <div>
        <p class="section-kicker">Verification Fabric</p>
        <h2 class="section-title">自动化测试与数据报告</h2>
        <p class="section-copy">上线前后保存快照，输出新增、减少、修改、异常波动，并交给 AI 判断是否符合 change 预期。</p>
      </div>
    </div>

    <div class="probe-grid">
      <div v-for="probe in probes" :key="probe.name" class="probe-card">
        <span :class="['probe-dot', probe.tone]"></span>
        <strong>{{ probe.name }}</strong>
        <small>{{ probe.status }}</small>
      </div>
    </div>
  </section>
</template>

<style scoped>
.quality-panel {
  padding: 1.15rem;
}
.section-head {
  margin-bottom: 1rem;
}
.probe-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.75rem;
}
.probe-card {
  position: relative;
  min-height: 92px;
  padding: 0.9rem;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.025);
}
.probe-card strong {
  display: block;
  margin-top: 0.9rem;
  font-size: 0.92rem;
}
.probe-card small {
  display: block;
  margin-top: 0.35rem;
  color: var(--muted);
  font-family: var(--mono);
  font-size: 0.7rem;
}
.probe-dot {
  position: absolute;
  top: 0.85rem;
  right: 0.85rem;
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 50%;
}
.probe-dot.ok {
  background: var(--ok);
}
.probe-dot.warn {
  background: var(--warn);
}
@media (max-width: 860px) {
  .probe-grid {
    grid-template-columns: 1fr 1fr;
  }
}
@media (max-width: 560px) {
  .probe-grid {
    grid-template-columns: 1fr;
  }
}
</style>
