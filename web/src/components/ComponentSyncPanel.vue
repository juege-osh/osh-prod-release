<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import { SyncOutline } from '@vicons/ionicons5'
import { NIcon } from 'naive-ui'
import { fetchComponentSyncActive, startBlueToGreenAllComponentSync } from '../api/deploy.js'

const message = useMessage()
const active = ref(null)
const loading = ref(false)
let timer = null

const components = [
  { name: 'MySQL', strategy: 'schema delta + REPLACE upsert', state: 'write green only' },
  { name: 'Nacos', strategy: 'config copy + osh-g rewrite', state: 'guarded' },
  { name: 'Redis', strategy: 'key restore replace by DB', state: 'idempotent' },
  { name: 'Elasticsearch', strategy: 'mapping + data upsert', state: 'osh_* indices' },
  { name: 'Kafka', strategy: 'topic metadata sync', state: 'no replay' },
]

async function load() {
  try {
    active.value = await fetchComponentSyncActive()
  } catch (_) {
    active.value = null
  }
}

async function startSync() {
  loading.value = true
  try {
    active.value = { busy: true, job: await startBlueToGreenAllComponentSync('manual all-components blue-to-green sync') }
    message.success('已启动蓝到绿所有组件增量同步')
    await load()
  } catch (e) {
    message.error(e.message || String(e))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  timer = setInterval(load, 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <section class="component-sync panel">
    <div class="section-head">
      <div>
        <p class="section-kicker">Component Sync</p>
        <h2 class="section-title">蓝到绿所有组件增量同步</h2>
        <p class="section-copy">
          生产在蓝时，把蓝环境作为只读源，增量同步到绿环境，保持绿环境是蓝环境的测试拷贝。
        </p>
      </div>
      <NButton type="primary" :loading="loading || active?.busy" @click="startSync">
        <template #icon><NIcon :component="SyncOutline" /></template>
        启动同步
      </NButton>
    </div>

    <div class="sync-state">
      <span :class="['status-pill', active?.busy ? 'warn' : active?.job?.status === 'success' ? 'ok' : '']">
        {{ active?.busy ? '同步中' : active?.job?.status === 'success' ? '最近成功' : '待命' }}
      </span>
      <span v-if="active?.job" class="job-text">
        {{ active.job.message || active.job.id }}
      </span>
    </div>

    <div class="component-grid">
      <div v-for="item in components" :key="item.name" class="component-card">
        <strong>{{ item.name }}</strong>
        <span>{{ item.strategy }}</span>
        <small>{{ item.state }}</small>
      </div>
    </div>
  </section>
</template>

<style scoped>
.component-sync {
  padding: 1.15rem;
}
.section-head {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: flex-start;
}
.sync-state {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 1rem 0;
}
.job-text {
  color: var(--muted);
  font-family: var(--mono);
  font-size: 0.75rem;
}
.component-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 0.7rem;
}
.component-card {
  min-height: 104px;
  padding: 0.85rem;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.025);
}
.component-card strong,
.component-card span,
.component-card small {
  display: block;
}
.component-card strong {
  font-size: 0.92rem;
}
.component-card span {
  margin-top: 0.45rem;
  color: var(--muted-strong);
  font-size: 0.78rem;
  line-height: 1.45;
}
.component-card small {
  margin-top: 0.45rem;
  color: var(--muted);
  font-family: var(--mono);
  font-size: 0.68rem;
}
@media (max-width: 1040px) {
  .component-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 560px) {
  .section-head {
    flex-direction: column;
  }
  .component-grid {
    grid-template-columns: 1fr;
  }
}
</style>
