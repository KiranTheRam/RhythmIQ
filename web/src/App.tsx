import { useCallback, useEffect, useMemo, useState } from 'react'
import { getAuthStatus, getDashboard, logout, refreshDashboard } from './api'
import { extractAccent } from './palette'
import { Wrapped } from './Wrapped'
import type {
  ArtistStat,
  AuthStatus,
  Dashboard,
  GenreStat,
  ListeningRun,
  PeriodKey,
  PeriodMetrics,
  ReplayStat
} from './types'

const numbers = new Intl.NumberFormat('en-US')
const PERIOD_ORDER: PeriodKey[] = ['week', 'month', 'year']
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function App() {
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [dashboard, setDashboard] = useState<Dashboard | null>(null)
  const [period, setPeriod] = useState<PeriodKey>('week')
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [wrappedFor, setWrappedFor] = useState<PeriodKey | null>(null)

  const bootstrap = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const status = await getAuthStatus()
      setAuth(status)
      if (status.authenticated) {
        try {
          setDashboard(await getDashboard())
        } catch (err) {
          const message = describeError(err)
          if (!message.includes('no dashboard yet')) {
            setError(message)
          }
          setDashboard(null)
        }
      }
    } catch (err) {
      setError(describeError(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  // The view lives in the URL hash so it is linkable, and so the phone's back
  // gesture closes the story instead of leaving the app.
  useEffect(() => {
    const sync = () => {
      const { period: hashPeriod, wrapped } = parseHash(window.location.hash)
      setWrappedFor(wrapped)
      if (hashPeriod) {
        setPeriod(hashPeriod)
      }
    }
    sync()
    window.addEventListener('popstate', sync)
    window.addEventListener('hashchange', sync)
    return () => {
      window.removeEventListener('popstate', sync)
      window.removeEventListener('hashchange', sync)
    }
  }, [])

  // Switching tabs replaces the entry so it does not fill up the back stack.
  const selectPeriod = useCallback((key: PeriodKey) => {
    setPeriod(key)
    window.history.replaceState(null, '', `#${key}`)
  }, [])

  const openWrapped = useCallback((key: PeriodKey) => {
    window.history.pushState(null, '', `#wrapped=${key}`)
    setWrappedFor(key)
  }, [])

  const closeWrapped = useCallback(() => {
    if (parseHash(window.location.hash).wrapped) {
      window.history.back()
      return
    }
    setWrappedFor(null)
  }, [])

  const active = useMemo(
    () => dashboard?.periods.find((entry) => entry.key === period) ?? null,
    [dashboard, period]
  )

  const leadArtist = active?.artists[0] ?? null

  const wrappedPeriod = useMemo(
    () =>
      wrappedFor ? (dashboard?.periods.find((entry) => entry.key === wrappedFor) ?? null) : null,
    [dashboard, wrappedFor]
  )

  // The signature move: recolour the page from whoever is at no.1.
  useEffect(() => {
    let cancelled = false
    const root = document.documentElement

    if (!leadArtist?.imageUrl) {
      root.style.removeProperty('--accent-h')
      root.style.removeProperty('--accent-s')
      root.style.removeProperty('--accent-l')
      return
    }

    void extractAccent(leadArtist.imageUrl).then((accent) => {
      if (cancelled || !accent) {
        return
      }
      root.style.setProperty('--accent-h', `${accent.hue}`)
      root.style.setProperty('--accent-s', `${accent.saturation}%`)
      root.style.setProperty('--accent-l', `${accent.lightness}%`)
    })

    return () => {
      cancelled = true
    }
  }, [leadArtist?.imageUrl])

  async function handleRefresh() {
    setRefreshing(true)
    setError(null)
    try {
      setDashboard(await refreshDashboard())
    } catch (err) {
      setError(describeError(err))
    } finally {
      setRefreshing(false)
    }
  }

  async function handleDisconnect() {
    try {
      await logout()
      setDashboard(null)
      await bootstrap()
    } catch (err) {
      setError(describeError(err))
    }
  }

  if (loading) {
    return (
      <div className="stage">
        <p className="loading">Reading your listening history</p>
      </div>
    )
  }

  return (
    <div className="stage">
      <Masthead
        capturedAt={dashboard?.capturedAt}
        name={auth?.profile?.displayName}
        authenticated={Boolean(auth?.authenticated)}
        refreshing={refreshing}
        onRefresh={handleRefresh}
        onDisconnect={handleDisconnect}
      />

      {error && <p className="notice notice-error">{error}</p>}

      {!auth?.spotifyConfigured && (
        <Gate
          title="Spotify keys are missing"
          body="Set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET in the server environment, then reload this page."
        />
      )}

      {auth?.spotifyConfigured && !auth.authenticated && (
        <Gate
          title="Connect Spotify to see your year"
          body="RhythmIQ reads your top artists, top tracks, and recent plays. It does not change anything in your account."
          action={
            <a className="button" href="/api/auth/login">
              Connect Spotify
            </a>
          }
        />
      )}

      {auth?.authenticated && !dashboard && (
        <Gate
          title="Nothing pulled yet"
          body="Load your listening data to build the spread."
          action={
            <button className="button" onClick={handleRefresh} disabled={refreshing}>
              {refreshing ? 'Loading' : 'Load my data'}
            </button>
          }
        />
      )}

      {dashboard && active && (
        <main className="spread">
          <Hero period={active} artist={leadArtist} />

          <nav className="periods" aria-label="Time range">
            {PERIOD_ORDER.map((key) => {
              const entry = dashboard.periods.find((item) => item.key === key)
              if (!entry) {
                return null
              }
              return (
                <button
                  key={key}
                  className={`period ${key === period ? 'is-active' : ''}`}
                  aria-current={key === period ? 'true' : undefined}
                  onClick={() => selectPeriod(key)}
                >
                  {entry.label}
                </button>
              )
            })}
          </nav>

          <p className="source">{active.source}</p>

          {active.artists.length > 0 && (
            <button className="wrapped-open" onClick={() => openWrapped(active.key)}>
              <span className="wrapped-open-label">Play your {active.key} wrapped</span>
              <span className="wrapped-open-hint">
                {active.hasTotals && active.totalMinutes > 0
                  ? `${numbers.format(active.totalMinutes)} minutes · tap through`
                  : `${active.artists.length} artists · tap through`}
              </span>
            </button>
          )}

          <Charts period={active} />
          <Highlights period={active} longestRun={active.key === 'week' ? dashboard.longestRun : null} />
          <Genres genres={active.genres} />
          {dashboard.mostReplayed && <OnRepeat replay={dashboard.mostReplayed} />}
          <Decades period={active} />
          <Clock playedAt={dashboard.playedAt} />
          <NewToYou period={active} />
          <Library stats={dashboard.library} />
        </main>
      )}

      {wrappedPeriod && (
        <Wrapped
          period={wrappedPeriod}
          // Both are derived from the recent playback window, so they belong
          // to the week's story only.
          mostReplayed={wrappedPeriod.key === 'week' ? dashboard?.mostReplayed : null}
          longestRun={wrappedPeriod.key === 'week' ? dashboard?.longestRun : null}
          onClose={closeWrapped}
        />
      )}
    </div>
  )
}

function Masthead({
  capturedAt,
  name,
  authenticated,
  refreshing,
  onRefresh,
  onDisconnect
}: {
  capturedAt?: string
  name?: string
  authenticated: boolean
  refreshing: boolean
  onRefresh: () => void
  onDisconnect: () => void
}) {
  return (
    <header className="masthead">
      <p className="wordmark">RhythmIQ</p>
      <div className="masthead-meta">
        {name && <span className="credit">{name}</span>}
        {capturedAt && <span className="credit">Updated {relativeTime(capturedAt)}</span>}
        {authenticated && (
          <>
            <button className="link" onClick={onRefresh} disabled={refreshing}>
              {refreshing ? 'Refreshing' : 'Refresh'}
            </button>
            <button className="link" onClick={onDisconnect}>
              Disconnect
            </button>
          </>
        )}
      </div>
    </header>
  )
}

function Gate({ title, body, action }: { title: string; body: string; action?: React.ReactNode }) {
  return (
    <section className="gate">
      <h1 className="gate-title">{title}</h1>
      <p className="gate-body">{body}</p>
      {action}
    </section>
  )
}

function Hero({ period, artist }: { period: PeriodMetrics; artist: ArtistStat | null }) {
  if (!artist) {
    return (
      <section className="hero hero-empty">
        <p className="eyebrow">{period.label}</p>
        <h1 className="hero-name">No plays yet</h1>
        <p className="hero-meta">Once Spotify has something to report, this is where your no.1 lands.</p>
      </section>
    )
  }

  const [first, ...rest] = artist.name.split(' ')

  return (
    <section className="hero">
      {artist.imageUrl && (
        <div className="hero-frame">
          {/* Spotify caps artist art at 640px, so the full-bleed layer is
              blurred rather than upscaled sharp — softness reads as intent. */}
          <img className="hero-backdrop" src={artist.imageUrl} alt="" aria-hidden="true" loading="eager" />
          <div className="hero-tint" />
          <div className="hero-scrim" />
        </div>
      )}
      <div className="hero-inner">
        <div className="hero-copy">
          <p className="eyebrow">Your no.1 &mdash; {period.label.toLowerCase()}</p>
          <h1 className="hero-name">
            <span>{first}</span>
            {rest.length > 0 && <span>{rest.join(' ')}</span>}
          </h1>
          <p className="hero-meta">{describeLead(period, artist)}</p>
          {artist.externalUrl && (
            <a className="hero-link" href={artist.externalUrl} target="_blank" rel="noopener noreferrer">
              Open in Spotify
            </a>
          )}
        </div>
        {artist.imageUrl && (
          <figure className="hero-portrait">
            {/* Displayed at or below its native size, so it stays sharp. */}
            <img src={artist.imageUrl} alt={artist.name} loading="eager" />
          </figure>
        )}
      </div>
    </section>
  )
}

function Charts({ period }: { period: PeriodMetrics }) {
  return (
    <section className="charts">
      <article className="chart">
        <h2 className="rubric">Top artists</h2>
        <ol className="rank-list">
          {period.artists.map((artist) => (
            <li key={artist.id}>
              <span className="rank">{pad(artist.rank)}</span>
              <a
                className="rank-name"
                href={artist.externalUrl || undefined}
                target="_blank"
                rel="noopener noreferrer"
              >
                {artist.name}
              </a>
              {artist.plays > 0 && <span className="rank-value">{artist.plays} plays</span>}
            </li>
          ))}
        </ol>
      </article>

      <article className="chart">
        <h2 className="rubric">Top tracks</h2>
        <ol className="rank-list">
          {period.tracks.map((track) => (
            <li key={track.id}>
              <span className="rank">{pad(track.rank)}</span>
              <a
                className="rank-name"
                href={track.externalUrl || undefined}
                target="_blank"
                rel="noopener noreferrer"
              >
                {track.name}
                <small>{track.artists.join(', ')}</small>
              </a>
              <span className="rank-value">
                {track.plays > 0 ? `${track.plays} plays` : formatDuration(track.durationMs)}
              </span>
            </li>
          ))}
        </ol>
      </article>
    </section>
  )
}

function Highlights({ period, longestRun }: { period: PeriodMetrics; longestRun: ListeningRun | null }) {
  const album = period.topAlbum
  const hasAnything = album || period.deepCut || longestRun || period.distinctArtists > 0
  if (!hasAnything) {
    return null
  }

  return (
    <section className="highlights">
      <h2 className="rubric">The details</h2>
      <div className="highlight-grid">
        {album && (
          <article className="highlight highlight-album">
            {album.imageUrl && <img src={album.imageUrl} alt="" loading="lazy" />}
            <div>
              <p className="highlight-label">Album you returned to</p>
              <h3>{album.name}</h3>
              <p className="highlight-detail">
                {album.artist}
                {album.releaseYear > 0 && ` · ${album.releaseYear}`}
              </p>
              <p className="highlight-detail">
                {album.trackCount} of your top {album.trackTotal} songs
              </p>
            </div>
          </article>
        )}

        {period.deepCut && (
          <article className="highlight">
            <p className="highlight-label">Deep cut</p>
            <h3>{period.deepCut.name}</h3>
            <p className="highlight-detail">The least widely known artist in your top 10.</p>
          </article>
        )}

        {longestRun && (
          <article className="highlight">
            <p className="highlight-label">Longest run</p>
            <h3>{longestRun.minutes} min</h3>
            <p className="highlight-detail">
              {longestRun.tracks} songs back to back, starting {formatRunStart(longestRun.startedAt)}.
            </p>
          </article>
        )}

        {period.distinctArtists > 0 && (
          <article className="highlight">
            <p className="highlight-label">Range</p>
            <h3>{numbers.format(period.distinctArtists)} artists</h3>
            <p className="highlight-detail">
              {period.distinctAlbums > 0
                ? `across ${numbers.format(period.distinctAlbums)} albums`
                : 'in this window'}
            </p>
          </article>
        )}
      </div>
    </section>
  )
}

function Decades({ period }: { period: PeriodMetrics }) {
  const decades = period.decades.filter((entry) => entry.count > 0)
  if (decades.length < 2) {
    return null
  }

  const peak = Math.max(...decades.map((entry) => entry.count))
  const first = decades[0].decade
  const last = decades[decades.length - 1].decade

  return (
    <section className="decades">
      <h2 className="rubric">Decades</h2>
      <p className="decades-lede">
        Your {period.label.toLowerCase().replace('this ', '')} spans <strong>{first}s</strong> to{' '}
        <strong>{last}s</strong>, across {decades.length} decades.
      </p>
      <div className="decade-row">
        {decades.map((entry) => (
          <div className="decade" key={entry.decade}>
            <div className="decade-track">
              <div className="decade-fill" style={{ height: `${(entry.count / peak) * 100}%` }} />
            </div>
            <span className="decade-name">{`${String(entry.decade).slice(2)}s`}</span>
            <span className="decade-count">{entry.count}</span>
          </div>
        ))}
      </div>
    </section>
  )
}

function Genres({ genres }: { genres: GenreStat[] }) {
  if (genres.length === 0) {
    return null
  }

  const total = genres.reduce((sum, entry) => sum + entry.share, 0)

  return (
    <section className="genres">
      <h2 className="rubric">Genres</h2>
      <div className="genre-band" role="img" aria-label={genres.map((g) => `${g.genre} ${g.share}%`).join(', ')}>
        {genres.map((entry, index) => (
          <div
            key={entry.genre}
            className="genre-slice"
            style={
              {
                flexGrow: entry.share,
                '--step': index
              } as React.CSSProperties
            }
          />
        ))}
      </div>
      <ul className="genre-key">
        {genres.map((entry) => (
          <li key={entry.genre}>
            <span className="genre-name">{entry.genre}</span>
            <span className="genre-share">{Math.round((entry.share / total) * 100)}%</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function OnRepeat({ replay }: { replay: ReplayStat }) {
  return (
    <section className="repeat">
      <h2 className="rubric">On repeat</h2>
      <div className="repeat-body">
        {replay.track.albumImageUrl && (
          <img className="repeat-art" src={replay.track.albumImageUrl} alt="" loading="lazy" />
        )}
        <div>
          <blockquote className="repeat-title">{replay.track.name}</blockquote>
          <p className="repeat-credit">{replay.track.artists.join(', ')}</p>
          <p className="repeat-count">
            <strong>{replay.plays}&times;</strong> in your last 50 plays
          </p>
          {replay.track.externalUrl && (
            <a
              className="hero-link"
              href={replay.track.externalUrl}
              target="_blank"
              rel="noopener noreferrer"
            >
              Open in Spotify
            </a>
          )}
        </div>
      </div>
    </section>
  )
}

function Clock({ playedAt }: { playedAt: string[] }) {
  const { hours, weekdays, peakHour, peakDay } = useMemo(() => {
    const hourCounts = new Array<number>(24).fill(0)
    const dayCounts = new Array<number>(7).fill(0)

    // Bucketed in the viewer's own timezone, not the server's.
    for (const stamp of playedAt) {
      const date = new Date(stamp)
      if (Number.isNaN(date.getTime())) {
        continue
      }
      hourCounts[date.getHours()] += 1
      dayCounts[date.getDay()] += 1
    }

    return {
      hours: hourCounts,
      weekdays: dayCounts,
      peakHour: indexOfMax(hourCounts),
      peakDay: indexOfMax(dayCounts)
    }
  }, [playedAt])

  if (playedAt.length === 0) {
    return null
  }

  const peak = Math.max(...hours, 1)
  const width = 960
  const height = 150
  const step = width / 23

  const points = hours.map((count, hour) => ({
    x: hour * step,
    y: height - (count / peak) * (height - 12)
  }))
  const line = smoothPath(points)
  const area = `${line} L ${width} ${height} L 0 ${height} Z`

  const dayPeak = Math.max(...weekdays, 1)

  return (
    <section className="clock">
      <h2 className="rubric">When you listened</h2>
      <p className="clock-lede">
        Your peak is <strong>{formatHour(peakHour)}</strong> on <strong>{WEEKDAYS[peakDay]}</strong>.
      </p>

      <svg className="ridge" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
        <path className="ridge-area" d={area} />
        <path className="ridge-line" d={line} />
      </svg>
      <div className="ridge-axis">
        {[0, 6, 12, 18, 23].map((hour) => (
          <span key={hour}>{formatHour(hour)}</span>
        ))}
      </div>

      <div className="weekdays">
        {weekdays.map((count, index) => (
          <div className="weekday" key={WEEKDAYS[index]}>
            <div className="weekday-track">
              <div className="weekday-fill" style={{ height: `${(count / dayPeak) * 100}%` }} />
            </div>
            <span>{WEEKDAYS[index].slice(0, 1)}</span>
          </div>
        ))}
      </div>
    </section>
  )
}

function NewToYou({ period }: { period: PeriodMetrics }) {
  if (period.newArtists.length === 0) {
    return null
  }

  return (
    <section className="discoveries">
      <h2 className="rubric">New to you</h2>
      <p className="discoveries-lede">
        In {period.label.toLowerCase()} but not in your year so far.
      </p>
      <ul className="discovery-list">
        {period.newArtists.map((artist) => (
          <li key={artist.id}>
            <a href={artist.externalUrl || undefined} target="_blank" rel="noopener noreferrer">
              {artist.name}
            </a>
            {artist.genres.length > 0 && <small>{artist.genres[0]}</small>}
          </li>
        ))}
      </ul>
    </section>
  )
}

function Library({ stats }: { stats: { savedTracks: number; playlists: number; following: number } }) {
  return (
    <section className="library">
      <h2 className="rubric">In your library</h2>
      <dl className="library-stats">
        <div>
          <dt>Saved tracks</dt>
          <dd>{numbers.format(stats.savedTracks)}</dd>
        </div>
        <div>
          <dt>Playlists</dt>
          <dd>{numbers.format(stats.playlists)}</dd>
        </div>
        <div>
          <dt>Artists followed</dt>
          <dd>{numbers.format(stats.following)}</dd>
        </div>
      </dl>
    </section>
  )
}

function describeLead(period: PeriodMetrics, artist: ArtistStat) {
  if (period.hasTotals && artist.plays > 0) {
    const minutes = period.totalMinutes > 0 ? `, ${numbers.format(period.totalMinutes)} minutes in total` : ''
    return `${artist.plays} of your last ${period.totalPlays} plays${minutes}.`
  }
  if (artist.genres.length > 0) {
    return `Leading ${period.label.toLowerCase()} — ${artist.genres.slice(0, 2).join(', ')}.`
  }
  return `Leading ${period.label.toLowerCase()}.`
}

function smoothPath(points: { x: number; y: number }[]) {
  if (points.length === 0) {
    return ''
  }
  let path = `M ${points[0].x} ${points[0].y}`
  for (let i = 0; i < points.length - 1; i += 1) {
    const current = points[i]
    const next = points[i + 1]
    const midX = (current.x + next.x) / 2
    path += ` C ${midX} ${current.y}, ${midX} ${next.y}, ${next.x} ${next.y}`
  }
  return path
}

function indexOfMax(values: number[]) {
  let best = 0
  for (let i = 1; i < values.length; i += 1) {
    if (values[i] > values[best]) {
      best = i
    }
  }
  return best
}

function pad(value: number) {
  return value < 10 ? `0${value}` : `${value}`
}

function formatHour(hour: number) {
  const normalized = ((hour % 24) + 24) % 24
  const suffix = normalized < 12 ? 'am' : 'pm'
  const display = normalized % 12 === 0 ? 12 : normalized % 12
  return `${display}${suffix}`
}

function formatRunStart(iso: string) {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) {
    return 'recently'
  }
  return date
    .toLocaleString(undefined, { weekday: 'long', hour: 'numeric', minute: '2-digit' })
    .toLowerCase()
}

function formatDuration(ms: number) {
  const totalSeconds = Math.round(ms / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}:${seconds < 10 ? '0' : ''}${seconds}`
}

function relativeTime(iso: string) {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) {
    return 'just now'
  }
  const minutes = Math.round((Date.now() - then) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

function parseHash(hash: string): { period: PeriodKey | null; wrapped: PeriodKey | null } {
  const story = /^#wrapped=(week|month|year)$/.exec(hash)
  if (story) {
    const key = story[1] as PeriodKey
    return { period: key, wrapped: key }
  }
  const tab = /^#(week|month|year)$/.exec(hash)
  return { period: tab ? (tab[1] as PeriodKey) : null, wrapped: null }
}

function describeError(err: unknown) {
  return err instanceof Error ? err.message : 'Something went wrong.'
}

export default App
