import { useEffect, useState } from 'react'
import { createAccount, listAccounts } from '../api'

const initialForm = {
  account_number: '',
  owner_name: '',
  initial_balance: '0',
  currency: 'USD',
}

export default function AccountsPage() {
  const [rows, setRows] = useState([])
  const [form, setForm] = useState(initialForm)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  async function refresh() {
    try {
      const data = await listAccounts(200)
      setRows(Array.isArray(data) ? data : [])
      setError('')
    } catch (err) {
      setError(err.message)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  async function onSubmit(e) {
    e.preventDefault()
    setLoading(true)
    setError('')
    setMessage('')
    try {
      await createAccount({
        account_number: form.account_number.trim(),
        owner_name: form.owner_name.trim(),
        initial_balance: Number(form.initial_balance),
        currency: form.currency.trim().toUpperCase(),
      })
      setMessage('Account created')
      setForm(initialForm)
      await refresh()
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="card">
      <h2>Accounts</h2>
      <p className="muted">Create accounts dynamically before transfers/replay demos.</p>

      <form className="account-form" onSubmit={onSubmit}>
        <label className="field">
          Account Number
          <input
            value={form.account_number}
            onChange={(e) => setForm((f) => ({ ...f, account_number: e.target.value }))}
            placeholder="acc-alice"
            required
          />
        </label>

        <label className="field">
          Owner Name
          <input
            value={form.owner_name}
            onChange={(e) => setForm((f) => ({ ...f, owner_name: e.target.value }))}
            placeholder="Alice"
            required
          />
        </label>

        <label className="field">
          Initial Balance
          <input
            type="number"
            step="0.01"
            min="0"
            value={form.initial_balance}
            onChange={(e) => setForm((f) => ({ ...f, initial_balance: e.target.value }))}
            required
          />
        </label>

        <label className="field">
          Currency
          <input
            value={form.currency}
            maxLength={3}
            onChange={(e) => setForm((f) => ({ ...f, currency: e.target.value }))}
            required
          />
        </label>

        <button className="btn" type="submit" disabled={loading}>
          {loading ? 'CREATING...' : 'CREATE ACCOUNT'}
        </button>
      </form>

      {message && <p className="muted">{message}</p>}
      {error && <p className="error">{error}</p>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Account #</th>
              <th>Owner</th>
              <th>Balance</th>
              <th>Currency</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr><td colSpan={6}>No accounts yet.</td></tr>
            ) : (
              rows.map((row) => (
                <tr key={row.id}>
                  <td>{row.account_number}</td>
                  <td>{row.owner_name}</td>
                  <td>{Number(row.balance).toFixed(2)}</td>
                  <td>{row.currency}</td>
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
