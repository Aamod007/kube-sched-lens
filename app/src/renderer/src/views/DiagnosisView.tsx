import { useEffect, useState } from 'react'
import { API, Diagnosis } from '../api'
import { CategoryBadge } from '../CategoryBadge'

const KIND_ICONS: Record<string, string> = {
  Pod: 'P',
  ResourceClaim: 'RC',
  ResourceSlice: 'RS',
  DeviceClass: 'DC',
  Node: 'N',
  Event: 'E'
}

interface Props {
  namespace: string
  pod: string
  onBack: () => void
}

export function DiagnosisView({ namespace, pod, onBack }: Props) {
  const [diag, setDiag] = useState<Diagnosis | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setDiag(null)
    setError(null)
    fetch(`${API}/api/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/diagnosis`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((d) => !cancelled && setDiag(d))
      .catch((e) => !cancelled && setError(String(e.message ?? e)))
    return () => {
      cancelled = true
    }
  }, [namespace, pod])

  return (
    <div className="view">
      <button className="back-btn" onClick={onBack}>
        &larr; Back to Pending Pods
      </button>
      {error && <div className="empty">Failed to load diagnosis: {error}</div>}
      {!error && !diag && <div className="empty">Loading diagnosis…</div>}
      {diag && (
        <>
          <div className="diag-header">
            <h1>
              <span className="ns">{diag.namespace}/</span>
              {diag.pod}
            </h1>
            <CategoryBadge category={diag.category} />
          </div>
          <p className="diag-summary">{diag.summary}</p>
          {diag.suggestion && (
            <div className="suggestion">
              <div className="suggestion-title">Suggestion</div>
              {diag.suggestion}
            </div>
          )}
          <h2>Evidence</h2>
          {diag.evidence.length === 0 ? (
            <div className="empty">No evidence collected.</div>
          ) : (
            <div className="timeline">
              {diag.evidence.map((ev, i) => (
                <div className="evidence-card" key={i}>
                  <div className="evidence-icon">{KIND_ICONS[ev.kind] ?? ev.kind.slice(0, 2)}</div>
                  <div className="evidence-body">
                    <div className="evidence-name">
                      <span className="evidence-kind">{ev.kind}</span> {ev.name}
                    </div>
                    <pre className="evidence-detail">{ev.detail}</pre>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
