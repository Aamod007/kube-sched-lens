import { useState } from 'react'
import { CapacityPool } from '../api'

function PoolCard({ pool }: { pool: CapacityPool }) {
  const [expanded, setExpanded] = useState(false)
  const pct = pool.deviceCount > 0 ? (pool.allocatedCount / pool.deviceCount) * 100 : 0
  const full = pool.allocatedCount >= pool.deviceCount

  return (
    <div className="cap-card">
      <div className="cap-head">
        <div>
          <div className="cap-driver">{pool.driver}</div>
          <div className="cap-sub">
            pool <span className="mono">{pool.pool}</span> · node <span className="mono">{pool.node}</span>
          </div>
        </div>
        <div className={'cap-count' + (full ? ' cap-full' : '')}>
          {pool.allocatedCount}/{pool.deviceCount}
        </div>
      </div>
      <div className="cap-bar">
        <div className={'cap-bar-fill' + (full ? ' cap-full-bg' : '')} style={{ width: `${pct}%` }} />
      </div>
      {pool.devices.length > 0 && (
        <>
          <button className="cap-toggle" onClick={() => setExpanded(!expanded)}>
            {expanded ? 'Hide' : 'Show'} {pool.devices.length} device{pool.devices.length !== 1 ? 's' : ''}
          </button>
          {expanded && (
            <div className="device-list">
              {pool.devices.map((d) => (
                <div className="device" key={d.name}>
                  <div className="device-name mono">{d.name}</div>
                  <div className="device-attrs">
                    {Object.entries(d.attributes ?? {}).map(([k, v]) => (
                      <span className="attr" key={k}>
                        <span className="attr-k">{k}</span>=<span className="mono">{String(v)}</span>
                      </span>
                    ))}
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

export function CapacityView({ capacity }: { capacity: CapacityPool[] }) {
  return (
    <div className="view">
      <h1>GPU Capacity</h1>
      {capacity.length === 0 ? (
        <div className="empty">No ResourceSlices found. No DRA drivers are publishing capacity.</div>
      ) : (
        <div className="cap-grid">
          {capacity.map((p) => (
            <PoolCard pool={p} key={`${p.driver}/${p.pool}/${p.node}`} />
          ))}
        </div>
      )}
    </div>
  )
}
