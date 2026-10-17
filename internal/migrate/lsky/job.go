package lsky

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

// maxStatusLines caps the failure and skip lines kept for the console.
const maxStatusLines = 200

// RunnerDeps wires a Runner onto the running server.
type RunnerDeps struct {
	DB *gorm.DB
	// Ingest stores one image under rule policyID (service.ImportImage with
	// operator-facing errors).
	Ingest func(ctx context.Context, policyID uint64, item Item) error
	// Adoption opens the objects of the rule's storage for an in-place import.
	Adoption func(ctx context.Context, policyID uint64) (service.ObjectOpener, error)
	// HTTP downloads originals in copy mode.
	HTTP     *http.Client
	MaxBytes int64
	Now      func() time.Time
}

// Runner keeps one uploaded Lsky export in memory and imports it in the
// background for the admin console. The export (with its password hashes)
// never touches disk and is dropped on Discard or restart; a rerun of the
// same export skips everything already imported.
type Runner struct {
	deps RunnerDeps
	base context.Context

	mu      sync.Mutex
	snap    *Snapshot
	summary *service.LskyDumpSummary
	status  service.LskyImportStatus
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewRunner creates a Runner whose imports stop when base is canceled.
func NewRunner(base context.Context, deps RunnerDeps) (*Runner, error) {
	if base == nil || deps.DB == nil || deps.Ingest == nil || deps.Adoption == nil {
		return nil, errors.New("lsky runner: missing dependencies")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.HTTP == nil {
		deps.HTTP = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Runner{deps: deps, base: base, status: service.LskyImportStatus{Phase: service.LskyPhaseIdle}}, nil
}

// LoadDump parses an uploaded export and replaces the previous one.
func (r *Runner) LoadDump(ctx context.Context, body io.Reader, zone string) (service.LskyImportStatus, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		return service.LskyImportStatus{}, service.ErrInvalidInput
	}
	snap, err := ParseDump(body, loc)
	if errors.Is(err, ErrNotLskyDump) {
		return service.LskyImportStatus{}, service.ErrNotLskyDump
	}
	if err != nil {
		if ctx.Err() != nil {
			return service.LskyImportStatus{}, ctx.Err()
		}
		return service.LskyImportStatus{}, fmt.Errorf("%w: %w", service.ErrNotLskyDump, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return service.LskyImportStatus{}, service.ErrImportBusy
	}
	r.snap, r.summary = &snap, summarize(snap, r.deps.Now().UTC())
	r.status = service.LskyImportStatus{Phase: service.LskyPhaseIdle}
	return r.view(), nil
}

// Discard forgets the uploaded export and the last run.
func (r *Runner) Discard(context.Context) (service.LskyImportStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return service.LskyImportStatus{}, service.ErrImportBusy
	}
	r.snap, r.summary = nil, nil
	r.status = service.LskyImportStatus{Phase: service.LskyPhaseIdle}
	return r.view(), nil
}

// Status returns the export summary and run progress.
func (r *Runner) Status(context.Context) (service.LskyImportStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.view(), nil
}

// Start begins importing the uploaded export with the given rule.
func (r *Runner) Start(ctx context.Context, opts service.LskyImportOptions) (service.LskyImportStatus, error) {
	if opts.PolicyID == 0 || (opts.Mode != service.LskyModeAdopt && opts.Mode != service.LskyModeCopy) {
		return service.LskyImportStatus{}, service.ErrInvalidInput
	}
	var policy model.Policy
	if err := r.deps.DB.WithContext(ctx).First(&policy, "id = ?", opts.PolicyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return service.LskyImportStatus{}, service.ErrNotFound
		}
		return service.LskyImportStatus{}, fmt.Errorf("find import rule: %w", err)
	}
	if opts.Mode == service.LskyModeCopy && policy.WebPMode == "webp_only" {
		return service.LskyImportStatus{}, service.ErrInvalidInput
	}
	r.mu.Lock()
	loaded := r.snap
	r.mu.Unlock()
	if loaded == nil {
		return service.LskyImportStatus{}, service.ErrNotFound
	}
	maxBytes, err := FetchLimit(ctx, r.deps.DB, *loaded, r.deps.MaxBytes)
	if err != nil {
		return service.LskyImportStatus{}, err
	}
	var fetch Fetcher = HTTPFetcher{Client: r.deps.HTTP, BaseURL: opts.SourceURL, MaxBytes: maxBytes}
	var open service.ObjectOpener
	if opts.Mode == service.LskyModeAdopt {
		if open, err = r.deps.Adoption(ctx, opts.PolicyID); err != nil {
			return service.LskyImportStatus{}, err
		}
		fetch = StorageFetcher{Open: open, MaxBytes: maxBytes}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snap == nil {
		return service.LskyImportStatus{}, service.ErrNotFound
	}
	if r.cancel != nil {
		return service.LskyImportStatus{}, service.ErrImportBusy
	}
	started := r.deps.Now().UTC()
	total := 0
	for _, image := range r.snap.Images {
		if opts.StrategyID == 0 || image.StrategyID == opts.StrategyID {
			total++
		}
	}
	r.status = service.LskyImportStatus{Running: true, Phase: service.LskyPhaseAccounts, Options: &opts, Total: total, StartedAt: &started}
	runCtx, cancel := context.WithCancel(r.base)
	done := make(chan struct{})
	r.cancel, r.done = cancel, done
	snap := *r.snap
	go func() {
		defer close(done)
		defer cancel()
		err := r.run(runCtx, snap, policy, opts, fetch, open)
		r.mu.Lock()
		defer r.mu.Unlock()
		finished := r.deps.Now().UTC()
		r.status.Running, r.status.FinishedAt = false, &finished
		switch {
		case errors.Is(err, context.Canceled):
			r.status.Phase = service.LskyPhaseCanceled
		case err != nil:
			r.status.Phase, r.status.Error = service.LskyPhaseFailed, err.Error()
		default:
			r.status.Phase = service.LskyPhaseDone
		}
		r.cancel, r.done = nil, nil
	}()
	return r.view(), nil
}

// Cancel stops a running import after the current image; a later start
// continues with the images not yet imported.
func (r *Runner) Cancel(context.Context) (service.LskyImportStatus, error) {
	r.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.view(), nil
}

// Wait stops a running import and waits for it to end.
func (r *Runner) Wait() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (r *Runner) run(ctx context.Context, snap Snapshot, policy model.Policy, opts service.LskyImportOptions, fetch Fetcher, open service.ObjectOpener) error {
	if open != nil {
		var backend model.Storage
		if err := r.deps.DB.WithContext(ctx).First(&backend, "id = ?", policy.StorageID).Error; err != nil {
			return fmt.Errorf("find import storage: %w", err)
		}
		if err := CheckAdoption(ctx, snap, opts.StrategyID, backend.BaseURL, backend.PathPrefix, open); err != nil {
			return err
		}
	}
	mapping, accounts, err := ImportAccounts(ctx, r.deps.DB, snap, Options{PolicyID: policy.ID, Adopt: opts.Mode == service.LskyModeAdopt, Now: r.deps.Now})
	if err != nil {
		return err
	}
	lines := []string{fmt.Sprintf("组：新建 %d，合并 %d；用户：新建 %d，合并 %d；Token：新建 %d，已存在 %d；相册：新建 %d，合并 %d",
		accounts.GroupsCreated, accounts.GroupsReused, accounts.UsersCreated, accounts.UsersReused,
		accounts.TokensCreated, accounts.TokensKept, accounts.AlbumsCreated, accounts.AlbumsReused)}
	lines = append(lines, accounts.Notes...)
	r.mu.Lock()
	r.status.Accounts, r.status.Phase = lines, service.LskyPhaseImages
	r.mu.Unlock()
	ingest := func(ctx context.Context, item Item) error { return r.deps.Ingest(ctx, policy.ID, item) }
	report, err := ImportImages(ctx, r.deps.DB, snap, mapping, fetch, ingest, ImageOptions{StorageID: policy.StorageID, StrategyID: opts.StrategyID, Report: r.progress})
	r.progress(report)
	return err
}

func (r *Runner) progress(report ImageReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Imported, r.status.Existing = report.Imported, report.AlreadyThere
	r.status.Skipped, r.status.Failed = len(report.Skipped), len(report.Failed)
	r.status.Processed = report.Imported + report.AlreadyThere + len(report.Skipped) + len(report.Failed)
	r.status.Failures = capLines(report.Failed)
	r.status.Skips = capLines(report.Skipped)
}

func capLines(lines []string) []string {
	return append([]string(nil), lines[:min(len(lines), maxStatusLines)]...)
}

func (r *Runner) view() service.LskyImportStatus {
	view := r.status
	view.Loaded = r.snap != nil
	view.Summary = r.summary
	view.Accounts = append([]string{}, view.Accounts...)
	view.Failures = append([]string{}, view.Failures...)
	view.Skips = append([]string{}, view.Skips...)
	return view
}

func summarize(snap Snapshot, at time.Time) *service.LskyDumpSummary {
	counts := map[int64]int{}
	for _, image := range snap.Images {
		counts[image.StrategyID]++
	}
	summary := &service.LskyDumpSummary{Groups: len(snap.Groups), Users: len(snap.Users), Tokens: len(snap.Tokens), Albums: len(snap.Albums), Images: len(snap.Images), LoadedAt: at, Strategies: []service.LskyStrategySummary{}}
	for _, strategy := range snap.Strategies {
		summary.Strategies = append(summary.Strategies, service.LskyStrategySummary{ID: strategy.ID, Name: strategy.Name, URL: strategy.URL, Images: counts[strategy.ID]})
	}
	return summary
}
