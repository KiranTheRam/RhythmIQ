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

// TrackSummary is a compact representation of a Spotify track.
type TrackSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Artists       []string `json:"artists"`
	ArtistIDs     []string `json:"artistIds"`
	Album         string   `json:"album"`
	AlbumID       string   `json:"albumId"`
	AlbumImageURL string   `json:"albumImageUrl"`
	AlbumURL      string   `json:"albumUrl"`
	ReleaseYear   int      `json:"releaseYear"`
	DurationMS    int      `json:"durationMs"`
	Popularity    int      `json:"popularity"`
	ExternalURL   string   `json:"externalUrl"`
}

// ArtistSummary is a compact representation of a Spotify artist.
type ArtistSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Genres      []string `json:"genres"`
	ImageURL    string   `json:"imageUrl"`
	Popularity  int      `json:"popularity"`
	Followers   int      `json:"followers"`
	ExternalURL string   `json:"externalUrl"`
}

// PlaybackEvent captures a single recently played item.
type PlaybackEvent struct {
	PlayedAt time.Time    `json:"playedAt"`
	Track    TrackSummary `json:"track"`
}

// ArtistStat is a ranked artist entry within a period.
type ArtistStat struct {
	Rank        int      `json:"rank"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Genres      []string `json:"genres"`
	ImageURL    string   `json:"imageUrl"`
	Plays       int      `json:"plays"`
	Popularity  int      `json:"popularity"`
	ExternalURL string   `json:"externalUrl"`
}

// TrackStat is a ranked track entry within a period.
type TrackStat struct {
	Rank          int      `json:"rank"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Artists       []string `json:"artists"`
	Album         string   `json:"album"`
	AlbumImageURL string   `json:"albumImageUrl"`
	DurationMS    int      `json:"durationMs"`
	Plays         int      `json:"plays"`
	ExternalURL   string   `json:"externalUrl"`
}

// GenreStat is a genre and its share of a period's listening.
type GenreStat struct {
	Genre string  `json:"genre"`
	Share float64 `json:"share"`
}

// AlbumStat is the album that the most of your top tracks come from.
// TrackCount is a plain count of those tracks, out of TrackTotal considered.
type AlbumStat struct {
	Name        string `json:"name"`
	Artist      string `json:"artist"`
	ImageURL    string `json:"imageUrl"`
	ReleaseYear int    `json:"releaseYear"`
	TrackCount  int    `json:"trackCount"`
	TrackTotal  int    `json:"trackTotal"`
	ExternalURL string `json:"externalUrl"`
}

// DecadeStat counts how many of a period's top tracks were released in a
// given decade, taken from album release dates.
type DecadeStat struct {
	Decade int `json:"decade"`
	Count  int `json:"count"`
}

// ListeningRun is the longest unbroken stretch of back-to-back plays found
// in the recent playback window.
type ListeningRun struct {
	Minutes   int       `json:"minutes"`
	Tracks    int       `json:"tracks"`
	StartedAt time.Time `json:"startedAt"`
}

// PeriodMetrics holds every ranked list for one time window.
type PeriodMetrics struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Source string `json:"source"`

	Artists []ArtistStat `json:"artists"`
	Tracks  []TrackStat  `json:"tracks"`
	Genres  []GenreStat  `json:"genres"`

	// NewArtists are artists in this window that are absent from the
	// year-long list. Empty for the year window itself.
	NewArtists []ArtistStat `json:"newArtists"`

	TopAlbum *AlbumStat   `json:"topAlbum"`
	Decades  []DecadeStat `json:"decades"`

	// DeepCut is the least widely known artist in the period's top list.
	DeepCut *ArtistStat `json:"deepCut"`

	DistinctArtists int `json:"distinctArtists"`
	DistinctAlbums  int `json:"distinctAlbums"`

	// TotalPlays and TotalMinutes are only measurable for the week window,
	// which is built from real playback events rather than ranked lists.
	TotalPlays   int  `json:"totalPlays"`
	TotalMinutes int  `json:"totalMinutes"`
	HasTotals    bool `json:"hasTotals"`
}

// ReplayStat is the single most repeated track in the playback window.
type ReplayStat struct {
	Track TrackStat `json:"track"`
	Plays int       `json:"plays"`
}

// LibraryStats are plain counts from the user's Spotify library.
type LibraryStats struct {
	SavedTracks int `json:"savedTracks"`
	Playlists   int `json:"playlists"`
	Following   int `json:"following"`
}

// Dashboard is the complete payload the UI renders.
type Dashboard struct {
	UserID       string          `json:"userId"`
	Profile      UserProfile     `json:"profile"`
	CapturedAt   time.Time       `json:"capturedAt"`
	Periods      []PeriodMetrics `json:"periods"`
	MostReplayed *ReplayStat     `json:"mostReplayed"`
	LongestRun   *ListeningRun   `json:"longestRun"`
	Library      LibraryStats    `json:"library"`

	// PlayedAt carries raw playback timestamps so the browser can bucket them
	// into a listening clock in the viewer's own timezone.
	PlayedAt []time.Time `json:"playedAt"`
}
