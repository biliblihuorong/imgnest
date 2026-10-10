package repo

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// pluginCounterRetention is how long an untouched counter is kept.
const pluginCounterRetention = 400 * 24 * time.Hour

// PluginCounterRepository stores extension plugins' counters.
type PluginCounterRepository struct {
	db  *gorm.DB
	now func() time.Time
}

// NewPluginCounterRepository creates the counter store.
func NewPluginCounterRepository(db *gorm.DB, now func() time.Time) (*PluginCounterRepository, error) {
	if db == nil || now == nil {
		return nil, errors.New("create plugin counter repository: missing dependencies")
	}
	return &PluginCounterRepository{db: db, now: now}, nil
}

func checkCounterKey(plugin, name, subject string) error {
	for _, part := range []struct {
		value string
		max   int
	}{{plugin, 32}, {name, 80}, {subject, 80}} {
		if part.value == "" || len(part.value) > part.max || !utf8.ValidString(part.value) {
			return model.ErrInvalidInput
		}
	}
	return nil
}

// Add adds delta to one counter, creating it at delta, and returns the total.
func (r *PluginCounterRepository) Add(ctx context.Context, plugin, name, subject string, delta int64) (int64, error) {
	if err := checkCounterKey(plugin, name, subject); err != nil {
		return 0, fmt.Errorf("add plugin counter: %w", err)
	}
	var total int64
	err := r.db.WithContext(ctx).Raw(`INSERT INTO plugin_counters (plugin, name, subject, value, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (plugin, name, subject) DO UPDATE
		SET value = plugin_counters.value + excluded.value, updated_at = excluded.updated_at
		RETURNING value`, plugin, name, subject, delta, r.now().UTC()).Scan(&total).Error
	if err != nil {
		return 0, repositoryError("add plugin counter", err)
	}
	return total, nil
}

// Get returns one counter's total; a missing counter is 0.
func (r *PluginCounterRepository) Get(ctx context.Context, plugin, name, subject string) (int64, error) {
	if err := checkCounterKey(plugin, name, subject); err != nil {
		return 0, fmt.Errorf("get plugin counter: %w", err)
	}
	var totals []int64
	err := r.db.WithContext(ctx).Raw(`SELECT value FROM plugin_counters WHERE plugin = ? AND name = ? AND subject = ?`, plugin, name, subject).Scan(&totals).Error
	if err != nil {
		return 0, repositoryError("get plugin counter", err)
	}
	if len(totals) == 0 {
		return 0, nil
	}
	return totals[0], nil
}

// Prune deletes counters untouched for the retention period.
func (r *PluginCounterRepository) Prune(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`DELETE FROM plugin_counters WHERE updated_at < ?`, r.now().UTC().Add(-pluginCounterRetention))
	if result.Error != nil {
		return 0, repositoryError("prune plugin counters", result.Error)
	}
	return result.RowsAffected, nil
}
