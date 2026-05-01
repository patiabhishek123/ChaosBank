import { useState } from 'react'
import { triggerReplay } from '../api'

export default function ReplayPage() {
  const [token, setToken] = useState('REPLAY_ALL_EVENTS')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState(null)

  async function replay() {
    setLoading(true)
    setError('')
    setResult(null)
    try {
      const data = await triggerReplay(token)
      setResult(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="card">
      <h2>Replay</h2>
      <p className="muted">Clears balances and rebuilds from Kafka events.</p>
      <label className="field">
        Confirm Token
        <input value={token} onChange={(e) => setToken(e.target.value)} />
      </label>
      {error && <p className="error">{error}</p>}
      <button className="btn" onClick={replay} disabled={loading}>
        {loading ? 'REPLAYING...' : 'RUN REPLAY'}
      </button>

      {result && (
        <pre className="output">{JSON.stringify(result, null, 2)}</pre>
      )}
    </section>
  )
}
