package service

import (
	"context"
	"testing"
	"time"
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
