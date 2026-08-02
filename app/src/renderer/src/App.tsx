import { useState } from 'react'
import { useLiveData } from './useLiveData'
import { PendingPodsView } from './views/PendingPodsView'
import { DiagnosisView } from './views/DiagnosisView'
import { CapacityView } from './views/CapacityView'

type Route =
  | { view: 'pending' }
  | { view: 'diagnosis'; namespace: string; pod: string }
  | { view: 'capacity' }

export default function App() {
  const { connected, everConnected, pendingPods, capacity } = useLiveData()
  const [route, setRoute] = useState<Route>({ view: 'pending' })

  if (!everConnected) {
    return (
      <div className="connecting">
        <div className="spinner" />
        <div>Connecting to backend at localhost:8151…</div>
      </div>
    )
  }

  const navView = route.view === 'diagnosis' ? 'pending' : route.view

  return (
    <div className="layout">
      <nav className="sidebar">
        <div className="logo">kube-sched-lens</div>
        <button
          className={navView === 'pending' ? 'active' : ''}
          onClick={() => setRoute({ view: 'pending' })}
        >
          Pending Pods
          {pendingPods.length > 0 && <span className="count">{pendingPods.length}</span>}
        </button>
        <button
          className={navView === 'capacity' ? 'active' : ''}
          onClick={() => setRoute({ view: 'capacity' })}
        >
          GPU Capacity
        </button>
      </nav>
      <main className="content">
        {!connected && <div className="banner">Backend unreachable — reconnecting…</div>}
        {route.view === 'pending' && (
          <PendingPodsView
            pods={pendingPods}
            onSelect={(p) => setRoute({ view: 'diagnosis', namespace: p.namespace, pod: p.name })}
          />
        )}
        {route.view === 'diagnosis' && (
          <DiagnosisView
            namespace={route.namespace}
            pod={route.pod}
            onBack={() => setRoute({ view: 'pending' })}
          />
        )}
        {route.view === 'capacity' && <CapacityView capacity={capacity} />}
      </main>
    </div>
  )
}
