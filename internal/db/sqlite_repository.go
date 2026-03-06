package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"rhythmiq/internal/models"

	_ "modernc.org/sqlite"
)

type sqliteRepository struct {
	db          *sql.DB
	tokenCipher *tokenCipher
}

func newSQLiteRepository(path, tokenCryptoSecret string) (Repository, error) {
	tokenCipher, err := newTokenCipher(tokenCryptoSecret)
	if err != nil {
		return nil, fmt.Errorf("init sqlite token encryption: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	repo := &sqliteRepository{
		db:          db,
		tokenCipher: tokenCipher,
	}
	if err := repo.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *sqliteRepository) Close() error {
	return r.db.Close()
}

func (r *sqliteRepository) ensureSchema(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			country TEXT,
			product TEXT,
			avatar_url TEXT,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS spotify_tokens (
			user_id TEXT PRIMARY KEY,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			token_type TEXT,
			scope TEXT,
			expires_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS metric_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			captured_at DATETIME NOT NULL,
			top_tracks_json TEXT NOT NULL,
			top_artists_json TEXT NOT NULL,
			recently_played_json TEXT NOT NULL,
			saved_track_count INTEGER NOT NULL,
			playlist_count INTEGER NOT NULL,
			following_count INTEGER NOT NULL,
			stats_json TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_metric_snapshots_user_time ON metric_snapshots(user_id, captured_at DESC);`,
	}

	for _, stmt := range schema {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}

	return nil
}

func (r *sqliteRepository) UpsertUserProfile(ctx context.Context, profile models.UserProfile) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO users (id, display_name, country, product, avatar_url, updated_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
			display_name=excluded.display_name,
			country=excluded.country,
			product=excluded.product,
			avatar_url=excluded.avatar_url,
			updated_at=CURRENT_TIMESTAMP`,
		profile.ID,
		profile.DisplayName,
		profile.Country,
		profile.Product,
		profile.AvatarURL,
	)
	if err != nil {
		return fmt.Errorf("upsert user profile: %w", err)
	}
	return nil
}

func (r *sqliteRepository) GetUserProfile(ctx context.Context, userID string) (models.UserProfile, error) {
	var profile models.UserProfile
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, display_name, country, product, avatar_url FROM users WHERE id = ?`,
		userID,
	).Scan(&profile.ID, &profile.DisplayName, &profile.Country, &profile.Product, &profile.AvatarURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return profile, ErrNotFound
		}
		return profile, fmt.Errorf("get user profile: %w", err)
	}

	return profile, nil
}

func (r *sqliteRepository) SaveToken(ctx context.Context, userID string, token models.SpotifyToken) error {
	accessToken, err := r.tokenCipher.encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}
	refreshToken, err := r.tokenCipher.encrypt(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh token: %w", err)
	}

	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO spotify_tokens (user_id, access_token, refresh_token, token_type, scope, expires_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
			access_token=excluded.access_token,
			refresh_token=excluded.refresh_token,
			token_type=excluded.token_type,
			scope=excluded.scope,
			expires_at=excluded.expires_at,
		updated_at=CURRENT_TIMESTAMP`,
		userID,
		accessToken,
		refreshToken,
		token.TokenType,
		token.Scope,
		token.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}

func (r *sqliteRepository) GetToken(ctx context.Context, userID string) (models.SpotifyToken, error) {
	var token models.SpotifyToken
	err := r.db.QueryRowContext(
		ctx,
		`SELECT access_token, refresh_token, token_type, scope, expires_at FROM spotify_tokens WHERE user_id = ?`,
		userID,
	).Scan(&token.AccessToken, &token.RefreshToken, &token.TokenType, &token.Scope, &token.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return token, ErrNotFound
		}
		return token, fmt.Errorf("get token: %w", err)
	}

	accessToken, err := r.tokenCipher.decrypt(token.AccessToken)
	if err != nil {
		return token, fmt.Errorf("decrypt access token: %w", err)
	}
	refreshToken, err := r.tokenCipher.decrypt(token.RefreshToken)
	if err != nil {
		return token, fmt.Errorf("decrypt refresh token: %w", err)
	}
	token.AccessToken = accessToken
	token.RefreshToken = refreshToken

	return token, nil
}

func (r *sqliteRepository) ListUserIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM users ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list user ids: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user ids: %w", err)
	}

	return userIDs, nil
}

func (r *sqliteRepository) SaveSnapshot(ctx context.Context, snapshot models.MetricSnapshot) (int64, error) {
	payload, err := marshalSnapshot(snapshot)
	if err != nil {
		return 0, err
	}

	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO metric_snapshots (
			user_id, captured_at, top_tracks_json, top_artists_json, recently_played_json,
			saved_track_count, playlist_count, following_count, stats_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.UserID,
		snapshot.CapturedAt.UTC(),
		string(payload.TopTracksJSON),
		string(payload.TopArtistsJSON),
		string(payload.RecentlyPlayedJSON),
		snapshot.SavedTrackCount,
		snapshot.PlaylistCount,
		snapshot.FollowingCount,
		string(payload.StatsJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("snapshot last insert id: %w", err)
	}
	return id, nil
}

func (r *sqliteRepository) GetLatestSnapshot(ctx context.Context, userID string) (models.MetricSnapshot, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, captured_at, top_tracks_json, top_artists_json, recently_played_json,
			saved_track_count, playlist_count, following_count, stats_json
		 FROM metric_snapshots WHERE user_id = ? ORDER BY captured_at DESC LIMIT 1`,
		userID,
	)
	return scanSnapshot(row)
}

func (r *sqliteRepository) GetSnapshotsSince(ctx context.Context, userID string, since time.Time) ([]models.MetricSnapshot, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, captured_at, top_tracks_json, top_artists_json, recently_played_json,
			saved_track_count, playlist_count, following_count, stats_json
		 FROM metric_snapshots WHERE user_id = ? AND captured_at >= ? ORDER BY captured_at ASC`,
		userID,
		since.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("query snapshots since: %w", err)
	}
	defer rows.Close()

	var snapshots []models.MetricSnapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots since: %w", err)
	}
	return snapshots, nil
}

func (r *sqliteRepository) GetRecentSnapshots(ctx context.Context, userID string, limit int) ([]models.MetricSnapshot, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, captured_at, top_tracks_json, top_artists_json, recently_played_json,
			saved_track_count, playlist_count, following_count, stats_json
		 FROM metric_snapshots WHERE user_id = ? ORDER BY captured_at DESC LIMIT ?`,
		userID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []models.MetricSnapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent snapshots: %w", err)
	}
	return snapshots, nil
}
