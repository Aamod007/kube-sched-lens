import { Category, CATEGORY_LABELS } from './api'

const COLORS: Record<Category, string> = {
  'unallocated-claim': '#b58900',
  'no-matching-device': '#c4553d',
  'insufficient-capacity': '#d33682',
  taint: '#6c71c4',
  affinity: '#2aa198',
  unknown: '#657b83'
}

export function CategoryBadge({ category }: { category: Category }) {
  const color = COLORS[category] ?? COLORS.unknown
  return (
    <span className="badge" style={{ backgroundColor: color + '2e', color, borderColor: color + '66' }}>
      {CATEGORY_LABELS[category] ?? category}
    </span>
  )
}
