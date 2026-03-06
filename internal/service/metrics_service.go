package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"rhythmiq/internal/db"
	"rhythmiq/internal/models"
	"rhythmiq/internal/spotify"
)

// MetricsService handles metric ingestion and historical reads.
type MetricsService struct {
	repo    db.Repository
	spotify *spotify.Client
}

// NewMetricsService creates a metrics service.
func NewMetricsService(repo db.Repository, spotifyClient *spotify.Client) *MetricsService {
	return &MetricsService{repo: repo, spotify: spotifyClient}
}

// RefreshSnapshot pulls fresh spotify data and stores a snapshot.
func (s *MetricsService) RefreshSnapshot(ctx context.Context, userID string) (models.MetricSnapshot, error) {
	token, err := s.getValidToken(ctx, userID)
	if err != nil {
		return models.MetricSnapshot{}, err
	}

	timeRanges := []string{"short_term", "medium_term", "long_term"}
	topTracks := make(map[string][]models.TrackSummary, len(timeRanges))
	topArtists := make(map[string][]models.ArtistSummary, len(timeRanges))

	for _, tr := range timeRanges {
		tracks, err := s.spotify.GetTopTracks(ctx, token.AccessToken, tr, 25)
		if err != nil {
			return models.MetricSnapshot{}, fmt.Errorf("fetch top tracks (%s): %w", tr, err)
		}
		topTracks[tr] = tracks

		artists, err := s.spotify.GetTopArtists(ctx, token.AccessToken, tr, 25)
		if err != nil {
			return models.MetricSnapshot{}, fmt.Errorf("fetch top artists (%s): %w", tr, err)
		}
		topArtists[tr] = artists
	}

	recentlyPlayed, err := s.spotify.GetRecentlyPlayed(ctx, token.AccessToken, 50)
	if err != nil {
		return models.MetricSnapshot{}, fmt.Errorf("fetch recently played: %w", err)
	}
	savedTracks, err := s.spotify.GetSavedTrackCount(ctx, token.AccessToken)
	if err != nil {
		return models.MetricSnapshot{}, fmt.Errorf("fetch saved tracks count: %w", err)
	}
	playlistCount, err := s.spotify.GetPlaylistCount(ctx, token.AccessToken)
	if err != nil {
		return models.MetricSnapshot{}, fmt.Errorf("fetch playlist count: %w", err)
	}
	followingCount, err := s.spotify.GetFollowingCount(ctx, token.AccessToken)
	if err != nil {
		return models.MetricSnapshot{}, fmt.Errorf("fetch following count: %w", err)
	}

	recentSnapshots, err := s.repo.GetRecentSnapshots(ctx, userID, 30)
	if err != nil && err != db.ErrNotFound {
		return models.MetricSnapshot{}, fmt.Errorf("load recent snapshots: %w", err)
	}

	stats := computeStats(topTracks, topArtists, recentlyPlayed, recentSnapshots)
	snapshot := models.MetricSnapshot{
		UserID:          userID,
		CapturedAt:      time.Now().UTC(),
		TopTracks:       topTracks,
		TopArtists:      topArtists,
		RecentlyPlayed:  recentlyPlayed,
		SavedTrackCount: savedTracks,
		PlaylistCount:   playlistCount,
		FollowingCount:  followingCount,
		Stats:           stats,
	}
	id, err := s.repo.SaveSnapshot(ctx, snapshot)
	if err != nil {
		return models.MetricSnapshot{}, err
	}
	snapshot.ID = id

	return snapshot, nil
}

// GetLatestSnapshot returns the newest snapshot.
func (s *MetricsService) GetLatestSnapshot(ctx context.Context, userID string) (models.MetricSnapshot, error) {
	snapshot, err := s.repo.GetLatestSnapshot(ctx, userID)
	if err != nil {
		return models.MetricSnapshot{}, err
	}
	if snapshot.Stats.EstimatedYearMinutes == 0 {
		recentSnapshots, err := s.repo.GetRecentSnapshots(ctx, userID, 30)
		if err == nil {
			snapshot.Stats.EstimatedYearMinutes = deriveYearlyMinutes(snapshot.Stats.EstimatedDailyMinutes, recentSnapshots)
		}
	}

	allSnapshots, err := s.repo.GetSnapshotsSince(ctx, userID, time.Unix(0, 0).UTC())
	if err == nil && len(allSnapshots) > 0 {
		ytd, allTime := computeTopArtistMinutes(allSnapshots, time.Now().UTC(), 8)
		snapshot.Stats.TopArtistMinutesYTD = ytd
		snapshot.Stats.TopArtistMinutesAll = allTime
	}
	return snapshot, nil
}

// GetHistory returns chart-ready points for a trailing period.
func (s *MetricsService) GetHistory(ctx context.Context, userID string, days int) ([]models.SnapshotPoint, error) {
	if days <= 0 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	snapshots, err := s.repo.GetSnapshotsSince(ctx, userID, since)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	points := make([]models.SnapshotPoint, 0, len(snapshots))
	for _, snapshot := range snapshots {
		points = append(points, models.SnapshotPoint{
			CapturedAt:            snapshot.CapturedAt,
			EstimatedDailyMinutes: snapshot.Stats.EstimatedDailyMinutes,
			UniqueArtistCount:     snapshot.Stats.UniqueArtistCount,
			UniqueGenreCount:      snapshot.Stats.UniqueGenreCount,
			ConsistencyScore:      round2(snapshot.Stats.ConsistencyScore),
			DiscoveryScore:        round2(snapshot.Stats.DiscoveryScore),
			ReplayScore:           round2(snapshot.Stats.ReplayScore),
			VarietyScore:          round2(snapshot.Stats.VarietyScore),
			SessionCount:          snapshot.Stats.SessionCount,
			AverageSessionMinutes: round2(snapshot.Stats.AverageSessionMinutes),
			WeekendListeningShare: round2(snapshot.Stats.WeekendListeningShare),
			NightOwlScore:         round2(snapshot.Stats.NightOwlScore),
			TopTrackConcentration: round2(snapshot.Stats.TopTrackConcentration),
		})
	}
	return points, nil
}

func (s *MetricsService) getValidToken(ctx context.Context, userID string) (models.SpotifyToken, error) {
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

func computeStats(topTracks map[string][]models.TrackSummary, topArtists map[string][]models.ArtistSummary, recentlyPlayed []models.PlaybackEvent, recentSnapshots []models.MetricSnapshot) models.SnapshotStats {
	genreWeights := buildGenreWeights(topArtists)
	uniqueArtists := countUniqueArtists(topTracks)
	uniqueGenres := len(genreWeights)
	if uniqueGenres > 12 {
		uniqueGenres = 12
	}

	dailyMinutes := estimateDailyMinutes(recentlyPlayed)
	consistency := computeConsistency(topArtists["short_term"], topArtists["long_term"])
	discovery := computeDiscovery(topArtists["short_term"], topArtists["long_term"])
	replay := computeReplay(recentlyPlayed)
	yearly := deriveYearlyMinutes(dailyMinutes, recentSnapshots)
	sessionCount, avgSessionMinutes := computeSessionStats(recentlyPlayed)
	avgTrackMinutes := computeAverageTrackMinutes(recentlyPlayed)
	weekendShare := computeWeekendShare(recentlyPlayed)
	nightOwl := computeNightOwlScore(recentlyPlayed)
	peakHour := computePeakListeningHour(recentlyPlayed)
	topTrackConcentration := computeTopTrackConcentration(recentlyPlayed)
	varietyScore := math.Max(0, 100-topTrackConcentration)
	daypartShare := computeDaypartShares(recentlyPlayed)
	weekdayShare := computeWeekdayShares(recentlyPlayed)

	topGenres := make([]models.GenreWeight, 0, len(genreWeights))
	for genre, weight := range genreWeights {
		topGenres = append(topGenres, models.GenreWeight{Genre: genre, Weight: round2(weight)})
	}
	sort.Slice(topGenres, func(i, j int) bool {
		return topGenres[i].Weight > topGenres[j].Weight
	})
	if len(topGenres) > 8 {
		topGenres = topGenres[:8]
	}

	mood := map[string]float64{
		"mainstreamAffinity": round2(mainstreamAffinity(topTracks["short_term"])),
		"exploration":        round2(discovery),
		"intensity":          round2(intensityIndex(recentlyPlayed)),
		"variety":            round2(varietyScore),
	}

	return models.SnapshotStats{
		EstimatedDailyMinutes: dailyMinutes,
		EstimatedYearMinutes:  yearly,
		UniqueArtistCount:     uniqueArtists,
		UniqueGenreCount:      uniqueGenres,
		ConsistencyScore:      round2(consistency),
		DiscoveryScore:        round2(discovery),
		ReplayScore:           round2(replay),
		VarietyScore:          round2(varietyScore),
		SessionCount:          sessionCount,
		AverageSessionMinutes: round2(avgSessionMinutes),
		AverageTrackMinutes:   round2(avgTrackMinutes),
		WeekendListeningShare: round2(weekendShare),
		NightOwlScore:         round2(nightOwl),
		PeakListeningHour:     peakHour,
		TopTrackConcentration: round2(topTrackConcentration),
		ListeningByDaypart:    daypartShare,
		ListeningByWeekday:    weekdayShare,
		TopGenres:             topGenres,
		MoodVector:            mood,
	}
}

type artistMinuteMeta struct {
	Name        string
	ExternalURL string
}

func computeTopArtistMinutes(snapshots []models.MetricSnapshot, now time.Time, limit int) ([]models.ArtistMinuteStat, []models.ArtistMinuteStat) {
	if len(snapshots) == 0 {
		return nil, nil
	}

	sorted := append([]models.MetricSnapshot(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CapturedAt.Before(sorted[j].CapturedAt)
	})

	ytdStart := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	ytdTotals := map[string]float64{}
	allTotals := map[string]float64{}
	artistMeta := map[string]artistMinuteMeta{}

	for i := range sorted {
		captured := sorted[i].CapturedAt.UTC()
		if captured.After(now) {
			break
		}

		var end time.Time
		if i+1 < len(sorted) {
			end = sorted[i+1].CapturedAt.UTC()
		} else {
			// Allow a small bounded projection window for the latest snapshot,
			// but avoid extrapolating indefinitely if ingestion is stale.
			const maxProjection = 12 * time.Hour
			end = minTime(now, captured.Add(maxProjection))
		}
		if end.After(now) {
			end = now
		}
		if !end.After(captured) {
			continue
		}

		registerArtistMeta(artistMeta, sorted[i].TopArtists)
		shares := estimateArtistShares(sorted[i])
		if len(shares) == 0 {
			continue
		}

		intervalMinutes := float64(sorted[i].Stats.EstimatedDailyMinutes) * end.Sub(captured).Hours() / 24.0
		if intervalMinutes <= 0 {
			continue
		}

		intervalDuration := end.Sub(captured).Seconds()
		if intervalDuration <= 0 {
			continue
		}

		overlapStart := maxTime(captured, ytdStart)
		overlapEnd := minTime(end, now)
		var ytdFactor float64
		if overlapEnd.After(overlapStart) {
			ytdFactor = overlapEnd.Sub(overlapStart).Seconds() / intervalDuration
		}

		for key, share := range shares {
			minutes := intervalMinutes * share
			allTotals[key] += minutes
			if ytdFactor > 0 {
				ytdTotals[key] += minutes * ytdFactor
			}
		}
	}

	return sortArtistMinuteStats(ytdTotals, artistMeta, limit), sortArtistMinuteStats(allTotals, artistMeta, limit)
}

func registerArtistMeta(meta map[string]artistMinuteMeta, topArtists map[string][]models.ArtistSummary) {
	for _, artists := range topArtists {
		for _, artist := range artists {
			key := artistKey(artist.Name)
			if key == "" {
				continue
			}

			existing, ok := meta[key]
			if !ok {
				meta[key] = artistMinuteMeta{
					Name:        artist.Name,
					ExternalURL: artist.ExternalURL,
				}
				continue
			}
			if existing.Name == "" {
				existing.Name = artist.Name
			}
			if existing.ExternalURL == "" && artist.ExternalURL != "" {
				existing.ExternalURL = artist.ExternalURL
			}
			meta[key] = existing
		}
	}
}

func estimateArtistShares(snapshot models.MetricSnapshot) map[string]float64 {
	shares := map[string]float64{}

	tracks := snapshot.TopTracks["short_term"]
	if len(tracks) == 0 {
		tracks = snapshot.TopTracks["medium_term"]
	}
	if len(tracks) == 0 {
		tracks = snapshot.TopTracks["long_term"]
	}

	totalWeight := 0.0
	for i, track := range tracks {
		if len(track.Artists) == 0 {
			continue
		}
		rankWeight := 1.0 / float64(i+1)
		durationWeight := float64(track.DurationMS) / 60000.0
		if durationWeight <= 0 {
			durationWeight = 3.0
		}
		trackWeight := rankWeight * durationWeight
		perArtist := trackWeight / float64(len(track.Artists))
		for _, name := range track.Artists {
			key := artistKey(name)
			if key == "" {
				continue
			}
			shares[key] += perArtist
			totalWeight += perArtist
		}
	}

	if totalWeight == 0 {
		artists := snapshot.TopArtists["short_term"]
		if len(artists) == 0 {
			artists = snapshot.TopArtists["medium_term"]
		}
		if len(artists) == 0 {
			artists = snapshot.TopArtists["long_term"]
		}
		for i, artist := range artists {
			key := artistKey(artist.Name)
			if key == "" {
				continue
			}
			weight := 1.0 / float64(i+1)
			shares[key] += weight
			totalWeight += weight
		}
	}

	if totalWeight <= 0 {
		return nil
	}
	for key := range shares {
		shares[key] /= totalWeight
	}
	return shares
}

func sortArtistMinuteStats(values map[string]float64, meta map[string]artistMinuteMeta, limit int) []models.ArtistMinuteStat {
	if len(values) == 0 {
		return nil
	}

	stats := make([]models.ArtistMinuteStat, 0, len(values))
	for key, minutes := range values {
		if minutes <= 0 {
			continue
		}
		info := meta[key]
		name := info.Name
		if name == "" {
			name = key
		}
		stats = append(stats, models.ArtistMinuteStat{
			Name:        name,
			Minutes:     int(math.Round(minutes)),
			ExternalURL: info.ExternalURL,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Minutes == stats[j].Minutes {
			return strings.ToLower(stats[i].Name) < strings.ToLower(stats[j].Name)
		}
		return stats[i].Minutes > stats[j].Minutes
	})

	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}

func artistKey(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func estimateDailyMinutes(recentlyPlayed []models.PlaybackEvent) int {
	if len(recentlyPlayed) == 0 {
		return 0
	}
	var totalDurationMS int
	var newest time.Time
	var oldest time.Time
	for i, event := range recentlyPlayed {
		totalDurationMS += event.Track.DurationMS
		if i == 0 || event.PlayedAt.After(newest) {
			newest = event.PlayedAt
		}
		if i == 0 || event.PlayedAt.Before(oldest) {
			oldest = event.PlayedAt
		}
	}
	windowHours := newest.Sub(oldest).Hours()
	if windowHours <= 0 {
		return totalDurationMS / 60000
	}
	rawDaily := float64(totalDurationMS) / 60000 * (24.0 / windowHours)
	if rawDaily > 800 {
		rawDaily = 800
	}
	if rawDaily < 0 {
		rawDaily = 0
	}
	return int(math.Round(rawDaily))
}

func deriveYearlyMinutes(currentDaily int, recentSnapshots []models.MetricSnapshot) int {
	if len(recentSnapshots) == 0 {
		return currentDaily * 365
	}
	var total int
	for _, snapshot := range recentSnapshots {
		total += snapshot.Stats.EstimatedDailyMinutes
	}
	avg := float64(total) / float64(len(recentSnapshots))
	if avg <= 0 {
		avg = float64(currentDaily)
	}
	return int(math.Round(avg * 365))
}

func countUniqueArtists(topTracks map[string][]models.TrackSummary) int {
	set := make(map[string]struct{})
	for _, list := range topTracks {
		for _, track := range list {
			for _, artist := range track.Artists {
				set[strings.ToLower(strings.TrimSpace(artist))] = struct{}{}
			}
		}
	}
	return len(set)
}

func buildGenreWeights(topArtists map[string][]models.ArtistSummary) map[string]float64 {
	weights := map[string]float64{}
	rangeWeight := map[string]float64{
		"short_term":  1.0,
		"medium_term": 0.7,
		"long_term":   0.5,
	}
	for tr, artists := range topArtists {
		for idx, artist := range artists {
			rankWeight := (1.0 / float64(idx+1)) * rangeWeight[tr]
			for _, genre := range artist.Genres {
				norm := strings.TrimSpace(strings.ToLower(genre))
				if norm == "" {
					continue
				}
				weights[norm] += rankWeight
			}
		}
	}
	return weights
}

func computeConsistency(short, long []models.ArtistSummary) float64 {
	if len(short) == 0 || len(long) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(long))
	for _, artist := range long {
		set[artist.ID] = struct{}{}
	}
	var overlap int
	for _, artist := range short {
		if _, ok := set[artist.ID]; ok {
			overlap++
		}
	}
	return (float64(overlap) / float64(len(short))) * 100
}

func computeDiscovery(short, long []models.ArtistSummary) float64 {
	if len(short) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(long))
	for _, artist := range long {
		set[artist.ID] = struct{}{}
	}
	var unseen int
	for _, artist := range short {
		if _, ok := set[artist.ID]; !ok {
			unseen++
		}
	}
	return (float64(unseen) / float64(len(short))) * 100
}

func computeReplay(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Track.ID]++
	}
	var repeats int
	for _, count := range counts {
		if count > 1 {
			repeats += count - 1
		}
	}
	return (float64(repeats) / float64(len(events))) * 100
}

func mainstreamAffinity(tracks []models.TrackSummary) float64 {
	if len(tracks) == 0 {
		return 0
	}
	var total int
	for _, track := range tracks {
		total += track.Popularity
	}
	return float64(total) / float64(len(tracks))
}

func intensityIndex(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	var totalDuration int
	for _, event := range events {
		totalDuration += event.Track.DurationMS
	}
	avgMinutes := float64(totalDuration) / 60000.0 / float64(len(events))
	return math.Min((avgMinutes/4.5)*100, 100)
}

func computeSessionStats(events []models.PlaybackEvent) (int, float64) {
	if len(events) == 0 {
		return 0, 0
	}

	sorted := append([]models.PlaybackEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PlayedAt.Before(sorted[j].PlayedAt)
	})

	const splitGap = 45 * time.Minute
	sessionCount := 1
	currentSessionMS := 0
	var sessionDurationsMS []int

	for i, event := range sorted {
		if i > 0 && event.PlayedAt.Sub(sorted[i-1].PlayedAt) > splitGap {
			sessionDurationsMS = append(sessionDurationsMS, currentSessionMS)
			currentSessionMS = 0
			sessionCount++
		}
		currentSessionMS += event.Track.DurationMS
	}
	sessionDurationsMS = append(sessionDurationsMS, currentSessionMS)

	var totalMS int
	for _, sessionMS := range sessionDurationsMS {
		totalMS += sessionMS
	}
	avgSessionMinutes := float64(totalMS) / 60000.0 / float64(len(sessionDurationsMS))
	return sessionCount, avgSessionMinutes
}

func computeAverageTrackMinutes(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	var totalMS int
	for _, event := range events {
		totalMS += event.Track.DurationMS
	}
	return float64(totalMS) / 60000.0 / float64(len(events))
}

func computeWeekendShare(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	weekend := 0
	for _, event := range events {
		switch event.PlayedAt.Weekday() {
		case time.Saturday, time.Sunday:
			weekend++
		}
	}
	return (float64(weekend) / float64(len(events))) * 100
}

func computeNightOwlScore(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	late := 0
	for _, event := range events {
		hour := event.PlayedAt.Hour()
		if hour >= 22 || hour < 5 {
			late++
		}
	}
	return (float64(late) / float64(len(events))) * 100
}

func computePeakListeningHour(events []models.PlaybackEvent) int {
	if len(events) == 0 {
		return 0
	}
	counts := make([]int, 24)
	for _, event := range events {
		counts[event.PlayedAt.Hour()]++
	}
	peakHour := 0
	peakCount := counts[0]
	for hour := 1; hour < len(counts); hour++ {
		if counts[hour] > peakCount {
			peakCount = counts[hour]
			peakHour = hour
		}
	}
	return peakHour
}

func computeTopTrackConcentration(events []models.PlaybackEvent) float64 {
	if len(events) == 0 {
		return 0
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Track.ID]++
	}
	values := make([]int, 0, len(counts))
	for _, count := range counts {
		values = append(values, count)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(values)))
	topN := 3
	if len(values) < topN {
		topN = len(values)
	}
	totalTop := 0
	for i := 0; i < topN; i++ {
		totalTop += values[i]
	}
	return (float64(totalTop) / float64(len(events))) * 100
}

func computeDaypartShares(events []models.PlaybackEvent) map[string]float64 {
	shares := map[string]float64{
		"morning":   0,
		"afternoon": 0,
		"evening":   0,
		"night":     0,
	}
	if len(events) == 0 {
		return shares
	}
	for _, event := range events {
		hour := event.PlayedAt.Hour()
		switch {
		case hour >= 5 && hour < 12:
			shares["morning"]++
		case hour >= 12 && hour < 17:
			shares["afternoon"]++
		case hour >= 17 && hour < 22:
			shares["evening"]++
		default:
			shares["night"]++
		}
	}
	for key, count := range shares {
		shares[key] = round2((count / float64(len(events))) * 100)
	}
	return shares
}

func computeWeekdayShares(events []models.PlaybackEvent) map[string]float64 {
	shares := map[string]float64{
		"mon": 0,
		"tue": 0,
		"wed": 0,
		"thu": 0,
		"fri": 0,
		"sat": 0,
		"sun": 0,
	}
	if len(events) == 0 {
		return shares
	}
	for _, event := range events {
		switch event.PlayedAt.Weekday() {
		case time.Monday:
			shares["mon"]++
		case time.Tuesday:
			shares["tue"]++
		case time.Wednesday:
			shares["wed"]++
		case time.Thursday:
			shares["thu"]++
		case time.Friday:
			shares["fri"]++
		case time.Saturday:
			shares["sat"]++
		case time.Sunday:
			shares["sun"]++
		}
	}
	for key, count := range shares {
		shares[key] = round2((count / float64(len(events))) * 100)
	}
	return shares
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
