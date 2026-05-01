import { useEffect, useState } from 'react'
import { getHealth, getStats } from '../api'

export default function StatsPage() {
  const [stats, setStats] = useState(null)
  const [health, setHealth] = useState({ ok: false, status: 0 })
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true

    async function refresh() {
      try {
        const [s, h] = await Promise.all([getStats(), getHealth()])
        if (!alive) return
        setStats(s)
        setHealth(h)
        setError('')
      } catch (err) {
        if (!alive) return
        setError(err.message)
      }
    }

    refresh()
    const id = setInterval(refresh, 3000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [])

  return (
    <section className="card">
      <h2>System Stats</h2>
      {error && <p className="error">{error}</p>}
      {!stats ? (
        <p className="muted">Loading...</p>
      ) : (
        <div className="stats-grid">
          <Stat label="Health" value={health.ok ? 'OK' : `DOWN (${health.status})`} />
          <Stat label="Accounts" value={stats.accounts_count} />
          <Stat label="Transactions" value={stats.transactions_count} />
          <Stat label="Processed Events" value={stats.processed_events_count} />
          <Stat label="Replay In Progress" value={stats.replay_in_progress ? 'YES' : 'NO'} />
          <Stat label="Chaos Enabled" value={stats.chaos_enabled ? 'YES' : 'NO'} />
        </div>
      )}
    </section>
  )
}

function Stat({ label, value }) {
  return (
    <div className="stat-box">
      <span className="muted">{label}</span>
      <strong>{value}</strong>
    </div>
  )
}
