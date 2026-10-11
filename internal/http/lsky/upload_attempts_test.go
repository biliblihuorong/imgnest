package lsky_test

import (
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// A rejected v1 body spends the per-minute allowance too, so a token holder
// cannot repeat malformed or oversized uploads without limit.
func TestV1RejectedUploadsSpendPerMinuteAllowance(t *testing.T) {
	fixture := newV1Fixture(t, "sqlite")
	if err := fixture.db.Model(&model.Group{}).Where("id = ?", fixture.alice.GroupID).Update("upload_per_min", 1).Error; err != nil {
		t.Fatal(err)
	}
	token := fixture.login(t)
	malformed := fixture.upload(t, "", pngBytes(t, 8, 8), token)
	mustStatus(t, malformed, 200)
	throttled := fixture.upload(t, "a.png", pngBytes(t, 8, 8), token)
	mustStatus(t, throttled, 429)
}
