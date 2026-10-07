package cli

import (
	"bytes"
	"io/fs"
	"os"
	"testing"
)

func TestFrontendDistFS(t *testing.T) {
	t.Parallel()
	assertFrontendDistFS(t, "../../web-vben/dist")
}

// The embedded filesystem must contain exactly the built frontend's files, with
// dist/ stripped.
func assertFrontendDistFS(t *testing.T, directory string) {
	t.Helper()
	got, err := frontendDistFS()
	if err != nil {
		t.Fatalf("open frontend: %v", err)
	}
	want := os.DirFS(directory)
	var wantPaths []string
	if err := fs.WalkDir(want, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		wantPaths = append(wantPaths, path)
		wantContent, err := fs.ReadFile(want, path)
		if err != nil {
			return err
		}
		gotContent, err := fs.ReadFile(got, path)
		if err != nil {
			t.Errorf("selected frontend missing %s: %v", path, err)
			return nil
		}
		if !bytes.Equal(gotContent, wantContent) {
			t.Errorf("selected frontend has wrong content for %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk expected frontend: %v", err)
	}
	if len(wantPaths) == 0 {
		t.Fatal("frontend fixture must include at least dist/.gitkeep")
	}
	gotFiles := 0
	if err := fs.WalkDir(got, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			gotFiles++
			if _, err := fs.Stat(want, path); err != nil {
				t.Errorf("unexpected embedded frontend file %s: %v", path, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk selected frontend: %v", err)
	}
	if gotFiles != len(wantPaths) {
		t.Errorf("selected frontend has %d files, want %d", gotFiles, len(wantPaths))
	}
}
