package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/biliblihuorong/imgnest/internal/storage"
)

// LskyMaxDumpBytes bounds an uploaded SQL export; a Lsky site with a few
// hundred thousand images exports well below it.
const LskyMaxDumpBytes = 128 << 20

// ErrImportBusy rejects changing a Lsky import while one is running.
var ErrImportBusy = errors.New("lsky import already running")

// ErrNotLskyDump rejects an uploaded file that is not a Lsky Pro export.
var ErrNotLskyDump = errors.New("not a Lsky Pro database export")

// Lsky import modes: adopt takes the originals over where they already are
// (the rule's storage must be the old bucket); copy downloads them from the
// old public links and stores them again under the same paths.
const (
	LskyModeAdopt = "adopt"
	LskyModeCopy  = "copy"
)

// LskyDumpSummary describes an uploaded export. It never carries password
// hashes, tokens or storage credentials.
type LskyDumpSummary struct {
	Groups     int                   `json:"groups"`
	Users      int                   `json:"users"`
	Tokens     int                   `json:"tokens"`
	Albums     int                   `json:"albums"`
	Images     int                   `json:"images"`
	Strategies []LskyStrategySummary `json:"strategies"`
	LoadedAt   time.Time             `json:"loaded_at"`
}

// LskyStrategySummary is one Lsky storage strategy with its public URL.
type LskyStrategySummary struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Images int    `json:"images"`
}

// LskyImportOptions starts an import of the uploaded export.
type LskyImportOptions struct {
	PolicyID uint64 `json:"policy_id"`
	Mode     string `json:"mode"`
	// StrategyID limits the import to one Lsky storage strategy (0 = all).
	StrategyID int64 `json:"strategy_id"`
	// SourceURL overrides each strategy's public URL in copy mode.
	SourceURL string `json:"source_url"`
}

// LskyImportStatus is the console view of the uploaded export and the
// current or last import run. Lines are capped; counts are not.
type LskyImportStatus struct {
	Loaded     bool               `json:"loaded"`
	Summary    *LskyDumpSummary   `json:"summary"`
	Running    bool               `json:"running"`
	Phase      string             `json:"phase"`
	Options    *LskyImportOptions `json:"options"`
	Accounts   []string           `json:"accounts"`
	Total      int                `json:"total"`
	Processed  int                `json:"processed"`
	Imported   int                `json:"imported"`
	Existing   int                `json:"existing"`
	Skipped    int                `json:"skipped"`
	Failed     int                `json:"failed"`
	Failures   []string           `json:"failures"`
	Skips      []string           `json:"skips"`
	Error      string             `json:"error"`
	StartedAt  *time.Time         `json:"started_at"`
	FinishedAt *time.Time         `json:"finished_at"`
}

// Lsky import phases.
const (
	LskyPhaseIdle     = "idle"
	LskyPhaseAccounts = "accounts"
	LskyPhaseImages   = "images"
	LskyPhaseDone     = "done"
	LskyPhaseFailed   = "failed"
	LskyPhaseCanceled = "canceled"
)

// ObjectOpener reads one stored object with its ownership metadata.
type ObjectOpener func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error)

// AdoptionReader opens objects in the storage behind a rule, for an import
// that adopts a Lsky bucket in place. Only storages whose driver can keep
// foreign objects untouched (S3 with versioning) qualify.
func (s *ImageService) AdoptionReader(ctx context.Context, policyID uint64) (ObjectOpener, error) {
	policy, err := s.deps.Policies.Find(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("find import rule: %w", err)
	}
	backend, err := s.deps.Storages.Find(ctx, policy.StorageID)
	if err != nil {
		return nil, fmt.Errorf("find import storage: %w", err)
	}
	if backend.Driver != "s3" {
		return nil, ErrInvalidInput
	}
	driver, err := s.deps.Drivers.DriverFor(ctx, backend)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if _, ok := driver.(storage.LegacyObjects); !ok {
		return nil, ErrInvalidInput
	}
	return driver.Open, nil
}
