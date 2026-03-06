package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"rhythmiq/internal/config"
	"rhythmiq/internal/models"
)

// RecommendationService generates user guidance and narrative insights.
type RecommendationService struct {
	httpClient *http.Client
	apiKey     string
	model      string
}

// NewRecommendationService builds a recommendation service.
func NewRecommendationService(cfg config.Config) *RecommendationService {
	return &RecommendationService{
		httpClient: &http.Client{Timeout: 25 * time.Second},
		apiKey:     cfg.OpenAIAPIKey,
		model:      cfg.OpenAIModel,
	}
}

// GenerateInsights produces recommendations and narrative text from metrics.
func (s *RecommendationService) GenerateInsights(ctx context.Context, snapshot models.MetricSnapshot, history []models.SnapshotPoint) (models.InsightResponse, error) {
	recs := heuristicRecommendations(snapshot, history)
	genreRecs := heuristicGenreRecommendations(snapshot)
	artistRecs := heuristicArtistRecommendations(snapshot)
	songRecs := heuristicSongRecommendations(snapshot)
	resp := models.InsightResponse{
		GeneratedAt:           time.Now().UTC(),
		Narrative:             baselineNarrative(snapshot, history),
		Recommendations:       recs,
		GenreRecommendations:  genreRecs,
		ArtistRecommendations: artistRecs,
		SongRecommendations:   songRecs,
		OpenAIGenerated:       false,
	}

	if s.apiKey == "" {
		return resp, nil
	}

	narrative, err := s.generateOpenAINarrative(ctx, snapshot, history)
	if err != nil {
		return resp, nil
	}
	if narrative != "" {
		resp.Narrative = narrative
		resp.OpenAIGenerated = true
	}
	return resp, nil
}

func heuristicRecommendations(snapshot models.MetricSnapshot, history []models.SnapshotPoint) []models.Recommendation {
	recs := []models.Recommendation{}

	if snapshot.Stats.DiscoveryScore < 35 {
		recs = append(recs, models.Recommendation{
			Title:       "Kick Off a Discovery Week",
			Description: "Your discovery score is low. Follow 3 new artists in a genre outside your top 3 and queue one track from each daily.",
			Confidence:  "high",
			Type:        "discovery",
		})
	}
	if snapshot.Stats.ReplayScore > 45 {
		recs = append(recs, models.Recommendation{
			Title:       "Break Replay Loops",
			Description: "You are replay-heavy right now. Build a short 'No Repeats' playlist and set it as your first listen each day.",
			Confidence:  "medium",
			Type:        "habit",
		})
	}
	if snapshot.Stats.ConsistencyScore > 60 {
		recs = append(recs, models.Recommendation{
			Title:       "Deep Catalog Mode",
			Description: "Your taste is stable. Explore album deep-cuts from your top artists instead of only their top tracks.",
			Confidence:  "high",
			Type:        "catalog",
		})
	}

	topGenre := ""
	if len(snapshot.Stats.TopGenres) > 0 {
		topGenre = snapshot.Stats.TopGenres[0].Genre
	}
	if topGenre != "" {
		recs = append(recs, models.Recommendation{
			Title:       "Genre Expansion",
			Description: fmt.Sprintf("You lean toward %s. Try adjacent genres this week to increase variety while staying in your lane.", topGenre),
			Confidence:  "medium",
			Type:        "genre",
		})
	}

	trend := historicalMinuteTrend(history)
	if trend < -10 {
		recs = append(recs, models.Recommendation{
			Title:       "Listening Streak Reset",
			Description: "Your listening minutes are trending down. Schedule a 20-minute daily listening block for the next 7 days.",
			Confidence:  "medium",
			Type:        "trend",
		})
	}

	if len(recs) == 0 {
		recs = append(recs, models.Recommendation{
			Title:       "Momentum Week",
			Description: "Your metrics are balanced. Keep momentum by rotating one new artist into your top tracks every day.",
			Confidence:  "medium",
			Type:        "general",
		})
	}

	if len(recs) > 5 {
		recs = recs[:5]
	}
	return recs
}

func baselineNarrative(snapshot models.MetricSnapshot, history []models.SnapshotPoint) string {
	topArtists := snapshot.TopArtists["short_term"]
	artistNames := make([]string, 0, 3)
	for i := 0; i < len(topArtists) && i < 3; i++ {
		artistNames = append(artistNames, topArtists[i].Name)
	}
	if len(artistNames) == 0 {
		artistNames = append(artistNames, "new sounds")
	}

	trend := historicalMinuteTrend(history)
	trendText := "stable"
	if trend > 10 {
		trendText = "rising"
	} else if trend < -10 {
		trendText = "cooling off"
	}

	return fmt.Sprintf(
		"Your current listening profile is led by %s. Daily minutes are %s, with %.0f%% consistency and %.0f%% discovery. Estimated yearly listening pace: %d minutes. You average %.1f-minute tracks across %d recent sessions, with %.0f%% weekend listening.",
		strings.Join(artistNames, ", "),
		trendText,
		snapshot.Stats.ConsistencyScore,
		snapshot.Stats.DiscoveryScore,
		snapshot.Stats.EstimatedYearMinutes,
		snapshot.Stats.AverageTrackMinutes,
		snapshot.Stats.SessionCount,
		snapshot.Stats.WeekendListeningShare,
	)
}

func heuristicGenreRecommendations(snapshot models.MetricSnapshot) []models.GenreRecommendation {
	recs := []models.GenreRecommendation{}
	if len(snapshot.Stats.TopGenres) == 0 {
		return recs
	}

	adjacency := map[string][]string{
		"hip hop":    {"alternative hip hop", "neo soul"},
		"rap":        {"jazz rap", "conscious hip hop"},
		"pop":        {"synth pop", "indie pop"},
		"rock":       {"indie rock", "post-punk"},
		"indie":      {"dream pop", "shoegaze"},
		"r&b":        {"neo soul", "alternative r&b"},
		"electronic": {"house", "downtempo"},
		"edm":        {"future bass", "progressive house"},
		"metal":      {"progressive metal", "post-metal"},
		"latin":      {"latin alternative", "reggaeton"},
		"country":    {"americana", "alt-country"},
	}

	for i, top := range snapshot.Stats.TopGenres {
		if i >= 3 {
			break
		}
		candidates := []string{"genre adjacent sounds"}
		for key, options := range adjacency {
			if strings.Contains(top.Genre, key) {
				candidates = options
				break
			}
		}

		recs = append(recs, models.GenreRecommendation{
			Genre:      candidates[0],
			Reason:     fmt.Sprintf("You currently lean toward %s (weight %.1f). This adjacent style should expand variety without feeling disconnected.", top.Genre, top.Weight),
			Confidence: "medium",
		})
	}

	return recs
}

func heuristicArtistRecommendations(snapshot models.MetricSnapshot) []models.ArtistRecommendation {
	shortSet := map[string]struct{}{}
	for _, artist := range snapshot.TopArtists["short_term"] {
		shortSet[artist.ID] = struct{}{}
	}

	recs := []models.ArtistRecommendation{}
	appendCandidate := func(artist models.ArtistSummary, reason string) {
		if _, inShort := shortSet[artist.ID]; inShort {
			return
		}
		for _, existing := range recs {
			if existing.Name == artist.Name {
				return
			}
		}
		recs = append(recs, models.ArtistRecommendation{
			Name:        artist.Name,
			Reason:      reason,
			ExternalURL: artist.ExternalURL,
			Confidence:  "medium",
		})
	}

	for _, artist := range snapshot.TopArtists["medium_term"] {
		appendCandidate(artist, "Strong medium-term fit that is not currently in your short-term rotation.")
		if len(recs) >= 4 {
			break
		}
	}
	if len(recs) < 4 {
		for _, artist := range snapshot.TopArtists["long_term"] {
			appendCandidate(artist, "Long-term favorite worth bringing back into your weekly queue.")
			if len(recs) >= 4 {
				break
			}
		}
	}
	if len(recs) == 0 && len(snapshot.TopArtists["short_term"]) > 0 {
		artist := snapshot.TopArtists["short_term"][0]
		recs = append(recs, models.ArtistRecommendation{
			Name:        artist.Name,
			Reason:      "Start with your strongest current favorite and explore deeper cuts from the same catalog.",
			ExternalURL: artist.ExternalURL,
			Confidence:  "medium",
		})
	}

	return recs
}

func heuristicSongRecommendations(snapshot models.MetricSnapshot) []models.SongRecommendation {
	shortSet := map[string]struct{}{}
	for _, track := range snapshot.TopTracks["short_term"] {
		shortSet[track.ID] = struct{}{}
	}

	recs := []models.SongRecommendation{}
	addTrack := func(track models.TrackSummary, reason string) {
		if _, inShort := shortSet[track.ID]; inShort {
			return
		}
		for _, existing := range recs {
			if existing.Track == track.Name && existing.Artist == primaryArtist(track.Artists) {
				return
			}
		}
		recs = append(recs, models.SongRecommendation{
			Track:       track.Name,
			Artist:      primaryArtist(track.Artists),
			Reason:      reason,
			ExternalURL: track.ExternalURL,
			Confidence:  "medium",
		})
	}

	for _, track := range snapshot.TopTracks["medium_term"] {
		addTrack(track, "Strong medium-term track that can refresh your current rotation.")
		if len(recs) >= 5 {
			break
		}
	}
	if len(recs) < 5 {
		for _, track := range snapshot.TopTracks["long_term"] {
			addTrack(track, "Long-term favorite you are currently underplaying.")
			if len(recs) >= 5 {
				break
			}
		}
	}
	if len(recs) == 0 && len(snapshot.TopTracks["short_term"]) > 0 {
		track := snapshot.TopTracks["short_term"][0]
		recs = append(recs, models.SongRecommendation{
			Track:       track.Name,
			Artist:      primaryArtist(track.Artists),
			Reason:      "Anchor your next session with your current top track before branching into new picks.",
			ExternalURL: track.ExternalURL,
			Confidence:  "medium",
		})
	}

	return recs
}

func historicalMinuteTrend(points []models.SnapshotPoint) float64 {
	if len(points) < 2 {
		return 0
	}
	sorted := append([]models.SnapshotPoint(nil), points...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CapturedAt.Before(sorted[j].CapturedAt)
	})
	first := float64(sorted[0].EstimatedDailyMinutes)
	last := float64(sorted[len(sorted)-1].EstimatedDailyMinutes)
	if first <= 0 {
		return 0
	}
	return ((last - first) / first) * 100
}

func (s *RecommendationService) generateOpenAINarrative(ctx context.Context, snapshot models.MetricSnapshot, history []models.SnapshotPoint) (string, error) {
	prompt := s.buildPrompt(snapshot, history)

	payload := map[string]any{
		"model": s.model,
		"input": []map[string]any{
			{
				"role":    "system",
				"content": "You are a music data analyst. Write concise, personalized, practical listening insights.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_output_tokens": 220,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal openai payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build openai request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read openai response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}

	if text, ok := parsed["output_text"].(string); ok && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text), nil
	}
	if output, ok := parsed["output"].([]any); ok {
		var chunks []string
		for _, item := range output {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			content, ok := obj["content"].([]any)
			if !ok {
				continue
			}
			for _, c := range content {
				cobj, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if ctype, ok := cobj["type"].(string); !ok || ctype != "output_text" {
					continue
				}
				if text, ok := cobj["text"].(string); ok && strings.TrimSpace(text) != "" {
					chunks = append(chunks, strings.TrimSpace(text))
				}
			}
		}
		if len(chunks) > 0 {
			return strings.Join(chunks, "\n"), nil
		}
	}

	return "", nil
}

func (s *RecommendationService) buildPrompt(snapshot models.MetricSnapshot, history []models.SnapshotPoint) string {
	artistNames := make([]string, 0, 5)
	for i, artist := range snapshot.TopArtists["short_term"] {
		if i >= 5 {
			break
		}
		artistNames = append(artistNames, artist.Name)
	}
	genreNames := make([]string, 0, 5)
	for i, genre := range snapshot.Stats.TopGenres {
		if i >= 5 {
			break
		}
		genreNames = append(genreNames, genre.Genre)
	}

	return fmt.Sprintf(`Generate a short personalized music insight report (max 220 words) with:
1) one paragraph summary,
2) three practical actions,
3) one genre suggestion,
4) two artist suggestions,
5) two song suggestions.
Use this data:
- Estimated daily minutes: %d
- Estimated yearly minutes: %d
- Consistency score: %.2f
- Discovery score: %.2f
- Replay score: %.2f
- Variety score: %.2f
- Session count: %d
- Avg session minutes: %.2f
- Weekend listening share: %.2f
- Unique artists: %d
- Unique genres: %d
- Top artists now: %s
- Top genres now: %s
- Historical points: %d`,
		snapshot.Stats.EstimatedDailyMinutes,
		snapshot.Stats.EstimatedYearMinutes,
		snapshot.Stats.ConsistencyScore,
		snapshot.Stats.DiscoveryScore,
		snapshot.Stats.ReplayScore,
		snapshot.Stats.VarietyScore,
		snapshot.Stats.SessionCount,
		snapshot.Stats.AverageSessionMinutes,
		snapshot.Stats.WeekendListeningShare,
		snapshot.Stats.UniqueArtistCount,
		snapshot.Stats.UniqueGenreCount,
		strings.Join(artistNames, ", "),
		strings.Join(genreNames, ", "),
		len(history),
	)
}

func primaryArtist(artists []string) string {
	if len(artists) == 0 {
		return "Unknown Artist"
	}
	return artists[0]
}
