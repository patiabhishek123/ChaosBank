import { useEffect, useState } from 'react'
import { getTransactionLog } from '../api'

export default function TransactionLogPage() {
  const [rows, setRows] = useState([])
  const [error, setError] = useState('')
  const [lastUpdated, setLastUpdated] = useState('')

  useEffect(() => {
    let active = true

    async function refresh() {
      try {
        const data = await getTransactionLog(100)
        if (!active) return
        setRows(Array.isArray(data) ? data : [])
        setError('')
        setLastUpdated(new Date().toLocaleTimeString())
      } catch (err) {
        if (!active) return
        setError(err.message)
      }
    }

    refresh()
    const id = setInterval(refresh, 2000)
    return () => {
      active = false
      clearInterval(id)
    }
  }, [])

  return (
    <section className="card">
      <h2>Transaction Log (Live)</h2>
      <p className="muted">Updated every 2s {lastUpdated ? `• ${lastUpdated}` : ''}</p>
      {error && <p className="error">{error}</p>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>From</th>
              <th>To</th>
              <th>Amount</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr><td colSpan={6}>No transactions yet.</td></tr>
            ) : (
              rows.map((row) => (
                <tr key={row.id}>
                  <td>{row.id}</td>
                  <td>{row.from}</td>
                  <td>{row.to}</td>
                  <td>{Number(row.amount).toFixed(2)}</td>
                  <td>{row.status}</td>
                  <td>{new Date(row.created_at).toLocaleString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  )
}
