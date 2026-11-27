package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const (
	JunkPatternsSetting = "junk_patterns"
	CommitSizesSetting  = "commit_sizes"
	RapidSeriesSetting  = "rapid_series"
)

// Querier is implemented by both a pool and a transaction
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LoadSetting returns the defaults when the setting was never saved
func LoadSetting[T any](ctx context.Context, db Querier, name string, defaults T) (value T, isDefault bool, err error) {
	var storedValue []byte
	err = db.QueryRow(ctx, "SELECT value FROM settings WHERE name = $1", name).Scan(&storedValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaults, true, nil
	}

	if err != nil {
		return value, false, err
	}

	err = json.Unmarshal(storedValue, &value)
	return value, false, err
}
