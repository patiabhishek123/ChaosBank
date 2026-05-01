import { useState } from 'react'
import TransactionLogPage from './pages/TransactionLogPage'
import ChaosPage from './pages/ChaosPage'
import ReplayPage from './pages/ReplayPage'
import StatsPage from './pages/StatsPage'

const tabs = [
  { id: 'log', label: 'TRANSACTION LOG' },
  { id: 'chaos', label: 'CHAOS TOGGLE' },
  { id: 'replay', label: 'REPLAY' },
  { id: 'stats', label: 'SYSTEM STATS' },
]

export default function App() {
  const [active, setActive] = useState('log')

  return (
    <main className="app">
      <header className="card header">
        <h1>CHAOSBANK TERMINAL</h1>
        <p className="muted">Minimalist control surface</p>
      </header>

      <nav className="tabs">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={`tab ${active === t.id ? 'tab-active' : ''}`}
            onClick={() => setActive(t.id)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      {active === 'log' && <TransactionLogPage />}
      {active === 'chaos' && <ChaosPage />}
      {active === 'replay' && <ReplayPage />}
      {active === 'stats' && <StatsPage />}
    </main>
  )
}
