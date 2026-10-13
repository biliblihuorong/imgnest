package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

type fixedTrashPolicy struct {
	days  int
	ok    bool
	asked *[]uint64
}

func (p fixedTrashPolicy) TrashDays(_ context.Context, userID, groupID uint64) (int, bool) {
	*p.asked = append(*p.asked, userID, groupID)
	return p.days, p.ok
}

func TestTrashPolicySetsRetention(t *testing.T) {
	for name, tc := range map[string]struct {
		policies []fixedTrashPolicy
		want     time.Duration
		purged   bool
	}{
		"site setting":        {want: 7 * 24 * time.Hour},
		"policy declines":     {policies: []fixedTrashPolicy{{days: 30}}, want: 7 * 24 * time.Hour},
		"first answer wins":   {policies: []fixedTrashPolicy{{days: 30, ok: true}, {days: 2, ok: true}}, want: 30 * 24 * time.Hour},
		"clamped above":       {policies: []fixedTrashPolicy{{days: 99999, ok: true}}, want: maxTrashDays * 24 * time.Hour},
		"zero purges at once": {policies: []fixedTrashPolicy{{days: -3, ok: true}}, purged: true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, rows, _, _, subject := uploadFixture(t, "png")
			var asked []uint64
			for _, p := range tc.policies {
				p.asked = &asked
				svc.deps.TrashPolicies = append(svc.deps.TrashPolicies, p)
			}
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if err != nil {
				t.Fatal(err)
			}
			before := time.Now().UTC()
			if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
				t.Fatal(err)
			}
			row, ok := rows.rows[view.Key]
			if tc.purged {
				if ok {
					t.Fatal("zero-day retention kept the image")
				}
				return
			}
			if !ok || row.PurgeAt == nil {
				t.Fatalf("row = %+v", row)
			}
			if got := row.PurgeAt.Sub(before); got < tc.want-time.Minute || got > tc.want+time.Minute {
				t.Fatalf("retention = %s, want %s", got, tc.want)
			}
			if len(tc.policies) > 0 && (len(asked) < 2 || asked[0] != subject.userID) {
				t.Fatalf("policy asked %v", asked)
			}
		})
	}
}

type retryTrashSettings struct {
	uploadSettings
	days int
	err  error
}

func (s *retryTrashSettings) TrashDays(context.Context) (int, error) { return s.days, s.err }

type retryTrashDriver struct {
	*storage.Local
	fail bool
}

func (d *retryTrashDriver) Copy(ctx context.Context, source, target string, opts storage.CopyOptions) (storage.Receipt, error) {
	if d.fail {
		d.fail = false
		return storage.Receipt{}, errors.New("temporary copy failure")
	}
	return d.Local.Copy(ctx, source, target, opts)
}

func TestTrashRetryUsesRecordedPolicyRetention(t *testing.T) {
	for _, tc := range []struct {
		name                string
		site, policy        int
		settingsUnavailable bool
	}{
		{name: "keeps positive policy despite immediate site retention", site: 0, policy: 30},
		{name: "immediate policy despite positive site retention", site: 7, policy: 0},
		{name: "does not reread unavailable settings", site: 7, policy: 30, settingsUnavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, rows, _, local, subject := uploadFixture(t, "png")
			svc.deps.Now = time.Now
			settings := &retryTrashSettings{days: tc.site}
			svc.deps.Settings = settings
			var asked []uint64
			svc.deps.TrashPolicies = []TrashPolicy{fixedTrashPolicy{days: tc.policy, ok: true, asked: &asked}}
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if err != nil {
				t.Fatal(err)
			}
			svc.deps.Drivers = uploadDriverProvider{driver: &retryTrashDriver{Local: local, fail: true}}
			if err := svc.Trash(t.Context(), subject, view.Key); !errors.Is(err, ErrStorage) {
				t.Fatalf("first move: %v", err)
			}
			first := rows.rows[view.Key]
			if first.Operation != model.ImageOperationTrash || first.DeletedAt == nil || first.PurgeAt == nil {
				t.Fatal("failed move did not retain its operation and deadline")
			}
			deadline := *first.PurgeAt
			if got := deadline.Sub(*first.DeletedAt); got != time.Duration(tc.policy)*24*time.Hour {
				t.Fatalf("recorded retention=%s", got)
			}
			if tc.settingsUnavailable {
				settings.err = errors.New("settings unavailable")
			}
			if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
				t.Fatalf("retry: %v", err)
			}
			row, exists := rows.rows[view.Key]
			if tc.policy == 0 {
				if exists {
					t.Fatal("retry kept an image with immediate policy retention")
				}
			} else if !exists {
				t.Fatal("retry permanently purged an image with positive policy retention")
			} else if row.State != model.ImageStateTrash || row.Operation != "" || row.PurgeAt == nil || !row.PurgeAt.Equal(deadline) {
				t.Fatal("retry changed the recorded retention or left an incomplete move")
			}
			if len(asked) != 2 {
				t.Fatal("retry recalculated plugin policy instead of using the recorded deadline")
			}
		})
	}
}
