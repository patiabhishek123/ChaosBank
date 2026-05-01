const base = import.meta.env.VITE_API_BASE_URL || '/api'

export async function getTransactionLog(limit = 50) {
  const res = await fetch(`${base}/transactions/log?limit=${limit}`)
  if (!res.ok) throw new Error(`transaction log failed: ${res.status}`)
  return res.json()
}

export async function getChaosStatus() {
  const res = await fetch(`${base}/chaos`)
  if (!res.ok) throw new Error(`chaos status failed: ${res.status}`)
  return res.json()
}

export async function setChaos(enabled) {
  const res = await fetch(`${base}/chaos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  if (!res.ok) throw new Error(`chaos toggle failed: ${res.status}`)
  return res.json()
}

export async function triggerReplay(token) {
  const res = await fetch(`${base}/replay`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Replay-Confirm': token,
    },
  })

  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `replay failed: ${res.status}`)
  }

  return res.json()
}

export async function getStats() {
  const res = await fetch(`${base}/stats`)
  if (!res.ok) throw new Error(`stats failed: ${res.status}`)
  return res.json()
}

export async function getHealth() {
  const res = await fetch(`${base}/health`)
  return { ok: res.ok, status: res.status }
}
