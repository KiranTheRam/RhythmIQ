package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"rhythmiq/internal/models"
)

type snapshotPayload struct {
	TopTracksJSON      []byte
	TopArtistsJSON     []byte
	RecentlyPlayedJSON []byte
	StatsJSON          []byte
}

func marshalSnapshot(snapshot models.MetricSnapshot) (snapshotPayload, error) {
	topTracksJSON, err := json.Marshal(snapshot.TopTracks)
	if err != nil {
		return snapshotPayload{}, fmt.Errorf("marshal top tracks: %w", err)
	}
	topArtistsJSON, err := json.Marshal(snapshot.TopArtists)
	if err != nil {
		return snapshotPayload{}, fmt.Errorf("marshal top artists: %w", err)
	}
	recentlyPlayedJSON, err := json.Marshal(snapshot.RecentlyPlayed)
	if err != nil {
		return snapshotPayload{}, fmt.Errorf("marshal recently played: %w", err)
	}
	statsJSON, err := json.Marshal(snapshot.Stats)
	if err != nil {
		return snapshotPayload{}, fmt.Errorf("marshal stats: %w", err)
	}

	return snapshotPayload{
		TopTracksJSON:      topTracksJSON,
		TopArtistsJSON:     topArtistsJSON,
		RecentlyPlayedJSON: recentlyPlayedJSON,
		StatsJSON:          statsJSON,
	}, nil
}

func scanSnapshot(scanner interface {
	Scan(dest ...any) error
}) (models.MetricSnapshot, error) {
	var snapshot models.MetricSnapshot
	var topTracksJSON, topArtistsJSON, recentlyPlayedJSON, statsJSON []byte

	err := scanner.Scan(
		&snapshot.ID,
		&snapshot.UserID,
		&snapshot.CapturedAt,
		&topTracksJSON,
		&topArtistsJSON,
		&recentlyPlayedJSON,
		&snapshot.SavedTrackCount,
		&snapshot.PlaylistCount,
		&snapshot.FollowingCount,
		&statsJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return snapshot, ErrNotFound
		}
		return snapshot, fmt.Errorf("scan snapshot: %w", err)
	}

	if err := json.Unmarshal(topTracksJSON, &snapshot.TopTracks); err != nil {
		return snapshot, fmt.Errorf("unmarshal top tracks: %w", err)
	}
	if err := json.Unmarshal(topArtistsJSON, &snapshot.TopArtists); err != nil {
		return snapshot, fmt.Errorf("unmarshal top artists: %w", err)
	}
	if err := json.Unmarshal(recentlyPlayedJSON, &snapshot.RecentlyPlayed); err != nil {
		return snapshot, fmt.Errorf("unmarshal recently played: %w", err)
	}
	if err := json.Unmarshal(statsJSON, &snapshot.Stats); err != nil {
		return snapshot, fmt.Errorf("unmarshal stats: %w", err)
	}

	return snapshot, nil
}
