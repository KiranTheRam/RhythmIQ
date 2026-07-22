package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"rhythmiq/internal/models"
)

func marshalDashboard(dashboard models.Dashboard) ([]byte, error) {
	payload, err := json.Marshal(dashboard)
	if err != nil {
		return nil, fmt.Errorf("marshal dashboard: %w", err)
	}
	return payload, nil
}

func scanDashboard(scanner interface {
	Scan(dest ...any) error
}) (models.Dashboard, error) {
	var dashboard models.Dashboard
	var payload []byte

	if err := scanner.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dashboard, ErrNotFound
		}
		return dashboard, fmt.Errorf("scan dashboard: %w", err)
	}

	if err := json.Unmarshal(payload, &dashboard); err != nil {
		return dashboard, fmt.Errorf("unmarshal dashboard: %w", err)
	}
	return dashboard, nil
}
