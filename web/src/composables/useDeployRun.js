import { computed, onMounted, ref } from 'vue'
import { fetchHealth, fetchRuns, startDeploy, streamRun } from '../api/deploy.js'

export function useDeployRun() {
  const health = ref(null)
  const run = ref(null)
  const history = ref([])
  const busy = ref(false)
  const error = ref('')
  let es = null

  const progress = computed(() => {
    if (!run.value?.steps?.length) return 0
    const active = run.value.steps.filter((s) => s.status !== 'skipped')
    if (!active.length) return 0
    const done = active.filter((s) => s.status === 'success').length
    const running = active.some((s) => s.status === 'running')
    const partial = done + (running ? 0.35 : 0)
    return Math.min(100, Math.round((partial / active.length) * 100))
  })

  const statusLabel = computed(() => {
    if (!run.value) return '空闲'
    const map = { running: '进行中', success: '成功', failed: '失败', pending: '等待' }
    return map[run.value.status] || run.value.status
  })

  async function loadHealth() {
    health.value = await fetchHealth()
  }

  async function loadHistory() {
    history.value = await fetchRuns()
  }

  function applyRun(data) {
    run.value = data
    if (data.status === 'success' || data.status === 'failed') {
      busy.value = false
      if (es) {
        es.close()
        es = null
      }
      loadHistory()
    }
  }

  async function start(mode) {
    error.value = ''
    busy.value = true
    try {
      const { run_id } = await startDeploy(mode)
      if (es) es.close()
      es = streamRun(run_id, applyRun)
    } catch (e) {
      error.value = e.message
      busy.value = false
    }
  }

  onMounted(async () => {
    await loadHealth()
    await loadHistory()
  })

  return {
    health,
    run,
    history,
    busy,
    error,
    progress,
    statusLabel,
    start,
    loadHistory,
  }
}
