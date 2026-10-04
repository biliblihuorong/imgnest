package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localFixture(t *testing.T) (*Local, string) {
	t.Helper()
	root := t.TempDir()
	d, err := NewLocal(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	return d, root
}
func readObject(t *testing.T, d Driver, key string) string {
	t.Helper()
	r, _, err := d.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestLocalNoOverwriteAndDurableOwnership(t *testing.T) {
	d, root := localFixture(t)
	opts := PutOptions{OwnerID: "image-one", MIME: "image/png"}
	got, err := d.PutNew(t.Context(), "2026/旅行.png", strings.NewReader("first"), opts)
	if err != nil || got.Size != 5 || got.OwnerID != opts.OwnerID {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := d.PutNew(t.Context(), got.Key, strings.NewReader("second"), opts); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	if readObject(t, d, got.Key) != "first" {
		t.Fatal("overwritten")
	}
	other, err := NewLocal(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	}()
	info, err := other.Stat(t.Context(), got.Key)
	if err != nil || info.OwnerID != "image-one" || info.Size != 5 || info.MIME != "image/png" {
		t.Fatalf("metadata %+v %v", info, err)
	}
}

type brokenSeeker struct{ *bytes.Reader }

func (b brokenSeeker) Read([]byte) (int, error) { return 0, errors.New("private-reader-payload") }
func TestLocalFailedWriteAndCancellation(t *testing.T) {
	d, _ := localFixture(t)
	if _, err := d.PutNew(t.Context(), "broken.jpg", brokenSeeker{bytes.NewReader([]byte("x"))}, PutOptions{OwnerID: "one"}); err == nil || strings.Contains(err.Error(), "private-reader-payload") {
		t.Fatalf("unsafe error %v", err)
	}
	if _, err := d.Stat(t.Context(), "broken.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.PutNew(ctx, "cancel.jpg", strings.NewReader("x"), PutOptions{OwnerID: "one"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestLocalEscapeAndForeignFiles(t *testing.T) {
	d, root := localFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "victim"), []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../victim", "/victim", "escape/victim", "a\\b", "a/../b"} {
		if _, err := d.PutNew(t.Context(), key, strings.NewReader("x"), PutOptions{OwnerID: "one"}); err == nil {
			t.Fatalf("accepted %q", key)
		}
	}
	outsideRoot, err := os.OpenRoot(outside)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := outsideRoot.Close(); err != nil {
			t.Error(err)
		}
	}()
	b, err := outsideRoot.ReadFile("victim")
	if err != nil || string(b) != "unchanged" {
		t.Fatal("escaped root")
	}
	if err := os.WriteFile(filepath.Join(root, "foreign"), []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteCurrent(t.Context(), "foreign"); !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
}
func TestLocalCopyOwnershipAndExactPurge(t *testing.T) {
	d, _ := localFixture(t)
	opts := PutOptions{OwnerID: "one", MIME: "image/jpeg"}
	for _, key := range []string{"a.jpg", "a.jpg-neighbor"} {
		if _, err := d.PutNew(t.Context(), key, strings.NewReader("bytes"), opts); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := d.Copy(t.Context(), "a.jpg", "_trash/a.jpg", opts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Copy(t.Context(), "a.jpg", "_trash/a.jpg", CopyOptions{OwnerID: "other"}); !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
	if err := d.DeleteCurrent(t.Context(), "a.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Open(t.Context(), "a.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.PurgeAllVersions(t.Context(), "_trash/a.jpg"); err != nil {
			t.Fatal(err)
		}
	}
	if readObject(t, d, "a.jpg-neighbor") != "bytes" {
		t.Fatal("neighbor removed")
	}
}

func TestLocalPurgeOwnedPreservesForeignObject(t *testing.T) {
	d, _ := localFixture(t)
	if _, err := d.PutNew(t.Context(), "a.jpg", strings.NewReader("first"), PutOptions{OwnerID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeOwned(t.Context(), "a.jpg", "second"); !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
	if readObject(t, d, "a.jpg") != "first" {
		t.Fatal("foreign object deleted")
	}
	if err := d.PurgeOwned(t.Context(), "a.jpg", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Stat(t.Context(), "a.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestLocalConcurrentInstallDoesNotOverwrite(t *testing.T) {
	d, root := localFixture(t)
	second, err := NewLocal(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	}()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, driver := range []*Local{d, second} {
		go func() {
			<-start
			_, err := driver.PutNew(t.Context(), "same.jpg", strings.NewReader("samebytes"), PutOptions{OwnerID: "same-owner"})
			results <- err
		}()
	}
	close(start)
	success, exists := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrExists):
			exists++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || exists != 1 || readObject(t, d, "same.jpg") != "samebytes" {
		t.Fatal("concurrent object installation was not exclusive")
	}
}

func TestLocalCopyRejectsSameOwnerDifferentContent(t *testing.T) {
	d, _ := localFixture(t)
	opts := PutOptions{OwnerID: "one"}
	for key, body := range map[string]string{"source": "first", "target": "other"} {
		if _, err := d.PutNew(t.Context(), key, strings.NewReader(body), opts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Copy(t.Context(), "source", "target", opts); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	if readObject(t, d, "target") != "other" {
		t.Fatal("copy overwrote differing owned content")
	}
}

func TestLocalOwnedCleanupRecoversInterruptedStaging(t *testing.T) {
	d, _ := localFixture(t)
	const interrupted = ".imgnest-tmp-a85531e08110a2972f7942e59774793185990bb0f289dd7d148fee771948f613"
	if err := d.root.WriteFile(interrupted, []byte("interrupted header"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeOwned(t.Context(), "a.jpg", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.root.Lstat(interrupted); err != nil {
		t.Fatal("foreign cleanup removed staging")
	}
	if err := d.PurgeOwned(t.Context(), "a.jpg", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.root.Lstat(interrupted); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted owned staging remained")
	}
}

func TestLocalPurgeImageRequiresOwnership(t *testing.T) {
	d, _ := localFixture(t)
	if _, err := d.PutNew(t.Context(), "a.jpg", strings.NewReader("original"), PutOptions{OwnerID: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeImage(t.Context(), "a.jpg", "other"); !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
	if readObject(t, d, "a.jpg") != "original" {
		t.Fatal("foreign final purge changed image")
	}
	for range 2 {
		if err := d.PurgeImage(t.Context(), "a.jpg", "one"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Stat(t.Context(), "a.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatal("final image remains")
	}
}
