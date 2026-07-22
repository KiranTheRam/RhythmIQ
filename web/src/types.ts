export interface UserProfile {
  id: string
  displayName: string
  country: string
  product: string
  avatarUrl: string
}

export interface AuthStatus {
  authenticated: boolean
  spotifyConfigured: boolean
  profile?: UserProfile
}

export interface ArtistStat {
  rank: number
  id: string
  name: string
  genres: string[]
  imageUrl: string
  plays: number
  popularity: number
  externalUrl: string
}

export interface TrackStat {
  rank: number
  id: string
  name: string
  artists: string[]
  album: string
  albumImageUrl: string
  durationMs: number
  plays: number
  externalUrl: string
}

export interface GenreStat {
  genre: string
  share: number
}

export interface AlbumStat {
  name: string
  artist: string
  imageUrl: string
  releaseYear: number
  trackCount: number
  trackTotal: number
  externalUrl: string
}

export interface DecadeStat {
  decade: number
  count: number
}

export interface ListeningRun {
  minutes: number
  tracks: number
  startedAt: string
}

export type PeriodKey = 'week' | 'month' | 'year'

export interface PeriodMetrics {
  key: PeriodKey
  label: string
  source: string
  artists: ArtistStat[]
  tracks: TrackStat[]
  genres: GenreStat[]
  newArtists: ArtistStat[]
  topAlbum: AlbumStat | null
  decades: DecadeStat[]
  deepCut: ArtistStat | null
  distinctArtists: number
  distinctAlbums: number
  totalPlays: number
  totalMinutes: number
  hasTotals: boolean
}

export interface ReplayStat {
  track: TrackStat
  plays: number
}

export interface LibraryStats {
  savedTracks: number
  playlists: number
  following: number
}

export interface Dashboard {
  userId: string
  profile: UserProfile
  capturedAt: string
  periods: PeriodMetrics[]
  mostReplayed: ReplayStat | null
  longestRun: ListeningRun | null
  library: LibraryStats
  playedAt: string[]
}
