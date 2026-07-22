package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
		`CREATE TABLE IF NOT EXISTS dashboards (
			user_id TEXT PRIMARY KEY,
			captured_at DATETIME NOT NULL,
			payload TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`DROP TABLE IF EXISTS metric_snapshots;`,
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

func (r *sqliteRepository) SaveDashboard(ctx context.Context, dashboard models.Dashboard) error {
	payload, err := marshalDashboard(dashboard)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO dashboards (user_id, captured_at, payload, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
			captured_at=excluded.captured_at,
			payload=excluded.payload,
			updated_at=CURRENT_TIMESTAMP`,
		dashboard.UserID,
		dashboard.CapturedAt.UTC(),
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("save dashboard: %w", err)
	}
	return nil
}

func (r *sqliteRepository) GetDashboard(ctx context.Context, userID string) (models.Dashboard, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT payload FROM dashboards WHERE user_id = ?`,
		userID,
	)
	return scanDashboard(row)
}
