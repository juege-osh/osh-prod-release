<script setup>
const changeRows = [
  {
    title: '发布内核升级',
    owner: 'release-owner',
    risk: '高',
    state: '设计中',
    scope: '审批、change item、责任归属',
  },
  {
    title: '组件增量上线协议',
    owner: 'sre-owner',
    risk: '高',
    state: '待接入',
    scope: 'MySQL / Redis / ES / Kafka / Nacos',
  },
  {
    title: 'SQL dry-run 与回滚',
    owner: 'db-owner',
    risk: '高',
    state: '待接入',
    scope: 'schema diff、数据 diff、危险语句拦截',
  },
  {
    title: '生产人工复测',
    owner: 'feature-owner',
    risk: '中',
    state: '流程固化',
    scope: '切绿后真实流量验证',
  },
]

const dependencyNodes = [
  'Change 填写',
  '双评审',
  '觉哥终审',
  '绿部署',
  '自动化测试',
  '切绿',
  '人工复测',
  '同步蓝',
]
</script>

<template>
  <section class="war-room panel">
    <div class="section-head">
      <div>
        <p class="section-kicker">Change War Room</p>
        <h2 class="section-title">上线内容作战台</h2>
        <p class="section-copy">从“发布单”升级为可追责、可测试、可回滚的 change 指挥面板。</p>
      </div>
      <span class="status-pill warn">表格 / 树 / 图状三视图</span>
    </div>

    <div class="change-table">
      <div class="change-row head">
        <span>上线项</span>
        <span>负责人</span>
        <span>风险</span>
        <span>状态</span>
      </div>
      <div v-for="row in changeRows" :key="row.title" class="change-row">
        <span>
          <strong>{{ row.title }}</strong>
          <small>{{ row.scope }}</small>
        </span>
        <span class="mono">{{ row.owner }}</span>
        <span :class="['risk', row.risk === '高' ? 'high' : 'mid']">{{ row.risk }}</span>
        <span class="status-pill">{{ row.state }}</span>
      </div>
    </div>

    <div class="flow-line" aria-label="上线流程">
      <template v-for="(node, index) in dependencyNodes" :key="node">
        <span>{{ node }}</span>
        <i v-if="index < dependencyNodes.length - 1"></i>
      </template>
    </div>
  </section>
</template>

<style scoped>
.war-room {
  padding: 1.15rem;
}
.section-head {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: flex-start;
  margin-bottom: 1rem;
}
.change-table {
  border: 1px solid var(--border);
  border-radius: 14px;
  overflow: hidden;
}
.change-row {
  display: grid;
  grid-template-columns: 1.4fr 0.7fr 0.4fr 0.65fr;
  gap: 0.8rem;
  align-items: center;
  padding: 0.78rem 0.9rem;
  border-top: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.025);
}
.change-row:first-child {
  border-top: 0;
}
.change-row.head {
  color: var(--muted);
  font-family: var(--mono);
  font-size: 0.72rem;
  text-transform: uppercase;
  background: rgba(255, 255, 255, 0.045);
}
.change-row strong {
  display: block;
  font-size: 0.9rem;
}
.change-row small {
  display: block;
  margin-top: 0.25rem;
  color: var(--muted);
  font-size: 0.74rem;
}
.mono {
  font-family: var(--mono);
  color: var(--muted-strong);
  font-size: 0.78rem;
}
.risk {
  font-weight: 700;
}
.risk.high {
  color: var(--err);
}
.risk.mid {
  color: var(--warn);
}
.flow-line {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  flex-wrap: wrap;
  margin-top: 1rem;
  padding: 0.85rem;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--bg-soft);
}
.flow-line span {
  color: var(--muted-strong);
  font-size: 0.78rem;
}
.flow-line i {
  width: 22px;
  height: 1px;
  background: var(--border-strong);
}
@media (max-width: 840px) {
  .change-row {
    grid-template-columns: 1fr;
  }
}
</style>
