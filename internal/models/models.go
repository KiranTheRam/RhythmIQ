package models

import "time"

// SpotifyToken stores auth credentials for a user.
type SpotifyToken struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	TokenType    string    `json:"tokenType"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// UserProfile captures relevant fields from Spotify's /me endpoint.
type UserProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Country     string `json:"country"`
	Product     string `json:"product"`
	AvatarURL   string `json:"avatarUrl"`
}

// TrackSummary is a compact representation used across snapshots.
type TrackSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	DurationMS  int      `json:"durationMs"`
	Popularity  int      `json:"popularity"`
	ExternalURL string   `json:"externalUrl"`
}

// ArtistSummary is a compact representation used across snapshots.
type ArtistSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Genres      []string `json:"genres"`
	Popularity  int      `json:"popularity"`
	Followers   int      `json:"followers"`
	ExternalURL string   `json:"externalUrl"`
}

// PlaybackEvent captures a recently played item.
type PlaybackEvent struct {
	PlayedAt time.Time    `json:"playedAt"`
	Track    TrackSummary `json:"track"`
}

// SnapshotStats are computed metrics for fast dashboard rendering.
type SnapshotStats struct {
	EstimatedDailyMinutes int                `json:"estimatedDailyMinutes"`
	EstimatedYearMinutes  int                `json:"estimatedYearMinutes"`
	UniqueArtistCount     int                `json:"uniqueArtistCount"`
	UniqueGenreCount      int                `json:"uniqueGenreCount"`
	ConsistencyScore      float64            `json:"consistencyScore"`
	DiscoveryScore        float64            `json:"discoveryScore"`
	ReplayScore           float64            `json:"replayScore"`
	VarietyScore          float64            `json:"varietyScore"`
	SessionCount          int                `json:"sessionCount"`
	AverageSessionMinutes float64            `json:"averageSessionMinutes"`
	AverageTrackMinutes   float64            `json:"averageTrackMinutes"`
	WeekendListeningShare float64            `json:"weekendListeningShare"`
	NightOwlScore         float64            `json:"nightOwlScore"`
	PeakListeningHour     int                `json:"peakListeningHour"`
	TopTrackConcentration float64            `json:"topTrackConcentration"`
	ListeningByDaypart    map[string]float64 `json:"listeningByDaypart"`
	ListeningByWeekday    map[string]float64 `json:"listeningByWeekday"`
	TopGenres             []GenreWeight      `json:"topGenres"`
	TopArtistMinutesYTD   []ArtistMinuteStat `json:"topArtistMinutesYtd"`
	TopArtistMinutesAll   []ArtistMinuteStat `json:"topArtistMinutesAllTime"`
	MoodVector            map[string]float64 `json:"moodVector"`
}

// ArtistMinuteStat captures estimated listening minutes for an artist over a period.
type ArtistMinuteStat struct {
	Name        string `json:"name"`
	Minutes     int    `json:"minutes"`
	ExternalURL string `json:"externalUrl"`
}

// GenreWeight ranks a genre by computed weight.
type GenreWeight struct {
	Genre  string  `json:"genre"`
	Weight float64 `json:"weight"`
}

// MetricSnapshot is the persisted analytics payload.
type MetricSnapshot struct {
	ID              int64                      `json:"id"`
	UserID          string                     `json:"userId"`
	CapturedAt      time.Time                  `json:"capturedAt"`
	TopTracks       map[string][]TrackSummary  `json:"topTracks"`
	TopArtists      map[string][]ArtistSummary `json:"topArtists"`
	RecentlyPlayed  []PlaybackEvent            `json:"recentlyPlayed"`
	SavedTrackCount int                        `json:"savedTrackCount"`
	PlaylistCount   int                        `json:"playlistCount"`
	FollowingCount  int                        `json:"followingCount"`
	Stats           SnapshotStats              `json:"stats"`
}

// SnapshotPoint is a trimmed representation for trend charts.
type SnapshotPoint struct {
	CapturedAt            time.Time `json:"capturedAt"`
	EstimatedDailyMinutes int       `json:"estimatedDailyMinutes"`
	UniqueArtistCount     int       `json:"uniqueArtistCount"`
	UniqueGenreCount      int       `json:"uniqueGenreCount"`
	ConsistencyScore      float64   `json:"consistencyScore"`
	DiscoveryScore        float64   `json:"discoveryScore"`
	ReplayScore           float64   `json:"replayScore"`
	VarietyScore          float64   `json:"varietyScore"`
	SessionCount          int       `json:"sessionCount"`
	AverageSessionMinutes float64   `json:"averageSessionMinutes"`
	WeekendListeningShare float64   `json:"weekendListeningShare"`
	NightOwlScore         float64   `json:"nightOwlScore"`
	TopTrackConcentration float64   `json:"topTrackConcentration"`
}

// Recommendation describes a personalized user suggestion.
type Recommendation struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Confidence  string `json:"confidence"`
	Type        string `json:"type"`
}

// GenreRecommendation suggests a genre direction.
type GenreRecommendation struct {
	Genre      string `json:"genre"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// ArtistRecommendation suggests an artist to explore.
type ArtistRecommendation struct {
	Name        string `json:"name"`
	Reason      string `json:"reason"`
	ExternalURL string `json:"externalUrl"`
	Confidence  string `json:"confidence"`
}

// SongRecommendation suggests a track to queue next.
type SongRecommendation struct {
	Track       string `json:"track"`
	Artist      string `json:"artist"`
	Reason      string `json:"reason"`
	ExternalURL string `json:"externalUrl"`
	Confidence  string `json:"confidence"`
}

// InsightResponse returns recommendation content to the UI.
type InsightResponse struct {
	GeneratedAt           time.Time              `json:"generatedAt"`
	Narrative             string                 `json:"narrative"`
	Recommendations       []Recommendation       `json:"recommendations"`
	GenreRecommendations  []GenreRecommendation  `json:"genreRecommendations"`
	ArtistRecommendations []ArtistRecommendation `json:"artistRecommendations"`
	SongRecommendations   []SongRecommendation   `json:"songRecommendations"`
	OpenAIGenerated       bool                   `json:"openAIGenerated"`
}
