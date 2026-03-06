import { useEffect, useMemo, useState } from 'react'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  PolarAngleAxis,
  PolarGrid,
  Radar,
  RadarChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from 'recharts'
import {
  getAuthStatus,
  getInsights,
  getLatestMetrics,
  getMetricHistory,
  logout,
  refreshMetrics
} from './api'
import type {
  AuthStatus,
  InsightResponse,
  MetricSnapshot,
  SnapshotPoint
} from './types'

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>
}

const numberFormat = new Intl.NumberFormat('en-US')
type DashboardTab = 'overview' | 'ai'

function App() {
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [snapshot, setSnapshot] = useState<MetricSnapshot | null>(null)
  const [history, setHistory] = useState<SnapshotPoint[]>([])
  const [insights, setInsights] = useState<InsightResponse | null>(null)
  const [insightsLoading, setInsightsLoading] = useState(false)
  const [insightsError, setInsightsError] = useState<string | null>(null)
  const [insightsSnapshotID, setInsightsSnapshotID] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<DashboardTab>('overview')
  const [installPrompt, setInstallPrompt] = useState<BeforeInstallPromptEvent | null>(null)

  useEffect(() => {
    const handler = (event: Event) => {
      event.preventDefault()
      setInstallPrompt(event as BeforeInstallPromptEvent)
    }
    window.addEventListener('beforeinstallprompt', handler)
    return () => window.removeEventListener('beforeinstallprompt', handler)
  }, [])

  useEffect(() => {
    void bootstrap()
  }, [])

  useEffect(() => {
    if (
      activeTab !== 'ai' ||
      !auth?.authenticated ||
      !snapshot ||
      insightsLoading ||
      insightsSnapshotID === snapshot.id
    ) {
      return
    }
    void fetchInsights()
  }, [activeTab, auth?.authenticated, snapshot, insightsLoading, insightsSnapshotID])

  async function bootstrap() {
    setLoading(true)
    setError(null)
    try {
      const status = await getAuthStatus()
      setAuth(status)
      if (status.authenticated) {
        await loadDashboardData()
      }
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  async function loadDashboardData() {
    const latestPromise = getLatestMetrics()
    const historyPromise = getMetricHistory(180)

    const [latest, historyResp] = await Promise.allSettled([latestPromise, historyPromise])

    if (latest.status === 'fulfilled') {
      setSnapshot(latest.value)
    } else {
      setSnapshot(null)
      const message = getErrorMessage(latest.reason)
      if (!message.includes('no snapshots found')) {
        setError(message)
      }
    }

    if (historyResp.status === 'fulfilled') {
      setHistory(historyResp.value.points)
    }
  }

  async function handleRefresh() {
    setRefreshing(true)
    setError(null)
    try {
      await refreshMetrics()
      setInsights(null)
      setInsightsError(null)
      setInsightsSnapshotID(null)
      await loadDashboardData()
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setRefreshing(false)
    }
  }

  async function handleLogout() {
    try {
      await logout()
      await bootstrap()
      setSnapshot(null)
      setHistory([])
      setInsights(null)
      setInsightsError(null)
      setInsightsSnapshotID(null)
      setActiveTab('overview')
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  async function fetchInsights(force = false) {
    if (!snapshot) {
      return
    }
    if (!force && insightsSnapshotID === snapshot.id) {
      return
    }

    setInsightsLoading(true)
    setInsightsError(null)
    try {
      const response = await getInsights()
      setInsights(response)
      setInsightsSnapshotID(snapshot.id)
    } catch (err) {
      setInsightsError(getErrorMessage(err))
    } finally {
      setInsightsLoading(false)
    }
  }

  async function handleInstall() {
    if (!installPrompt) {
      return
    }
    await installPrompt.prompt()
    await installPrompt.userChoice
    setInstallPrompt(null)
  }

  const historyChartData = useMemo(
    () =>
      history.map((point) => ({
        date: new Date(point.capturedAt).toLocaleDateString(undefined, {
          month: 'short',
          day: 'numeric'
        }),
        minutes: point.estimatedDailyMinutes,
        discovery: point.discoveryScore,
        consistency: point.consistencyScore
      })),
    [history]
  )

  const moodChartData = useMemo(() => {
    if (!snapshot) {
      return []
    }
    const entries = Object.entries(snapshot.stats.moodVector)
    return entries.map(([key, value]) => ({
      trait: prettifyTrait(key),
      value
    }))
  }, [snapshot])

  const daypartData = useMemo(() => {
    if (!snapshot) {
      return []
    }
    const order = ['morning', 'afternoon', 'evening', 'night']
    return order.map((key) => ({
      label: prettifyTrait(key),
      value: snapshot.stats.listeningByDaypart?.[key] ?? 0
    }))
  }, [snapshot])

  const weekdayData = useMemo(() => {
    if (!snapshot) {
      return []
    }
    const order = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']
    return order.map((key) => ({
      day: key.toUpperCase(),
      value: snapshot.stats.listeningByWeekday?.[key] ?? 0
    }))
  }, [snapshot])

  const behaviorTrendData = useMemo(
    () =>
      history.map((point) => ({
        date: new Date(point.capturedAt).toLocaleDateString(undefined, {
          month: 'short',
          day: 'numeric'
        }),
        variety: point.varietyScore,
        replay: point.replayScore,
        concentration: point.topTrackConcentration,
        weekend: point.weekendListeningShare,
        night: point.nightOwlScore
      })),
    [history]
  )

  const sessionTrendData = useMemo(
    () =>
      history.map((point) => ({
        date: new Date(point.capturedAt).toLocaleDateString(undefined, {
          month: 'short',
          day: 'numeric'
        }),
        sessions: point.sessionCount,
        avgSession: point.averageSessionMinutes
      })),
    [history]
  )

  const topArtists = snapshot?.topArtists.short_term.slice(0, 8) ?? []
  const topTracks = snapshot?.topTracks.short_term.slice(0, 8) ?? []
  const topArtistYtd = snapshot?.stats.topArtistMinutesYtd ?? []
  const topArtistAllTime = snapshot?.stats.topArtistMinutesAllTime ?? []

  if (loading) {
    return (
      <main className="app-shell">
        <div className="panel loading-panel">Calibrating your listening universe...</div>
      </main>
    )
  }

  return (
    <main className="app-shell">
      <div className="backdrop-grid" />
      <div className="grain-overlay" />

      <header className="hero panel reveal">
        <div>
          <h1 className="wordmark">RhythmIQ</h1>
          <p className="subtitle">
            Wrapped-style Spotify intelligence with persistent history and behavior-aware recommendations.
          </p>
        </div>
        <div className="hero-actions">
          {installPrompt && (
            <button className="btn btn-accent" onClick={handleInstall}>
              Install App
            </button>
          )}
          {auth?.authenticated && (
            <>
              <button className="btn" onClick={handleRefresh} disabled={refreshing}>
                {refreshing ? 'Refreshing...' : 'Refresh Metrics'}
              </button>
              <button className="btn btn-ghost" onClick={handleLogout}>
                Disconnect
              </button>
            </>
          )}
        </div>
      </header>

      {error && <section className="panel error-panel reveal">{error}</section>}

      {!auth?.spotifyConfigured && (
        <section className="panel setup-panel reveal delay-1">
          <h2>Spotify Configuration Required</h2>
          <p>
            Set <code>SPOTIFY_CLIENT_ID</code>, <code>SPOTIFY_CLIENT_SECRET</code>, and optional{' '}
            <code>SPOTIFY_REDIRECT_URL</code> in your backend environment.
          </p>
        </section>
      )}

      {auth?.spotifyConfigured && !auth.authenticated && (
        <section className="panel setup-panel reveal delay-1">
          <h2>Connect Your Spotify Account</h2>
          <p>
            Authorize RhythmIQ to read top tracks, artists, library counts, and recently played history.
          </p>
          <a className="btn btn-accent" href="/api/auth/login">
            Connect Spotify
          </a>
        </section>
      )}

      {auth?.authenticated && snapshot && (
        <>
          <section className="panel tabs-panel reveal delay-1">
            <button
              className={`tab-button ${activeTab === 'overview' ? 'active' : ''}`}
              onClick={() => setActiveTab('overview')}
            >
              Overview
            </button>
            <button
              className={`tab-button ${activeTab === 'ai' ? 'active' : ''}`}
              onClick={() => setActiveTab('ai')}
            >
              AI Insights
            </button>
          </section>

          {activeTab === 'overview' && (
            <section className="stats-grid reveal delay-1">
              <MetricCard
                label="Daily Minutes"
                value={numberFormat.format(snapshot.stats.estimatedDailyMinutes)}
                description="Estimated listening time per day based on your recent playback window."
              />
              <MetricCard
                label="Year Pace"
                value={`${numberFormat.format(snapshot.stats.estimatedYearMinutes)} min`}
                description="Projected yearly listening minutes if your current daily pace remains steady."
              />
              <MetricCard
                label="Discovery"
                value={`${snapshot.stats.discoveryScore.toFixed(1)}%`}
                description="How much of your recent top artists are new compared with your longer-term profile."
              />
              <MetricCard
                label="Consistency"
                value={`${snapshot.stats.consistencyScore.toFixed(1)}%`}
                description="Overlap between short-term and long-term favorites; higher means more stable taste."
              />
              <MetricCard
                label="Replay Index"
                value={`${snapshot.stats.replayScore.toFixed(1)}%`}
                description="Share of recent plays that were repeats of tracks already played in the same window."
              />
              <MetricCard
                label="Unique Artists"
                value={numberFormat.format(snapshot.stats.uniqueArtistCount)}
                description="Distinct artists represented across your tracked top tracks."
              />
              <MetricCard
                label="Saved Tracks"
                value={numberFormat.format(snapshot.savedTrackCount)}
                description="Total tracks currently saved to your Spotify library."
              />
              <MetricCard
                label="Playlists"
                value={numberFormat.format(snapshot.playlistCount)}
                description="Total playlists you own or follow."
              />
              <MetricCard
                label="Variety Score"
                value={`${snapshot.stats.varietyScore.toFixed(1)}%`}
                description="Inverse of concentration in your top repeated tracks. Higher means broader rotation."
              />
              <MetricCard
                label="Recent Sessions"
                value={numberFormat.format(snapshot.stats.sessionCount)}
                description="Estimated number of listening sessions inferred from breaks between recent plays."
              />
              <MetricCard
                label="Avg Session"
                value={`${snapshot.stats.averageSessionMinutes.toFixed(1)} min`}
                description="Average minutes per inferred listening session."
              />
              <MetricCard
                label="Avg Track Length"
                value={`${snapshot.stats.averageTrackMinutes.toFixed(2)} min`}
                description="Average duration of recently played tracks."
              />
              <MetricCard
                label="Weekend Share"
                value={`${snapshot.stats.weekendListeningShare.toFixed(1)}%`}
                description="Share of recent listening that happened on Saturday and Sunday."
              />
              <MetricCard
                label="Night Owl Score"
                value={`${snapshot.stats.nightOwlScore.toFixed(1)}%`}
                description="Share of listening between 10PM and 5AM."
              />
              <MetricCard
                label="Peak Hour"
                value={formatHour(snapshot.stats.peakListeningHour)}
                description="Hour of day with the highest number of recent plays."
              />
              <MetricCard
                label="Top-3 Concentration"
                value={`${snapshot.stats.topTrackConcentration.toFixed(1)}%`}
                description="How much your three most repeated recent tracks dominate your listening."
              />
            </section>
          )}

          {activeTab === 'overview' && (
            <>
              <section className="charts-grid reveal delay-2">
                <article className="panel chart-panel">
                  <h3>Listening Trajectory</h3>
                  <p className="chart-subtitle">Daily minutes, discovery, and consistency across snapshots.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={historyChartData}>
                        <CartesianGrid stroke="rgba(146, 167, 198, 0.17)" strokeDasharray="4 6" />
                        <XAxis dataKey="date" stroke="#aac3e7" />
                        <YAxis stroke="#aac3e7" />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                        <Legend />
                        <Area type="monotone" dataKey="minutes" stroke="#18f2b2" fill="rgba(24,242,178,0.25)" />
                        <Area type="monotone" dataKey="discovery" stroke="#ff8f63" fill="rgba(255,143,99,0.2)" />
                        <Area type="monotone" dataKey="consistency" stroke="#f7d364" fill="rgba(247,211,100,0.22)" />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                </article>

                <article className="panel chart-panel">
                  <h3>Genre Gravity</h3>
                  <p className="chart-subtitle">Weighted by rank and time range to reveal your sonic center.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={snapshot.stats.topGenres} layout="vertical">
                        <CartesianGrid stroke="rgba(146, 167, 198, 0.17)" strokeDasharray="4 6" />
                        <XAxis type="number" stroke="#aac3e7" />
                        <YAxis dataKey="genre" type="category" width={120} stroke="#aac3e7" />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                        <Bar dataKey="weight" fill="url(#genreBar)" radius={[0, 10, 10, 0]} />
                        <defs>
                          <linearGradient id="genreBar" x1="0" y1="0" x2="1" y2="0">
                            <stop offset="0%" stopColor="#1be3d0" />
                            <stop offset="100%" stopColor="#fdb557" />
                          </linearGradient>
                        </defs>
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </article>

                <article className="panel chart-panel">
                  <h3>Mood Vector</h3>
                  <p className="chart-subtitle">A compact fingerprint of your current listening behavior.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <RadarChart data={moodChartData}>
                        <PolarGrid stroke="rgba(146, 167, 198, 0.3)" />
                        <PolarAngleAxis dataKey="trait" stroke="#aac3e7" />
                        <Radar
                          name="Mood"
                          dataKey="value"
                          stroke="#ff7a59"
                          fill="#ff7a59"
                          fillOpacity={0.5}
                        />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                      </RadarChart>
                    </ResponsiveContainer>
                  </div>
                </article>
              </section>

              <section className="lists-grid reveal delay-2">
                <article className="panel list-panel">
                  <h3>Top Artists Right Now</h3>
                  <ul>
                    {topArtists.map((artist, index) => (
                      <li key={artist.id}>
                        <span className="rank">{index + 1}</span>
                        <div>
                          <strong>{artist.name}</strong>
                          <small>{artist.genres.slice(0, 2).join(' / ') || 'Genre unknown'}</small>
                        </div>
                        {artist.externalUrl && (
                          <a href={artist.externalUrl} target="_blank" rel="noopener noreferrer">
                            Open
                          </a>
                        )}
                      </li>
                    ))}
                  </ul>
                </article>

                <article className="panel list-panel">
                  <h3>Top Tracks Right Now</h3>
                  <ul>
                    {topTracks.map((track, index) => (
                      <li key={track.id}>
                        <span className="rank">{index + 1}</span>
                        <div>
                          <strong>{track.name}</strong>
                          <small>{track.artists.join(', ')}</small>
                        </div>
                        {track.externalUrl && (
                          <a href={track.externalUrl} target="_blank" rel="noopener noreferrer">
                            Play
                          </a>
                        )}
                      </li>
                    ))}
                  </ul>
                </article>
              </section>

              <section className="lists-grid reveal delay-2">
                <article className="panel list-panel">
                  <h3>Top Artist Minutes (Year to Date)</h3>
                  {topArtistYtd.length === 0 ? (
                    <p className="chart-subtitle">No YTD artist-minute data yet.</p>
                  ) : (
                    <ul>
                      {topArtistYtd.map((item, index) => (
                        <li key={`ytd-${item.name}`}>
                          <span className="rank">{index + 1}</span>
                          <div>
                            <strong>{item.name}</strong>
                            <small>{numberFormat.format(item.minutes)} estimated minutes</small>
                          </div>
                          {item.externalUrl && (
                            <a href={item.externalUrl} target="_blank" rel="noopener noreferrer">
                              Open
                            </a>
                          )}
                        </li>
                      ))}
                    </ul>
                  )}
                </article>

                <article className="panel list-panel">
                  <h3>Top Artist Minutes (All Time)</h3>
                  {topArtistAllTime.length === 0 ? (
                    <p className="chart-subtitle">No all-time artist-minute data yet.</p>
                  ) : (
                    <ul>
                      {topArtistAllTime.map((item, index) => (
                        <li key={`all-${item.name}`}>
                          <span className="rank">{index + 1}</span>
                          <div>
                            <strong>{item.name}</strong>
                            <small>{numberFormat.format(item.minutes)} estimated minutes</small>
                          </div>
                          {item.externalUrl && (
                            <a href={item.externalUrl} target="_blank" rel="noopener noreferrer">
                              Open
                            </a>
                          )}
                        </li>
                      ))}
                    </ul>
                  )}
                </article>
              </section>

              <section className="panel habit-panel reveal delay-2">
                <h3>Listening Habits Breakdown</h3>
                <p className="chart-subtitle">Time-of-day behavior from your recent playback window.</p>
                <div className="daypart-grid">
                  {daypartData.map((item) => (
                    <article key={item.label} className="daypart-item">
                      <div className="daypart-head">
                        <span>{item.label}</span>
                        <strong>{item.value.toFixed(1)}%</strong>
                      </div>
                      <div className="daypart-bar-track">
                        <div className="daypart-bar-fill" style={{ width: `${Math.min(item.value, 100)}%` }} />
                      </div>
                    </article>
                  ))}
                </div>
              </section>

              <section className="charts-grid reveal delay-2">
                <article className="panel chart-panel">
                  <h3>Behavior Trend Scores</h3>
                  <p className="chart-subtitle">Variety, replay tendency, and top-track concentration over time.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={behaviorTrendData}>
                        <CartesianGrid stroke="rgba(146, 167, 198, 0.17)" strokeDasharray="4 6" />
                        <XAxis dataKey="date" stroke="#aac3e7" />
                        <YAxis stroke="#aac3e7" domain={[0, 100]} />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                        <Legend />
                        <Area type="monotone" dataKey="variety" stroke="#1de5ba" fill="rgba(29,229,186,0.2)" />
                        <Area type="monotone" dataKey="replay" stroke="#ff8f63" fill="rgba(255,143,99,0.2)" />
                        <Area
                          type="monotone"
                          dataKey="concentration"
                          stroke="#f7d364"
                          fill="rgba(247,211,100,0.18)"
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                </article>

                <article className="panel chart-panel">
                  <h3>Session Dynamics</h3>
                  <p className="chart-subtitle">How session count and average session length evolve across snapshots.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={sessionTrendData}>
                        <CartesianGrid stroke="rgba(146, 167, 198, 0.17)" strokeDasharray="4 6" />
                        <XAxis dataKey="date" stroke="#aac3e7" />
                        <YAxis stroke="#aac3e7" />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                        <Legend />
                        <Area type="monotone" dataKey="sessions" stroke="#6ec2ff" fill="rgba(110,194,255,0.2)" />
                        <Area type="monotone" dataKey="avgSession" stroke="#ff6f91" fill="rgba(255,111,145,0.18)" />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                </article>

                <article className="panel chart-panel">
                  <h3>Weekday Rhythm</h3>
                  <p className="chart-subtitle">Current listening share by weekday.</p>
                  <div className="chart-wrap">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={weekdayData}>
                        <CartesianGrid stroke="rgba(146, 167, 198, 0.17)" strokeDasharray="4 6" />
                        <XAxis dataKey="day" stroke="#aac3e7" />
                        <YAxis stroke="#aac3e7" domain={[0, 100]} />
                        <Tooltip
                          contentStyle={{ background: '#081022', border: '1px solid #28548a', borderRadius: 12 }}
                          labelStyle={{ color: '#d9e7ff' }}
                        />
                        <Bar dataKey="value" fill="url(#weekdayBar)" radius={[8, 8, 0, 0]} />
                        <defs>
                          <linearGradient id="weekdayBar" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="0%" stopColor="#57d7ff" />
                            <stop offset="100%" stopColor="#74f2cf" />
                          </linearGradient>
                        </defs>
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </article>
              </section>
            </>
          )}

          {activeTab === 'ai' && (
            <section className="panel ai-tab-panel reveal delay-2">
              <div className="insights-header">
                <h3>Personalized Recommendations</h3>
                <span>{insights?.openAIGenerated ? 'AI-Generated' : 'Awaiting Analysis'}</span>
              </div>
              <p className="chart-subtitle">
                AI content is generated only when this tab is opened to reduce token usage and API cost.
              </p>

              {insightsLoading && <p className="narrative">Generating insights from your latest listening profile...</p>}

              {insightsError && (
                <div className="error-inline">
                  <p>{insightsError}</p>
                  <button className="btn" onClick={() => void fetchInsights(true)} disabled={insightsLoading}>
                    Retry
                  </button>
                </div>
              )}

              {!insightsLoading && insights && (
                <>
                  <p className="narrative">{insights.narrative}</p>
                  <h4 className="ai-section-title">Habit Recommendations</h4>
                  <div className="recommendation-grid">
                    {insights.recommendations.map((item) => (
                      <article key={item.title}>
                        <p className="tag">{item.type.toUpperCase()}</p>
                        <h4>{item.title}</h4>
                        <p>{item.description}</p>
                        <small>Confidence: {item.confidence}</small>
                      </article>
                    ))}
                  </div>

                  <h4 className="ai-section-title">Genre Recommendations</h4>
                  <div className="recommendation-grid">
                    {insights.genreRecommendations.map((item) => (
                      <article key={item.genre}>
                        <p className="tag">GENRE</p>
                        <h4>{item.genre}</h4>
                        <p>{item.reason}</p>
                        <small>Confidence: {item.confidence}</small>
                      </article>
                    ))}
                  </div>

                  <h4 className="ai-section-title">Artist Recommendations</h4>
                  <div className="recommendation-grid">
                    {insights.artistRecommendations.map((item) => (
                      <article key={item.name}>
                        <p className="tag">ARTIST</p>
                        <h4>{item.name}</h4>
                        <p>{item.reason}</p>
                        <small>Confidence: {item.confidence}</small>
                        {item.externalUrl && (
                          <a
                            className="entity-link"
                            href={item.externalUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Open in Spotify
                          </a>
                        )}
                      </article>
                    ))}
                  </div>

                  <h4 className="ai-section-title">Song Recommendations</h4>
                  <div className="recommendation-grid">
                    {insights.songRecommendations.map((item) => (
                      <article key={`${item.track}-${item.artist}`}>
                        <p className="tag">SONG</p>
                        <h4>{item.track}</h4>
                        <p>{item.artist}</p>
                        <p>{item.reason}</p>
                        <small>Confidence: {item.confidence}</small>
                        {item.externalUrl && (
                          <a
                            className="entity-link"
                            href={item.externalUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Open in Spotify
                          </a>
                        )}
                      </article>
                    ))}
                  </div>
                </>
              )}
            </section>
          )}
        </>
      )}

      {auth?.authenticated && !snapshot && (
        <section className="panel setup-panel reveal delay-1">
          <h2>No snapshots yet</h2>
          <p>Generate your first data snapshot to populate analytics and trend charts.</p>
          <button className="btn btn-accent" onClick={handleRefresh} disabled={refreshing}>
            {refreshing ? 'Generating...' : 'Generate First Snapshot'}
          </button>
        </section>
      )}
    </main>
  )
}

function MetricCard({ label, value, description }: { label: string; value: string; description: string }) {
  return (
    <article className="panel metric-card">
      <div className="metric-label-row">
        <p>{label}</p>
        <span className="metric-info-wrap">
          <button type="button" className="info-pill" aria-label={`What ${label} means`}>
            i
          </button>
          <div className="metric-tooltip" role="tooltip">
            {description}
          </div>
        </span>
      </div>
      <h3>{value}</h3>
    </article>
  )
}

function getErrorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message
  }
  return 'An unexpected error occurred.'
}

function prettifyTrait(value: string) {
  return value
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, (char) => char.toUpperCase())
    .trim()
}

function formatHour(hour: number) {
  const normalized = ((hour % 24) + 24) % 24
  const suffix = normalized >= 12 ? 'PM' : 'AM'
  const twelveHour = normalized % 12 === 0 ? 12 : normalized % 12
  return `${twelveHour}:00 ${suffix}`
}

export default App
