import { PendingPod, formatDuration } from '../api'
import { CategoryBadge } from '../CategoryBadge'

interface Props {
  pods: PendingPod[]
  onSelect: (pod: PendingPod) => void
}

export function PendingPodsView({ pods, onSelect }: Props) {
  return (
    <div className="view">
      <h1>Pending Pods</h1>
      {pods.length === 0 ? (
        <div className="empty">No pending pods. All workloads are scheduled.</div>
      ) : (
        <table className="pod-table">
          <thead>
            <tr>
              <th>Pod</th>
              <th>Waiting</th>
              <th>Category</th>
              <th>Summary</th>
            </tr>
          </thead>
          <tbody>
            {pods.map((p) => (
              <tr key={`${p.namespace}/${p.name}`} onClick={() => onSelect(p)}>
                <td>
                  <span className="ns">{p.namespace}/</span>
                  {p.name}
                </td>
                <td className="mono">{formatDuration(p.sinceSeconds)}</td>
                <td>
                  <CategoryBadge category={p.category} />
                </td>
                <td className="summary-cell">{p.summary}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
