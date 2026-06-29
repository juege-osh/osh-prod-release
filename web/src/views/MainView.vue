<script setup>
import { ref, watch } from 'vue'
import {
  NLayout,
  NLayoutHeader,
  NLayoutContent,
  NButton,
  useMessage,
} from 'naive-ui'
import { TimeOutline, PulseOutline } from '@vicons/ionicons5'
import { NIcon } from 'naive-ui'
import CommandOverview from '../components/CommandOverview.vue'
import DeployHero from '../components/DeployHero.vue'
import ChangeWarRoom from '../components/ChangeWarRoom.vue'
import ComponentSyncPanel from '../components/ComponentSyncPanel.vue'
import UserExperiencePanel from '../components/UserExperiencePanel.vue'
import QualityReportPanel from '../components/QualityReportPanel.vue'
import StepTimeline from '../components/StepTimeline.vue'
import LogTerminal from '../components/LogTerminal.vue'
import RunHistoryDrawer from '../components/RunHistoryDrawer.vue'
import { useDeployRun } from '../composables/useDeployRun.js'

const showHistory = ref(false)
const {
  health,
  run,
  history,
  busy,
  error,
  progress,
  statusLabel,
  start,
} = useDeployRun()

const message = useMessage()

watch(error, (v) => {
  if (v) message.error(v)
})

watch(
  () => run.value?.status,
  (s, prev) => {
    if (s === 'success' && prev === 'running') message.success('绿环境部署完成')
    if (s === 'failed') message.error('发布失败，请查看日志')
  }
)
</script>

<template>
  <NLayout class="shell">
    <NLayoutHeader class="topbar" bordered>
      <div class="brand">
        <div class="logo">OSH</div>
        <div>
          <div class="brand-title">OSH Release Control</div>
          <div class="brand-sub">生产发布、蓝绿切换、数据上线作战台</div>
        </div>
      </div>
      <nav class="topnav" aria-label="发布平台导航">
        <a href="#overview">总览</a>
        <a href="#change">Change</a>
        <a href="#sync">组件同步</a>
        <a href="#identity">用户中心</a>
        <a href="#quality">测试报告</a>
        <a href="#deploy">部署</a>
      </nav>
      <div class="topbar-actions">
        <span v-if="health" class="health">
          <NIcon :component="PulseOutline" :color="health.mock_mode ? 'var(--warn)' : 'var(--ok)'" />
          {{ health.mock_mode ? '演示模式' : `绿环境 · ${health.prod_host}` }}
        </span>
        <NButton quaternary @click="showHistory = true">
          <template #icon><NIcon :component="TimeOutline" /></template>
          历史
        </NButton>
      </div>
    </NLayoutHeader>

    <NLayoutContent class="content">
      <div class="container">
        <section id="overview">
          <CommandOverview
            :health="health"
            :run="run"
            :busy="busy"
            :progress="progress"
            :status-label="statusLabel"
          />
        </section>

        <div class="workspace-grid">
          <section id="change">
            <ChangeWarRoom />
          </section>
          <section id="identity">
            <UserExperiencePanel />
          </section>
        </div>

        <section id="sync">
          <ComponentSyncPanel />
        </section>

        <section id="quality">
          <QualityReportPanel />
        </section>

        <section id="deploy">
          <DeployHero
            :busy="busy"
            :progress="progress"
            :status-label="statusLabel"
            :run-id="run?.id"
            :run-mode="run?.mode"
            :mock-mode="health?.mock_mode"
            :green-url="health?.green_url"
            @start="start"
          />
        </section>

        <div class="ops-grid">
          <StepTimeline :steps="run?.steps || []" />
          <LogTerminal :lines="run?.log || []" />
        </div>
      </div>
    </NLayoutContent>
  </NLayout>

  <RunHistoryDrawer v-model:show="showHistory" :history="history" />
</template>

<style scoped>
.shell {
  min-height: 100vh;
  background: transparent;
}
.topbar {
  position: sticky;
  top: 0.75rem;
  z-index: 20;
  min-height: 68px;
  padding: 0.75rem 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin: 0.75rem 1.25rem 0;
  border: 1px solid var(--border) !important;
  border-radius: 18px !important;
  background: rgba(5, 7, 11, 0.82);
  backdrop-filter: blur(18px);
}
.brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 245px;
}
.logo {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  border: 1px solid rgba(124, 156, 255, 0.45);
  background: rgba(124, 156, 255, 0.16);
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 0.75rem;
}
.brand-title {
  font-weight: 700;
  font-size: 0.95rem;
}
.brand-sub {
  font-size: 0.75rem;
  color: var(--muted);
}
.topnav {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  padding: 0.25rem;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.035);
}
.topnav a {
  color: var(--muted-strong);
  text-decoration: none;
  font-size: 0.78rem;
  padding: 0.38rem 0.75rem;
  border-radius: 999px;
}
.topnav a:hover {
  color: var(--text);
  background: rgba(255, 255, 255, 0.065);
}
.topbar-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.health {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.85rem;
  color: var(--muted);
}
.content {
  padding: 1rem 1.25rem 2rem;
}
.container {
  max-width: 1320px;
  margin: 0 auto;
  display: grid;
  gap: 1rem;
}
.workspace-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.18fr) minmax(360px, 0.82fr);
  gap: 1rem;
  align-items: stretch;
}
.ops-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
  align-items: start;
}
@media (max-width: 1100px) {
  .topnav {
    display: none;
  }
  .workspace-grid,
  .ops-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 720px) {
  .topbar {
    align-items: flex-start;
    flex-direction: column;
  }
  .topbar-actions {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
