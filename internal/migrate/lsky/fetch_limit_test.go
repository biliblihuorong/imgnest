package lsky

import "testing"

func TestFetchLimitCoversTheLargestGroupLimit(t *testing.T) {
	db, _ := target(t)
	snap := Snapshot{Groups: []Group{{ID: 1, Name: "默认组", MaxFileKB: 5120}}}
	if got, err := FetchLimit(t.Context(), db, snap, 100<<20); err != nil || got != 100<<20 {
		t.Fatalf("default limit = %d, %v", got, err)
	}
	// An exported Lsky group becomes an ImgNest group with its own limit.
	snap.Groups = append(snap.Groups, Group{ID: 2, Name: "大图", MaxFileKB: 300 * 1024})
	if got, err := FetchLimit(t.Context(), db, snap, 100<<20); err != nil || got != 300<<20 {
		t.Fatalf("export group limit = %d, %v", got, err)
	}
	if err := db.Exec(`INSERT INTO groups (name, max_file_bytes) VALUES ('raw', ?)`, int64(500<<20)).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := FetchLimit(t.Context(), db, snap, 100<<20); err != nil || got != 500<<20 {
		t.Fatalf("ImgNest group limit = %d, %v", got, err)
	}
}
