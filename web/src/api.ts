import type {
  AuthStatus,
  HistoryResponse,
  InsightResponse,
  MetricSnapshot
} from './types'

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
      // ignore parse errors
    }
    throw new Error(message)
  }

  return (await response.json()) as T
}

export function getAuthStatus() {
  return request<AuthStatus>('/api/auth/status')
}

export function refreshMetrics() {
  return request<MetricSnapshot>('/api/metrics/refresh', {
    method: 'POST',
    body: JSON.stringify({})
  })
}

export function getLatestMetrics() {
  return request<MetricSnapshot>('/api/metrics/latest')
}

export function getMetricHistory(days = 180) {
  return request<HistoryResponse>(`/api/metrics/history?days=${days}`)
}

export function getInsights() {
  return request<InsightResponse>('/api/recommendations/insights')
}

export async function logout() {
  return request<{ ok: boolean }>('/api/auth/logout', {
    method: 'POST',
    body: JSON.stringify({})
  })
}
