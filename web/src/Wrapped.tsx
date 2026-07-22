import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ListeningRun, PeriodMetrics, ReplayStat } from './types'

const SLIDE_MS = 5500
const numbers = new Intl.NumberFormat('en-US')

interface Slide {
  key: string
  /** Hue offset from the extracted accent, so each card reads distinctly. */
  tone: number
  node: React.ReactNode
}

interface WrappedProps {
  period: PeriodMetrics
  /** Only meaningful for the week, which is built from real playback events. */
  mostReplayed?: ReplayStat | null
  longestRun?: ListeningRun | null
  onClose: () => void
}

export function Wrapped({ period, mostReplayed, longestRun, onClose }: WrappedProps) {
  const slides = useMemo(
    () => buildSlides(period, mostReplayed ?? null, longestRun ?? null),
    [period, mostReplayed, longestRun]
  )
  const [index, setIndex] = useState(0)
  const [progress, setProgress] = useState(0)
  const [paused, setPaused] = useState(false)

  const reducedMotion = useMemo(
    () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false,
    []
  )

  const next = useCallback(() => {
    setIndex((current) => {
      if (current >= slides.length - 1) {
        onClose()
        return current
      }
      return current + 1
    })
    setProgress(0)
  }, [slides.length, onClose])

  const previous = useCallback(() => {
    setIndex((current) => Math.max(0, current - 1))
    setProgress(0)
  }, [])

  // Drive the progress bar and auto-advance from one clock so they can never
  // drift apart. Holding pauses both.
  useEffect(() => {
    if (reducedMotion) {
      return
    }

    let frame = 0
    let elapsed = 0
    let last = performance.now()

    const tick = (now: number) => {
      const delta = now - last
      last = now
      if (!paused) {
        elapsed += delta
        const ratio = Math.min(1, elapsed / SLIDE_MS)
        setProgress(ratio)
        if (ratio >= 1) {
          next()
          return
        }
      }
      frame = requestAnimationFrame(tick)
    }

    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [index, paused, next, reducedMotion])

  // Lock the page behind the story so it cannot scroll under it.
  useEffect(() => {
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previousOverflow
    }
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
      if (event.key === 'ArrowRight') next()
      if (event.key === 'ArrowLeft') previous()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [next, previous, onClose])

  const touchStart = useRef<{ x: number; y: number } | null>(null)

  function handleTouchStart(event: React.TouchEvent) {
    const touch = event.touches[0]
    touchStart.current = { x: touch.clientX, y: touch.clientY }
    setPaused(true)
  }

  function handleTouchEnd(event: React.TouchEvent) {
    setPaused(false)
    const start = touchStart.current
    touchStart.current = null
    if (!start) return

    const touch = event.changedTouches[0]
    const dx = touch.clientX - start.x
    const dy = touch.clientY - start.y

    // A decisive downward swipe dismisses, matching the gesture people expect
    // from a full-screen story.
    if (dy > 90 && Math.abs(dy) > Math.abs(dx)) {
      onClose()
      return
    }
    if (Math.abs(dx) > 60) {
      dx < 0 ? next() : previous()
    }
  }

  const slide = slides[index]

  return (
    <div
      className="wrapped"
      style={{ '--tone': slide.tone } as React.CSSProperties}
      role="dialog"
      aria-modal="true"
      aria-label={`${period.label} wrapped`}
    >
      <div className="wrapped-card">
      <div className="wrapped-progress">
        {slides.map((entry, position) => (
          <span key={entry.key} className="wrapped-progress-track">
            <span
              className="wrapped-progress-fill"
              style={{
                transform: `scaleX(${
                  position < index ? 1 : position === index ? (reducedMotion ? 1 : progress) : 0
                })`
              }}
            />
          </span>
        ))}
      </div>

      <header className="wrapped-bar">
        <span className="wrapped-label">{period.label} &middot; Wrapped</span>
        <button className="wrapped-close" onClick={onClose} aria-label="Close">
          &times;
        </button>
      </header>

      <div
        className="wrapped-stage"
        onPointerDown={() => setPaused(true)}
        onPointerUp={() => setPaused(false)}
        onPointerCancel={() => setPaused(false)}
        onTouchStart={handleTouchStart}
        onTouchEnd={handleTouchEnd}
      >
        <div key={slide.key} className="wrapped-slide">
          {slide.node}
        </div>
      </div>

      <button className="wrapped-zone wrapped-zone-prev" onClick={previous} aria-label="Previous" />
      <button className="wrapped-zone wrapped-zone-next" onClick={next} aria-label="Next" />

      <p className="wrapped-hint">
        {index + 1} / {slides.length}
      </p>
      </div>
    </div>
  )
}

function buildSlides(
  period: PeriodMetrics,
  mostReplayed: ReplayStat | null,
  longestRun: ListeningRun | null
): Slide[] {
  const slides: Slide[] = []
  const window = period.label.toLowerCase().replace('this ', '')
  const topArtist = period.artists[0]
  const topTrack = period.tracks[0]
  const topGenre = period.genres[0]

  slides.push({
    key: 'intro',
    tone: 0,
    node: (
      <div className="wr-center">
        <p className="wr-eyebrow">Your {window}</p>
        <h2 className="wr-huge">
          <span>Wrapped</span>
        </h2>
        <p className="wr-body">Everything you played, counted up.</p>
      </div>
    )
  })

  if (period.hasTotals && period.totalMinutes > 0) {
    slides.push({
      key: 'minutes',
      tone: 25,
      node: (
        <div className="wr-center">
          <p className="wr-eyebrow">You listened for</p>
          <p className="wr-figure">{numbers.format(period.totalMinutes)}</p>
          <h2 className="wr-mid">minutes</h2>
          <p className="wr-body">across {numbers.format(period.totalPlays)} plays</p>
        </div>
      )
    })
  }

  if (period.distinctArtists > 0) {
    slides.push({
      key: 'counts',
      tone: 55,
      node: (
        <div className="wr-center">
          <p className="wr-eyebrow">You listened to</p>
          <p className="wr-figure">{numbers.format(period.distinctArtists)}</p>
          <h2 className="wr-mid">different artists</h2>
          {period.distinctAlbums > 0 && (
            <p className="wr-body">across {numbers.format(period.distinctAlbums)} albums</p>
          )}
        </div>
      )
    })
  }

  if (topArtist) {
    slides.push({
      key: 'top-artist',
      tone: 80,
      node: (
        <div className="wr-hero">
          {topArtist.imageUrl && <img className="wr-portrait" src={topArtist.imageUrl} alt="" />}
          <p className="wr-eyebrow">Your no.1 artist</p>
          <h2 className="wr-big">{topArtist.name}</h2>
          {topArtist.plays > 0 && <p className="wr-body">{topArtist.plays} plays</p>}
        </div>
      )
    })
  }

  if (period.artists.length > 1) {
    slides.push({
      key: 'artist-list',
      tone: 120,
      node: (
        <div className="wr-list-slide">
          <p className="wr-eyebrow">Your top artists</p>
          <ol className="wr-list">
            {period.artists.slice(0, 5).map((artist, position) => (
              <li key={artist.id} style={{ animationDelay: `${position * 110 + 150}ms` }}>
                <span>{position + 1}</span>
                {artist.name}
              </li>
            ))}
          </ol>
        </div>
      )
    })
  }

  if (topTrack) {
    slides.push({
      key: 'top-track',
      tone: 165,
      node: (
        <div className="wr-hero">
          {topTrack.albumImageUrl && <img className="wr-art" src={topTrack.albumImageUrl} alt="" />}
          <p className="wr-eyebrow">Your no.1 song</p>
          <h2 className="wr-big">{topTrack.name}</h2>
          <p className="wr-body">{topTrack.artists.join(', ')}</p>
        </div>
      )
    })
  }

  if (period.tracks.length > 1) {
    slides.push({
      key: 'track-list',
      tone: 205,
      node: (
        <div className="wr-list-slide">
          <p className="wr-eyebrow">Your top songs</p>
          <ol className="wr-list">
            {period.tracks.slice(0, 5).map((track, position) => (
              <li key={track.id} style={{ animationDelay: `${position * 110 + 150}ms` }}>
                <span>{position + 1}</span>
                {track.name}
              </li>
            ))}
          </ol>
        </div>
      )
    })
  }

  if (mostReplayed) {
    slides.push({
      key: 'on-repeat',
      tone: 215,
      node: (
        <div className="wr-hero">
          {mostReplayed.track.albumImageUrl && (
            <img className="wr-art" src={mostReplayed.track.albumImageUrl} alt="" />
          )}
          <p className="wr-eyebrow">On repeat</p>
          <h2 className="wr-big">{mostReplayed.track.name}</h2>
          <p className="wr-body">
            You played it {mostReplayed.plays} times.
          </p>
        </div>
      )
    })
  }

  if (longestRun) {
    slides.push({
      key: 'longest-run',
      tone: 228,
      node: (
        <div className="wr-center">
          <p className="wr-eyebrow">Your longest run</p>
          <p className="wr-figure">{numbers.format(longestRun.minutes)}</p>
          <h2 className="wr-mid">minutes straight</h2>
          <p className="wr-body">
            {longestRun.tracks} songs back to back, starting {formatRunStart(longestRun.startedAt)}.
          </p>
        </div>
      )
    })
  }

  if (period.topAlbum) {
    const album = period.topAlbum
    slides.push({
      key: 'album',
      tone: 240,
      node: (
        <div className="wr-hero">
          {album.imageUrl && <img className="wr-art" src={album.imageUrl} alt="" />}
          <p className="wr-eyebrow">The album you kept returning to</p>
          <h2 className="wr-big">{album.name}</h2>
          <p className="wr-body">
            {album.artist} &middot; {album.trackCount} of your top {album.trackTotal} songs
          </p>
        </div>
      )
    })
  }

  if (topGenre) {
    slides.push({
      key: 'genre',
      tone: 275,
      node: (
        <div className="wr-center">
          <p className="wr-eyebrow">Your sound</p>
          <h2 className="wr-big wr-lower">{topGenre.genre}</h2>
          <ul className="wr-chips">
            {period.genres.slice(1, 5).map((genre, position) => (
              <li key={genre.genre} style={{ animationDelay: `${position * 90 + 250}ms` }}>
                {genre.genre}
              </li>
            ))}
          </ul>
        </div>
      )
    })
  }

  const decades = period.decades.filter((entry) => entry.count > 0)
  if (decades.length > 1) {
    const peak = Math.max(...decades.map((entry) => entry.count))
    slides.push({
      key: 'decades',
      tone: 310,
      node: (
        <div className="wr-center">
          <p className="wr-eyebrow">You played music from</p>
          <p className="wr-figure">{decades.length}</p>
          <h2 className="wr-mid">decades</h2>
          <div className="wr-decades">
            {decades.map((entry, position) => (
              <div key={entry.decade} style={{ animationDelay: `${position * 90 + 250}ms` }}>
                <div className="wr-decade-bar" style={{ height: `${(entry.count / peak) * 100}%` }} />
                <span>{`${String(entry.decade).slice(2)}s`}</span>
              </div>
            ))}
          </div>
        </div>
      )
    })
  }

  if (period.deepCut) {
    slides.push({
      key: 'deep-cut',
      tone: 340,
      node: (
        <div className="wr-hero">
          {period.deepCut.imageUrl && (
            <img className="wr-portrait" src={period.deepCut.imageUrl} alt="" />
          )}
          <p className="wr-eyebrow">Your deep cut</p>
          <h2 className="wr-big">{period.deepCut.name}</h2>
          <p className="wr-body">The least widely known artist in your top 10.</p>
        </div>
      )
    })
  }

  slides.push({
    key: 'outro',
    tone: 20,
    node: (
      <div className="wr-card-slide">
        <p className="wr-eyebrow">Your {window}, in short</p>
        <dl className="wr-card">
          {topArtist && (
            <div>
              <dt>Artist</dt>
              <dd>{topArtist.name}</dd>
            </div>
          )}
          {topTrack && (
            <div>
              <dt>Song</dt>
              <dd>{topTrack.name}</dd>
            </div>
          )}
          {period.topAlbum && (
            <div>
              <dt>Album</dt>
              <dd>{period.topAlbum.name}</dd>
            </div>
          )}
          {topGenre && (
            <div>
              <dt>Genre</dt>
              <dd>{topGenre.genre}</dd>
            </div>
          )}
        </dl>
      </div>
    )
  })

  return slides
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
