export async function fetchHealth() {
  const res = await fetch('/api/health')
  if (!res.ok) throw new Error('health check failed')
  return res.json()
}

export async function startDeploy(mode) {
  const res = await fetch('/api/deploy/start', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `HTTP ${res.status}`)
  }
  return res.json()
}

export async function fetchRuns() {
  const res = await fetch('/api/runs')
  if (!res.ok) throw new Error('list runs failed')
  return res.json()
}

export async function fetchComponentSyncActive() {
  const res = await fetch('/api/component-sync/active')
  if (!res.ok) throw new Error('component sync status failed')
  return res.json()
}

export async function startBlueToGreenAllComponentSync(reason = '') {
  const res = await fetch('/api/component-sync/blue-to-green/all', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ actor: 'ops', reason }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `HTTP ${res.status}`)
  }
  return res.json()
}

export async function fetchRun(runId) {
  const res = await fetch(`/api/runs/${runId}`)
  if (!res.ok) throw new Error('get run failed')
  return res.json()
}

export function streamRun(runId, onData) {
  const es = new EventSource(`/api/runs/${runId}/stream`)
  es.onmessage = (ev) => {
    try {
      onData(JSON.parse(ev.data))
    } catch (_) {}
  }
  es.onerror = () => es.close()
  return es
}
