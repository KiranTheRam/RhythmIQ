package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rhythmiq/internal/models"
)

// ErrNotFound indicates missing records.
var ErrNotFound = errors.New("not found")

// Repository defines the persistence contract used by services.
type Repository interface {
	Close() error
	UpsertUserProfile(ctx context.Context, profile models.UserProfile) error
	GetUserProfile(ctx context.Context, userID string) (models.UserProfile, error)
	SaveToken(ctx context.Context, userID string, token models.SpotifyToken) error
	GetToken(ctx context.Context, userID string) (models.SpotifyToken, error)
	ListUserIDs(ctx context.Context) ([]string, error)
	SaveSnapshot(ctx context.Context, snapshot models.MetricSnapshot) (int64, error)
	GetLatestSnapshot(ctx context.Context, userID string) (models.MetricSnapshot, error)
	GetSnapshotsSince(ctx context.Context, userID string, since time.Time) ([]models.MetricSnapshot, error)
	GetRecentSnapshots(ctx context.Context, userID string, limit int) ([]models.MetricSnapshot, error)
}

// New creates a repository implementation based on driver configuration.
func New(driver, sqlitePath, postgresDSN, tokenCryptoSecret string) (Repository, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite":
		if strings.TrimSpace(sqlitePath) == "" {
			sqlitePath = "./rhythmiq.db"
		}
		return newSQLiteRepository(sqlitePath, tokenCryptoSecret)
	case "postgres", "postgresql":
		if strings.TrimSpace(postgresDSN) == "" {
			return nil, fmt.Errorf("postgres selected but RHYTHMIQ_DB_DSN is empty")
		}
		return newPostgresRepository(postgresDSN, tokenCryptoSecret)
	default:
		return nil, fmt.Errorf("unsupported database driver %q (expected sqlite or postgres)", driver)
	}
}
