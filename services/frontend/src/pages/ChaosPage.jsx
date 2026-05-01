import { useEffect, useState } from 'react'
import { getChaosStatus, setChaos } from '../api'

export default function ChaosPage() {
  const [enabled, setEnabled] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    getChaosStatus()
      .then((data) => {
        setEnabled(Boolean(data.enabled))
      })
      .catch((err) => setError(err.message))
  }, [])

  async function toggle() {
    setLoading(true)
    setError('')
    try {
      const data = await setChaos(!enabled)
      setEnabled(Boolean(data.enabled))
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="card">
      <h2>Chaos Toggle</h2>
      <p className="muted">Current mode: <strong>{enabled ? 'ON' : 'OFF'}</strong></p>
      {error && <p className="error">{error}</p>}
      <button className="btn" onClick={toggle} disabled={loading}>
        {loading ? 'WORKING...' : enabled ? 'TURN CHAOS OFF' : 'TURN CHAOS ON'}
      </button>
    </section>
  )
}
