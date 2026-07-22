package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"rhythmiq/internal/db"
	"rhythmiq/internal/models"
	"rhythmiq/internal/spotify"
)

// Period keys understood by the dashboard.
const (
	PeriodWeek  = "week"
	PeriodMonth = "month"
	PeriodYear  = "year"
)

// listSize caps every ranked list the UI renders.
const listSize = 10

// DashboardService builds the listening dashboard from Spotify data.
type DashboardService struct {
	repo    db.Repository
	spotify *spotify.Client
}

// NewDashboardService creates a dashboard service.
func NewDashboardService(repo db.Repository, spotifyClient *spotify.Client) *DashboardService {
	return &DashboardService{repo: repo, spotify: spotifyClient}
}

// Get returns the cached dashboard, building one if nothing is cached yet.
func (s *DashboardService) Get(ctx context.Context, userID string) (models.Dashboard, error) {
	dashboard, err := s.repo.GetDashboard(ctx, userID)
	if err == nil {
		return dashboard, nil
	}
	if err != db.ErrNotFound {
		return models.Dashboard{}, err
	}
	return s.Refresh(ctx, userID)
}

// Refresh pulls fresh Spotify data, rebuilds the dashboard, and caches it.
func (s *DashboardService) Refresh(ctx context.Context, userID string) (models.Dashboard, error) {
	token, err := s.getValidToken(ctx, userID)
	if err != nil {
		return models.Dashboard{}, err
	}
	accessToken := token.AccessToken

	profile, err := s.repo.GetUserProfile(ctx, userID)
	if err != nil && err != db.ErrNotFound {
		return models.Dashboard{}, fmt.Errorf("load profile: %w", err)
	}

	// Month and year come from Spotify's own ranked lists.
	monthArtists, err := s.spotify.GetTopArtists(ctx, accessToken, "short_term", 20)
	if err != nil {
		return models.Dashboard{}, fmt.Errorf("fetch month artists: %w", err)
	}
	monthTracks, err := s.spotify.GetTopTracks(ctx, accessToken, "short_term", 20)
	if err != nil {
		return models.Dashboard{}, fmt.Errorf("fetch month tracks: %w", err)
	}
	yearArtists, err := s.spotify.GetTopArtists(ctx, accessToken, "long_term", 20)
	if err != nil {
		return models.Dashboard{}, fmt.Errorf("fetch year artists: %w", err)
	}
	yearTracks, err := s.spotify.GetTopTracks(ctx, accessToken, "long_term", 20)
	if err != nil {
		return models.Dashboard{}, fmt.Errorf("fetch year tracks: %w", err)
	}

	// The week comes from real playback events.
	recentlyPlayed, err := s.spotify.GetRecentlyPlayed(ctx, accessToken, 50)
	if err != nil {
		return models.Dashboard{}, fmt.Errorf("fetch recently played: %w", err)
	}

	// Playback events carry artist IDs but no genres or images, so hydrate them.
	artistDetail := indexArtists(monthArtists, yearArtists)
	missing := make([]string, 0)
	for _, event := range recentlyPlayed {
		for _, id := range event.Track.ArtistIDs {
			if _, ok := artistDetail[id]; !ok {
				missing = append(missing, id)
			}
		}
	}
	if len(missing) > 0 {
		hydrated, err := s.spotify.GetArtistsByID(ctx, accessToken, missing)
		if err != nil {
			return models.Dashboard{}, fmt.Errorf("hydrate recent artists: %w", err)
		}
		for _, artist := range hydrated {
			artistDetail[artist.ID] = artist
		}
	}

	library, err := s.fetchLibrary(ctx, accessToken)
	if err != nil {
		return models.Dashboard{}, err
	}

	yearArtistIDs := make(map[string]struct{}, len(yearArtists))
	for _, artist := range yearArtists {
		yearArtistIDs[artist.ID] = struct{}{}
	}

	week := buildWeekPeriod(recentlyPlayed, artistDetail, yearArtistIDs)
	month := buildRankedPeriod(PeriodMonth, "This Month", "Spotify's top lists for the last 4 weeks", monthArtists, monthTracks, yearArtistIDs)
	year := buildRankedPeriod(PeriodYear, "This Year", "Spotify's top lists for roughly the last 12 months", yearArtists, yearTracks, nil)

	dashboard := models.Dashboard{
		UserID:       userID,
		Profile:      profile,
		CapturedAt:   time.Now().UTC(),
		Periods:      []models.PeriodMetrics{week, month, year},
		MostReplayed: computeMostReplayed(recentlyPlayed),
		LongestRun:   computeLongestRun(recentlyPlayed),
		Library:      library,
		PlayedAt:     collectPlayTimes(recentlyPlayed),
	}

	if err := s.repo.SaveDashboard(ctx, dashboard); err != nil {
		return models.Dashboard{}, err
	}
	return dashboard, nil
}

func (s *DashboardService) fetchLibrary(ctx context.Context, accessToken string) (models.LibraryStats, error) {
	saved, err := s.spotify.GetSavedTrackCount(ctx, accessToken)
	if err != nil {
		return models.LibraryStats{}, fmt.Errorf("fetch saved tracks count: %w", err)
	}
	playlists, err := s.spotify.GetPlaylistCount(ctx, accessToken)
	if err != nil {
		return models.LibraryStats{}, fmt.Errorf("fetch playlist count: %w", err)
	}
	following, err := s.spotify.GetFollowingCount(ctx, accessToken)
	if err != nil {
		return models.LibraryStats{}, fmt.Errorf("fetch following count: %w", err)
	}
	return models.LibraryStats{
		SavedTracks: saved,
		Playlists:   playlists,
		Following:   following,
	}, nil
}

func (s *DashboardService) getValidToken(ctx context.Context, userID string) (models.SpotifyToken, error) {
	token, err := s.repo.GetToken(ctx, userID)
	if err != nil {
		return models.SpotifyToken{}, fmt.Errorf("load spotify token: %w", err)
	}

	if time.Until(token.ExpiresAt) > 2*time.Minute {
		return token, nil
	}

	refreshed, err := s.spotify.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return models.SpotifyToken{}, fmt.Errorf("refresh token: %w", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	if err := s.repo.SaveToken(ctx, userID, refreshed); err != nil {
		return models.SpotifyToken{}, fmt.Errorf("save refreshed token: %w", err)
	}
	return refreshed, nil
}

func indexArtists(lists ...[]models.ArtistSummary) map[string]models.ArtistSummary {
	index := map[string]models.ArtistSummary{}
	for _, list := range lists {
		for _, artist := range list {
			if artist.ID == "" {
				continue
			}
			index[artist.ID] = artist
		}
	}
	return index
}

// buildRankedPeriod converts Spotify's ranked lists into period metrics. Rank
// is the only signal available here — Spotify exposes no play counts for these.
func buildRankedPeriod(key, label, source string, artists []models.ArtistSummary, tracks []models.TrackSummary, compareAgainst map[string]struct{}) models.PeriodMetrics {
	artistStats := make([]models.ArtistStat, 0, len(artists))
	for i, artist := range artists {
		artistStats = append(artistStats, models.ArtistStat{
			Rank:        i + 1,
			ID:          artist.ID,
			Name:        artist.Name,
			Genres:      artist.Genres,
			ImageURL:    artist.ImageURL,
			ExternalURL: artist.ExternalURL,
		})
	}

	trackStats := make([]models.TrackStat, 0, len(tracks))
	for i, track := range tracks {
		trackStats = append(trackStats, models.TrackStat{
			Rank:          i + 1,
			ID:            track.ID,
			Name:          track.Name,
			Artists:       track.Artists,
			Album:         track.Album,
			AlbumImageURL: track.AlbumImageURL,
			DurationMS:    track.DurationMS,
			ExternalURL:   track.ExternalURL,
		})
	}

	// Weight genres by inverse rank so the top artists shape the mix most.
	genreWeights := map[string]float64{}
	for i, artist := range artists {
		weight := 1.0 / float64(i+1)
		for _, genre := range artist.Genres {
			if norm := normalizeGenre(genre); norm != "" {
				genreWeights[norm] += weight
			}
		}
	}

	return models.PeriodMetrics{
		Key:             key,
		Label:           label,
		Source:          source,
		Artists:         truncateArtists(artistStats, listSize),
		Tracks:          truncateTracks(trackStats, listSize),
		Genres:          rankGenres(genreWeights, 6),
		NewArtists:      findNewArtists(artistStats, compareAgainst),
		TopAlbum:        computeTopAlbum(tracks),
		Decades:         computeDecades(tracks),
		DeepCut:         findDeepCut(artistStats, artists),
		DistinctArtists: countDistinctArtists(tracks),
		DistinctAlbums:  countDistinctAlbums(tracks),
	}
}

// computeTopAlbum finds the album contributing the most of a period's top
// tracks. Spotify has no top-albums endpoint, so this is a plain count.
func computeTopAlbum(tracks []models.TrackSummary) *models.AlbumStat {
	if len(tracks) == 0 {
		return nil
	}

	counts := map[string]int{}
	info := map[string]models.TrackSummary{}
	for _, track := range tracks {
		if track.AlbumID == "" {
			continue
		}
		counts[track.AlbumID]++
		if _, ok := info[track.AlbumID]; !ok {
			info[track.AlbumID] = track
		}
	}

	bestID := ""
	bestCount := 0
	for id, count := range counts {
		if count > bestCount || (count == bestCount && id < bestID) {
			bestID, bestCount = id, count
		}
	}
	// A single track from an album is not an album you favour.
	if bestID == "" || bestCount < 2 {
		return nil
	}

	track := info[bestID]
	artist := ""
	if len(track.Artists) > 0 {
		artist = track.Artists[0]
	}
	return &models.AlbumStat{
		Name:        track.Album,
		Artist:      artist,
		ImageURL:    track.AlbumImageURL,
		ReleaseYear: track.ReleaseYear,
		TrackCount:  bestCount,
		TrackTotal:  len(tracks),
		ExternalURL: track.AlbumURL,
	}
}

// computeDecades buckets a period's top tracks by release decade.
func computeDecades(tracks []models.TrackSummary) []models.DecadeStat {
	counts := map[int]int{}
	for _, track := range tracks {
		if track.ReleaseYear <= 0 {
			continue
		}
		counts[(track.ReleaseYear/10)*10]++
	}

	stats := make([]models.DecadeStat, 0, len(counts))
	for decade, count := range counts {
		stats = append(stats, models.DecadeStat{Decade: decade, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Decade < stats[j].Decade })
	return stats
}

// findDeepCut returns the least widely known artist among a period's top
// artists — the favourite that fewest other people share.
func findDeepCut(stats []models.ArtistStat, artists []models.ArtistSummary) *models.ArtistStat {
	popularity := map[string]int{}
	for _, artist := range artists {
		popularity[artist.ID] = artist.Popularity
	}

	var best *models.ArtistStat
	bestPopularity := 101
	for i := range stats {
		p, ok := popularity[stats[i].ID]
		if !ok {
			continue
		}
		if p < bestPopularity {
			bestPopularity = p
			candidate := stats[i]
			candidate.Popularity = p
			best = &candidate
		}
	}
	return best
}

func countDistinctArtists(tracks []models.TrackSummary) int {
	seen := map[string]struct{}{}
	for _, track := range tracks {
		for _, id := range track.ArtistIDs {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func countDistinctAlbums(tracks []models.TrackSummary) int {
	seen := map[string]struct{}{}
	for _, track := range tracks {
		if track.AlbumID != "" {
			seen[track.AlbumID] = struct{}{}
		}
	}
	return len(seen)
}

// computeLongestRun finds the longest unbroken stretch of listening in the
// playback window. A gap of more than 15 minutes between the start of one
// track and the next ends the run.
func computeLongestRun(events []models.PlaybackEvent) *models.ListeningRun {
	if len(events) < 2 {
		return nil
	}

	sorted := append([]models.PlaybackEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PlayedAt.Before(sorted[j].PlayedAt)
	})

	const maxGap = 15 * time.Minute
	bestMS, bestTracks := 0, 0
	var bestStart time.Time

	runMS, runTracks := sorted[0].Track.DurationMS, 1
	runStart := sorted[0].PlayedAt

	commit := func() {
		if runMS > bestMS {
			bestMS, bestTracks, bestStart = runMS, runTracks, runStart
		}
	}

	for i := 1; i < len(sorted); i++ {
		if sorted[i].PlayedAt.Sub(sorted[i-1].PlayedAt) > maxGap {
			commit()
			runMS, runTracks, runStart = 0, 0, sorted[i].PlayedAt
		}
		runMS += sorted[i].Track.DurationMS
		runTracks++
	}
	commit()

	if bestTracks < 2 {
		return nil
	}
	return &models.ListeningRun{
		Minutes:   bestMS / 60000,
		Tracks:    bestTracks,
		StartedAt: bestStart.UTC(),
	}
}

// buildWeekPeriod derives the week from real playback events, so it can report
// actual play counts and actual minutes.
func buildWeekPeriod(events []models.PlaybackEvent, artistDetail map[string]models.ArtistSummary, yearArtistIDs map[string]struct{}) models.PeriodMetrics {
	period := models.PeriodMetrics{
		Key:        PeriodWeek,
		Label:      "This Week",
		Source:     "Your last 50 plays, with real timestamps",
		HasTotals:  true,
		Artists:    []models.ArtistStat{},
		Tracks:     []models.TrackStat{},
		Genres:     []models.GenreStat{},
		NewArtists: []models.ArtistStat{},
		Decades:    []models.DecadeStat{},
	}
	if len(events) == 0 {
		return period
	}

	trackPlays := map[string]int{}
	trackInfo := map[string]models.TrackSummary{}
	artistPlays := map[string]int{}
	totalMS := 0

	for _, event := range events {
		track := event.Track
		totalMS += track.DurationMS
		if track.ID != "" {
			trackPlays[track.ID]++
			trackInfo[track.ID] = track
		}
		for _, id := range track.ArtistIDs {
			artistPlays[id]++
		}
	}

	period.TotalPlays = len(events)
	period.TotalMinutes = totalMS / 60000

	// Rank artists by how many plays they appeared on.
	artistStats := make([]models.ArtistStat, 0, len(artistPlays))
	for id, plays := range artistPlays {
		detail := artistDetail[id]
		name := detail.Name
		if name == "" {
			continue
		}
		artistStats = append(artistStats, models.ArtistStat{
			ID:          id,
			Name:        name,
			Genres:      detail.Genres,
			ImageURL:    detail.ImageURL,
			Plays:       plays,
			ExternalURL: detail.ExternalURL,
		})
	}
	sort.Slice(artistStats, func(i, j int) bool {
		if artistStats[i].Plays == artistStats[j].Plays {
			return strings.ToLower(artistStats[i].Name) < strings.ToLower(artistStats[j].Name)
		}
		return artistStats[i].Plays > artistStats[j].Plays
	})
	for i := range artistStats {
		artistStats[i].Rank = i + 1
	}

	trackStats := make([]models.TrackStat, 0, len(trackPlays))
	for id, plays := range trackPlays {
		track := trackInfo[id]
		trackStats = append(trackStats, models.TrackStat{
			ID:            id,
			Name:          track.Name,
			Artists:       track.Artists,
			Album:         track.Album,
			AlbumImageURL: track.AlbumImageURL,
			DurationMS:    track.DurationMS,
			Plays:         plays,
			ExternalURL:   track.ExternalURL,
		})
	}
	sort.Slice(trackStats, func(i, j int) bool {
		if trackStats[i].Plays == trackStats[j].Plays {
			return strings.ToLower(trackStats[i].Name) < strings.ToLower(trackStats[j].Name)
		}
		return trackStats[i].Plays > trackStats[j].Plays
	})
	for i := range trackStats {
		trackStats[i].Rank = i + 1
	}

	// Genres weighted by real play counts rather than rank.
	genreWeights := map[string]float64{}
	for id, plays := range artistPlays {
		for _, genre := range artistDetail[id].Genres {
			if norm := normalizeGenre(genre); norm != "" {
				genreWeights[norm] += float64(plays)
			}
		}
	}

	// Deduplicated tracks, so album and decade counts describe distinct
	// records rather than being skewed by repeat plays.
	uniqueTracks := make([]models.TrackSummary, 0, len(trackInfo))
	for _, track := range trackInfo {
		uniqueTracks = append(uniqueTracks, track)
	}

	artistSummaries := make([]models.ArtistSummary, 0, len(artistPlays))
	for id := range artistPlays {
		if detail, ok := artistDetail[id]; ok {
			artistSummaries = append(artistSummaries, detail)
		}
	}

	period.Artists = truncateArtists(artistStats, listSize)
	period.Tracks = truncateTracks(trackStats, listSize)
	period.Genres = rankGenres(genreWeights, 6)
	period.NewArtists = findNewArtists(artistStats, yearArtistIDs)
	period.TopAlbum = computeTopAlbum(uniqueTracks)
	period.Decades = computeDecades(uniqueTracks)
	period.DeepCut = findDeepCut(artistStats, artistSummaries)
	period.DistinctArtists = len(artistPlays)
	period.DistinctAlbums = countDistinctAlbums(uniqueTracks)
	return period
}

// findNewArtists returns artists absent from the comparison set — the
// "new to you" list. A nil comparison set means the check does not apply.
// The result is always non-nil so it serialises as [] rather than null.
func findNewArtists(artists []models.ArtistStat, compareAgainst map[string]struct{}) []models.ArtistStat {
	out := make([]models.ArtistStat, 0)
	if compareAgainst == nil {
		return out
	}
	for _, artist := range artists {
		if _, known := compareAgainst[artist.ID]; known {
			continue
		}
		out = append(out, artist)
		if len(out) == 5 {
			break
		}
	}
	return out
}

// rankGenres converts raw weights into percentage shares. The result is always
// non-nil so it serialises as [] rather than null.
func rankGenres(weights map[string]float64, limit int) []models.GenreStat {
	stats := make([]models.GenreStat, 0, len(weights))
	if len(weights) == 0 {
		return stats
	}

	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 {
		return stats
	}

	for genre, weight := range weights {
		stats = append(stats, models.GenreStat{
			Genre: genre,
			Share: round1(weight / total * 100),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Share == stats[j].Share {
			return stats[i].Genre < stats[j].Genre
		}
		return stats[i].Share > stats[j].Share
	})

	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}

func computeMostReplayed(events []models.PlaybackEvent) *models.ReplayStat {
	if len(events) == 0 {
		return nil
	}

	counts := map[string]int{}
	info := map[string]models.TrackSummary{}
	for _, event := range events {
		if event.Track.ID == "" {
			continue
		}
		counts[event.Track.ID]++
		info[event.Track.ID] = event.Track
	}

	bestID := ""
	bestCount := 0
	for id, count := range counts {
		if count > bestCount || (count == bestCount && id < bestID) {
			bestID = id
			bestCount = count
		}
	}
	if bestID == "" || bestCount < 2 {
		return nil
	}

	track := info[bestID]
	return &models.ReplayStat{
		Plays: bestCount,
		Track: models.TrackStat{
			Rank:          1,
			ID:            track.ID,
			Name:          track.Name,
			Artists:       track.Artists,
			Album:         track.Album,
			AlbumImageURL: track.AlbumImageURL,
			DurationMS:    track.DurationMS,
			Plays:         bestCount,
			ExternalURL:   track.ExternalURL,
		},
	}
}

func collectPlayTimes(events []models.PlaybackEvent) []time.Time {
	times := make([]time.Time, 0, len(events))
	for _, event := range events {
		times = append(times, event.PlayedAt.UTC())
	}
	return times
}

func truncateArtists(list []models.ArtistStat, limit int) []models.ArtistStat {
	if limit > 0 && len(list) > limit {
		return list[:limit]
	}
	return list
}

func truncateTracks(list []models.TrackStat, limit int) []models.TrackStat {
	if limit > 0 && len(list) > limit {
		return list[:limit]
	}
	return list
}

func normalizeGenre(genre string) string {
	return strings.TrimSpace(strings.ToLower(genre))
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
