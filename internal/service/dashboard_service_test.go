package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"rhythmiq/internal/models"
)

// The UI reads .length on every list, so a null would blank the page.
// Every list field must serialise as [] even when there is nothing to report.
func TestPeriodListsNeverSerialiseAsNull(t *testing.T) {
	artists := []models.ArtistSummary{{ID: "a1", Name: "Someone", Genres: []string{"pop"}}}
	tracks := []models.TrackSummary{{ID: "t1", Name: "A Song", Artists: []string{"Someone"}}}

	cases := map[string]models.PeriodMetrics{
		// The year has no longer window to compare against, so newArtists is empty.
		"year": buildRankedPeriod(PeriodYear, "This Year", "src", artists, tracks, nil),
		// A period with no data at all.
		"empty ranked": buildRankedPeriod(PeriodMonth, "This Month", "src", nil, nil, map[string]struct{}{}),
		// A week with no playback events.
		"empty week": buildWeekPeriod(nil, map[string]models.ArtistSummary{}, map[string]struct{}{}),
	}

	for name, period := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(period)
			if err != nil {
				t.Fatalf("marshal period: %v", err)
			}

			for _, field := range []string{"artists", "tracks", "genres", "newArtists"} {
				if strings.Contains(string(encoded), `"`+field+`":null`) {
					t.Errorf("%s serialised as null; want []\npayload: %s", field, encoded)
				}
			}
		})
	}
}

func TestFindNewArtistsExcludesKnownArtists(t *testing.T) {
	artists := []models.ArtistStat{
		{ID: "known", Name: "Known"},
		{ID: "fresh", Name: "Fresh"},
	}
	known := map[string]struct{}{"known": {}}

	got := findNewArtists(artists, known)
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("got %+v, want only the artist absent from the comparison set", got)
	}
}

func TestComputeTopAlbumNeedsMoreThanOneTrack(t *testing.T) {
	// One track from an album is not an album you favour.
	single := []models.TrackSummary{
		{ID: "1", AlbumID: "x", Album: "X", Artists: []string{"A"}},
		{ID: "2", AlbumID: "y", Album: "Y", Artists: []string{"B"}},
	}
	if got := computeTopAlbum(single); got != nil {
		t.Fatalf("got %+v, want nil when no album repeats", got)
	}

	repeated := append(single, models.TrackSummary{ID: "3", AlbumID: "y", Album: "Y", Artists: []string{"B"}})
	got := computeTopAlbum(repeated)
	if got == nil || got.Name != "Y" || got.TrackCount != 2 || got.TrackTotal != 3 {
		t.Fatalf("got %+v, want album Y with 2 of 3 tracks", got)
	}
}

func TestComputeDecadesBucketsAndSkipsUnknownYears(t *testing.T) {
	tracks := []models.TrackSummary{
		{ReleaseYear: 1971}, {ReleaseYear: 1979},
		{ReleaseYear: 2024},
		{ReleaseYear: 0}, // unknown release date
	}
	got := computeDecades(tracks)
	want := []models.DecadeStat{{Decade: 1970, Count: 2}, {Decade: 2020, Count: 1}}

	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
}

func TestFindDeepCutPicksLeastPopular(t *testing.T) {
	stats := []models.ArtistStat{{ID: "big"}, {ID: "small"}, {ID: "mid"}}
	artists := []models.ArtistSummary{
		{ID: "big", Popularity: 90},
		{ID: "small", Popularity: 11},
		{ID: "mid", Popularity: 55},
	}

	got := findDeepCut(stats, artists)
	if got == nil || got.ID != "small" || got.Popularity != 11 {
		t.Fatalf("got %+v, want the least popular artist", got)
	}
}

func TestComputeLongestRunSplitsOnGaps(t *testing.T) {
	base := time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)
	threeMin := 180000

	events := []models.PlaybackEvent{
		// A two-track run.
		{PlayedAt: base, Track: models.TrackSummary{DurationMS: threeMin}},
		{PlayedAt: base.Add(3 * time.Minute), Track: models.TrackSummary{DurationMS: threeMin}},
		// A gap longer than 15 minutes ends it, then a longer three-track run.
		{PlayedAt: base.Add(2 * time.Hour), Track: models.TrackSummary{DurationMS: threeMin}},
		{PlayedAt: base.Add(2*time.Hour + 3*time.Minute), Track: models.TrackSummary{DurationMS: threeMin}},
		{PlayedAt: base.Add(2*time.Hour + 6*time.Minute), Track: models.TrackSummary{DurationMS: threeMin}},
	}

	got := computeLongestRun(events)
	if got == nil {
		t.Fatal("got nil, want the longer run")
	}
	if got.Tracks != 3 || got.Minutes != 9 {
		t.Fatalf("got %d tracks / %d minutes, want 3 / 9", got.Tracks, got.Minutes)
	}
	if !got.StartedAt.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("got start %v, want %v", got.StartedAt, base.Add(2*time.Hour))
	}
}

func TestComputeMostReplayedIgnoresSinglePlays(t *testing.T) {
	events := []models.PlaybackEvent{
		{Track: models.TrackSummary{ID: "a", Name: "A"}},
		{Track: models.TrackSummary{ID: "b", Name: "B"}},
	}
	if got := computeMostReplayed(events); got != nil {
		t.Fatalf("got %+v, want nil when nothing was actually repeated", got)
	}

	events = append(events, models.PlaybackEvent{Track: models.TrackSummary{ID: "b", Name: "B"}})
	got := computeMostReplayed(events)
	if got == nil || got.Track.ID != "b" || got.Plays != 2 {
		t.Fatalf("got %+v, want track b with 2 plays", got)
	}
}
