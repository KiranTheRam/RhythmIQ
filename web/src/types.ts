export type TimeRange = 'short_term' | 'medium_term' | 'long_term'

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

export interface TrackSummary {
  id: string
  name: string
  artists: string[]
  album: string
  durationMs: number
  popularity: number
  externalUrl: string
}

export interface ArtistSummary {
  id: string
  name: string
  genres: string[]
  popularity: number
  followers: number
  externalUrl: string
}

export interface GenreWeight {
  genre: string
  weight: number
}

export interface SnapshotStats {
  estimatedDailyMinutes: number
  estimatedYearMinutes: number
  uniqueArtistCount: number
  uniqueGenreCount: number
  consistencyScore: number
  discoveryScore: number
  replayScore: number
  varietyScore: number
  sessionCount: number
  averageSessionMinutes: number
  averageTrackMinutes: number
  weekendListeningShare: number
  nightOwlScore: number
  peakListeningHour: number
  topTrackConcentration: number
  listeningByDaypart: Record<string, number>
  listeningByWeekday: Record<string, number>
  topGenres: GenreWeight[]
  topArtistMinutesYtd: ArtistMinuteStat[]
  topArtistMinutesAllTime: ArtistMinuteStat[]
  moodVector: Record<string, number>
}

export interface ArtistMinuteStat {
  name: string
  minutes: number
  externalUrl: string
}

export interface PlaybackEvent {
  playedAt: string
  track: TrackSummary
}

export interface MetricSnapshot {
  id: number
  userId: string
  capturedAt: string
  topTracks: Record<TimeRange, TrackSummary[]>
  topArtists: Record<TimeRange, ArtistSummary[]>
  recentlyPlayed: PlaybackEvent[]
  savedTrackCount: number
  playlistCount: number
  followingCount: number
  stats: SnapshotStats
}

export interface SnapshotPoint {
  capturedAt: string
  estimatedDailyMinutes: number
  uniqueArtistCount: number
  uniqueGenreCount: number
  consistencyScore: number
  discoveryScore: number
  replayScore: number
  varietyScore: number
  sessionCount: number
  averageSessionMinutes: number
  weekendListeningShare: number
  nightOwlScore: number
  topTrackConcentration: number
}

export interface HistoryResponse {
  days: number
  points: SnapshotPoint[]
}

export interface Recommendation {
  title: string
  description: string
  confidence: string
  type: string
}

export interface GenreRecommendation {
  genre: string
  reason: string
  confidence: string
}

export interface ArtistRecommendation {
  name: string
  reason: string
  externalUrl: string
  confidence: string
}

export interface SongRecommendation {
  track: string
  artist: string
  reason: string
  externalUrl: string
  confidence: string
}

export interface InsightResponse {
  generatedAt: string
  narrative: string
  recommendations: Recommendation[]
  genreRecommendations: GenreRecommendation[]
  artistRecommendations: ArtistRecommendation[]
  songRecommendations: SongRecommendation[]
  openAIGenerated: boolean
}
