package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"rhythmiq/internal/models"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type postgresRepository struct {
	db          *sql.DB
	tokenCipher *tokenCipher
}

func newPostgresRepository(dsn, tokenCryptoSecret string) (Repository, error) {
	tokenCipher, err := newTokenCipher(tokenCryptoSecret)
	if err != nil {
		return nil, fmt.Errorf("init postgres token encryption: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	repo := &postgresRepository{
		db:          db,
		tokenCipher: tokenCipher,
	}
	if err := repo.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *postgresRepository) Close() error {
	return r.db.Close()
}

func (r *postgresRepository) ensureSchema(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			country TEXT,
			product TEXT,
			avatar_url TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS spotify_tokens (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			token_type TEXT,
			scope TEXT,
			expires_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS metric_snapshots (
			id BIGSERIAL PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			captured_at TIMESTAMPTZ NOT NULL,
			top_tracks_json JSONB NOT NULL,
			top_artists_json JSONB NOT NULL,
			recently_played_json JSONB NOT NULL,
			saved_track_count INTEGER NOT NULL,
			playlist_count INTEGER NOT NULL,
			following_count INTEGER NOT NULL,
			stats_json JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

func (r *postgresRepository) UpsertUserProfile(ctx context.Context, profile models.UserProfile) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO users (id, display_name, country, product, avatar_url, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT(id) DO UPDATE SET
			display_name=EXCLUDED.display_name,
			country=EXCLUDED.country,
			product=EXCLUDED.product,
			avatar_url=EXCLUDED.avatar_url,
			updated_at=NOW()`,
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

func (r *postgresRepository) GetUserProfile(ctx context.Context, userID string) (models.UserProfile, error) {
	var profile models.UserProfile
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, display_name, country, product, avatar_url FROM users WHERE id = $1`,
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

func (r *postgresRepository) SaveToken(ctx context.Context, userID string, token models.SpotifyToken) error {
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
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT(user_id) DO UPDATE SET
			access_token=EXCLUDED.access_token,
			refresh_token=EXCLUDED.refresh_token,
			token_type=EXCLUDED.token_type,
			scope=EXCLUDED.scope,
			expires_at=EXCLUDED.expires_at,
		updated_at=NOW()`,
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

func (r *postgresRepository) GetToken(ctx context.Context, userID string) (models.SpotifyToken, error) {
	var token models.SpotifyToken
	err := r.db.QueryRowContext(
		ctx,
		`SELECT access_token, refresh_token, token_type, scope, expires_at FROM spotify_tokens WHERE user_id = $1`,
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

func (r *postgresRepository) ListUserIDs(ctx context.Context) ([]string, error) {
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

func (r *postgresRepository) SaveSnapshot(ctx context.Context, snapshot models.MetricSnapshot) (int64, error) {
	payload, err := marshalSnapshot(snapshot)
	if err != nil {
		return 0, err
	}

	var id int64
	err = r.db.QueryRowContext(
		ctx,
		`INSERT INTO metric_snapshots (
			user_id, captured_at, top_tracks_json, top_artists_json, recently_played_json,
			saved_track_count, playlist_count, following_count, stats_json
		) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6, $7, $8, $9::jsonb)
		RETURNING id`,
		snapshot.UserID,
		snapshot.CapturedAt.UTC(),
		string(payload.TopTracksJSON),
		string(payload.TopArtistsJSON),
		string(payload.RecentlyPlayedJSON),
		snapshot.SavedTrackCount,
		snapshot.PlaylistCount,
		snapshot.FollowingCount,
		string(payload.StatsJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}

	return id, nil
}

func (r *postgresRepository) GetLatestSnapshot(ctx context.Context, userID string) (models.MetricSnapshot, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, captured_at,
			top_tracks_json::text, top_artists_json::text, recently_played_json::text,
			saved_track_count, playlist_count, following_count, stats_json::text
		 FROM metric_snapshots WHERE user_id = $1 ORDER BY captured_at DESC LIMIT 1`,
		userID,
	)
	return scanSnapshot(row)
}

func (r *postgresRepository) GetSnapshotsSince(ctx context.Context, userID string, since time.Time) ([]models.MetricSnapshot, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, captured_at,
			top_tracks_json::text, top_artists_json::text, recently_played_json::text,
			saved_track_count, playlist_count, following_count, stats_json::text
		 FROM metric_snapshots WHERE user_id = $1 AND captured_at >= $2 ORDER BY captured_at ASC`,
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

func (r *postgresRepository) GetRecentSnapshots(ctx context.Context, userID string, limit int) ([]models.MetricSnapshot, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, captured_at,
			top_tracks_json::text, top_artists_json::text, recently_played_json::text,
			saved_track_count, playlist_count, following_count, stats_json::text
		 FROM metric_snapshots WHERE user_id = $1 ORDER BY captured_at DESC LIMIT $2`,
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
