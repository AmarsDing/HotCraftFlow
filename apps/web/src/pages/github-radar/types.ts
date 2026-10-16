/** Local DTO until buf generate → services/kratos. Do not hand-edit kratos gen. */
export type RepoItem = {
  id: string
  full_name: string
  description: string
  language: string
  topics: string[]
  stars: number
  forks: number
  rank: number
  stars_delta: number
  is_new: boolean
  html_url: string
  sparkline: number[]
}

export type WatchItem = {
  id: string
  full_name: string
  stars: number
  stars_delta: number
  unread_count?: number
  sparkline?: number[]
  html_url?: string
}

export type WatchEvent = {
  id: string
  full_name: string
  kind: string
  message: string
  created_at: string
  read: boolean
}

export type RadarNotification = {
  id: string
  title: string
  body: string
  kind: string
  created_at: string
  read: boolean
}

export function splitFullName(full: string) {
  const i = full.indexOf('/')
  if (i < 0) return { owner: '', name: full }
  return { owner: full.slice(0, i), name: full.slice(i + 1) }
}

export function abbrevCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}
