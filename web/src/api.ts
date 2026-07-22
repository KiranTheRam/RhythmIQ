import type { AuthStatus, Dashboard } from './types'

const headers = {
  'Content-Type': 'application/json'
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    ...options,
    headers: {
      ...headers,
      ...(options.headers ?? {})
    }
  })

  if (!response.ok) {
    let message = `Request failed (${response.status})`
    try {
      const payload = (await response.json()) as { error?: string }
      if (payload.error) {
        message = payload.error
      }
    } catch {
      // Response had no JSON body; keep the status-based message.
    }
    throw new Error(message)
  }

  return (await response.json()) as T
}

export function getAuthStatus() {
  return request<AuthStatus>('/api/auth/status')
}

export async function getDashboard() {
  return normalize(await request<Dashboard>('/api/dashboard'))
}

export async function refreshDashboard() {
  return normalize(
    await request<Dashboard>('/api/dashboard/refresh', {
      method: 'POST',
      body: JSON.stringify({})
    })
  )
}

/**
 * Guarantees every list on the payload is an array. A missing list would
 * otherwise throw on `.length` during render and blank the whole page.
 */
function normalize(dashboard: Dashboard): Dashboard {
  return {
    ...dashboard,
    playedAt: dashboard.playedAt ?? [],
    periods: (dashboard.periods ?? []).map((period) => ({
      ...period,
      artists: period.artists ?? [],
      tracks: period.tracks ?? [],
      genres: period.genres ?? [],
      newArtists: period.newArtists ?? [],
      decades: period.decades ?? []
    }))
  }
}

export function logout() {
  return request<{ ok: boolean }>('/api/auth/logout', {
    method: 'POST',
    body: JSON.stringify({})
  })
}
