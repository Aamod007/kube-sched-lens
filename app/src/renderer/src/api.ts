export type Category =
  | 'unallocated-claim'
  | 'no-matching-device'
  | 'insufficient-capacity'
  | 'taint'
  | 'affinity'
  | 'unknown'

export interface PendingPod {
  namespace: string
  name: string
  sinceSeconds: number
  category: Category
  summary: string
}

export interface Evidence {
  kind: string
  name: string
  detail: string
}

export interface Diagnosis {
  pod: string
  namespace: string
  category: Category
  summary: string
  evidence: Evidence[]
  suggestion: string
}

export interface Device {
  name: string
  attributes: Record<string, unknown>
}

export interface CapacityPool {
  driver: string
  pool: string
  node: string
  deviceCount: number
  allocatedCount: number
  devices: Device[]
}

export const API = 'http://localhost:8151'
export const WS_URL = 'ws://localhost:8151/api/watch'

export const CATEGORY_LABELS: Record<Category, string> = {
  'unallocated-claim': 'Unallocated Claim',
  'no-matching-device': 'No Matching Device',
  'insufficient-capacity': 'Insufficient Capacity',
  taint: 'Taint',
  affinity: 'Affinity',
  unknown: 'Unknown'
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${Math.floor(seconds % 60)}s`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}
