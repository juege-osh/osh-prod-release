<script setup>
const baselines = [
  '邮箱注册、用户名、密码强度、邀请码',
  '登录后保存 token、权限列表、角色等级',
  '用户中心含订单、核销码、会员、邀请、资料、安全设置',
  '后台用户管理含筛选、角色、拉黑、邀请用户',
]

const upgrades = [
  '注册改为分步流程：身份信息、邀请校验、安全确认、完成引导',
  '用户中心展示权限来源、审核职责、近期发布参与、登录会话与风险提示',
  '管理端支持邀请批量导入、角色有效期、操作审计、责任人追溯',
  '安全体验补齐：强密码反馈、二次确认、会话列表、异常登录提示',
]
</script>

<template>
  <section class="user-panel panel">
    <div class="section-head">
      <div>
        <p class="section-kicker">Identity Center</p>
        <h2 class="section-title">注册与用户中心升级</h2>
        <p class="section-copy">以绿项目用户模块为基线，但在体验、功能、可用性和责任追踪上全面超过它。</p>
      </div>
      <span class="status-pill ok">全项目级体验基线</span>
    </div>

    <div class="compare-grid">
      <div class="compare-card">
        <h3>绿项目已有能力</h3>
        <ul>
          <li v-for="item in baselines" :key="item">{{ item }}</li>
        </ul>
      </div>
      <div class="compare-card strong">
        <h3>发布平台升级方向</h3>
        <ul>
          <li v-for="item in upgrades" :key="item">{{ item }}</li>
        </ul>
      </div>
    </div>
  </section>
</template>

<style scoped>
.user-panel {
  padding: 1.15rem;
}
.section-head {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: flex-start;
  margin-bottom: 1rem;
}
.compare-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.85rem;
}
.compare-card {
  padding: 1rem;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.025);
}
.compare-card.strong {
  background: var(--accent-soft);
  border-color: rgba(124, 156, 255, 0.32);
}
h3 {
  margin: 0 0 0.75rem;
  font-size: 0.9rem;
}
ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  gap: 0.62rem;
}
li {
  color: var(--muted-strong);
  font-size: 0.82rem;
  line-height: 1.55;
}
li::before {
  content: '';
  display: inline-block;
  width: 0.42rem;
  height: 0.42rem;
  margin-right: 0.5rem;
  border-radius: 50%;
  background: var(--accent);
  vertical-align: 0.08rem;
}
@media (max-width: 820px) {
  .compare-grid {
    grid-template-columns: 1fr;
  }
}
</style>
