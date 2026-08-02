import { useEffect, useRef, useState } from 'react'
import { API, WS_URL, PendingPod, CapacityPool } from './api'

// Polls /api/healthz until up, then opens WS; reconnects on drop.
export function useLiveData() {
  const [connected, setConnected] = useState(false)
  const [everConnected, setEverConnected] = useState(false)
  const [pendingPods, setPendingPods] = useState<PendingPod[]>([])
  const [capacity, setCapacity] = useState<CapacityPool[]>([])
  const stopped = useRef(false)

  useEffect(() => {
    stopped.current = false
    let ws: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout>

    const fetchAll = async () => {
      const [pods, cap] = await Promise.all([
        fetch(`${API}/api/pending-pods`).then((r) => r.json()),
        fetch(`${API}/api/capacity`).then((r) => r.json())
      ])
      setPendingPods(pods ?? [])
      setCapacity(cap ?? [])
    }

    const connect = async () => {
      if (stopped.current) return
      try {
        const r = await fetch(`${API}/api/healthz`)
        if (!r.ok) throw new Error('unhealthy')
        await fetchAll()
      } catch {
        setConnected(false)
        timer = setTimeout(connect, 2000)
        return
      }
      ws = new WebSocket(WS_URL)
      ws.onopen = () => {
        setConnected(true)
        setEverConnected(true)
      }
      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          if (msg.type === 'pending-pods') setPendingPods(msg.data ?? [])
          else if (msg.type === 'capacity') setCapacity(msg.data ?? [])
        } catch {
          /* ignore malformed */
        }
      }
      ws.onclose = () => {
        setConnected(false)
        if (!stopped.current) timer = setTimeout(connect, 2000)
      }
    }

    connect()
    return () => {
      stopped.current = true
      clearTimeout(timer)
      ws?.close()
    }
  }, [])

  return { connected, everConnected, pendingPods, capacity }
}
