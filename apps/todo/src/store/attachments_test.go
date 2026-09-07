package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An Attachment is a pointer, and both kinds of pointer are the same kind of
// thing to the store: a file path and a web address are one list.
func TestAnAttachmentIsAPointerOfEitherKind(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("File the accounts")})

	file := filepath.Join(t.TempDir(), "receipts.pdf")
	if err := os.WriteFile(file, []byte("%PDF"), 0o600); err != nil {
		t.Fatalf("writing the file to point at: %v", err)
	}
	for _, target := range []string{file, "https://example.com/policy"} {
		if err := s.Attach("alice", id, target); err != nil {
			t.Fatalf("Attach %q: %v", target, err)
		}
	}

	got := only(t, s, Query{}).Attachments
	if len(got) != 2 || got[0] != file || got[1] != "https://example.com/policy" {
		t.Errorf("the Task carries %v", got)
	}
}

// Nothing is copied. What the store holds is the text of the pointer and the
// moment it was added, and the bytes stay where they were.
func TestNothingIsCopiedIn(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Read the report")})

	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	body := "the whole of the report, which is not the tracker's to hold"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	if err := s.Attach("alice", id, file); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Every cell of the database, read as text. The pointer is in there; the
	// bytes it points at are not.
	rows, err := s.db.Query(`SELECT payload FROM change_history`)
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(payload, body) {
			t.Fatalf("the file's contents were copied into the record: %s", payload)
		}
	}
}

// A dead link is a dead link. The store points at what it was given and never
// looks, so attaching something that is not there is not an error, and neither
// is deleting it afterwards.
func TestADeadLinkIsAcceptedAndNeverChecked(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Find the thing")})

	gone := filepath.Join(t.TempDir(), "moved-away.txt")
	if err := os.WriteFile(gone, nil, 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	if err := s.Attach("alice", id, gone); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatalf("removing the file: %v", err)
	}
	if got := only(t, s, Query{}).Attachments; len(got) != 1 || got[0] != gone {
		t.Errorf("the pointer changed when its target went: %v", got)
	}

	// And one that never existed attaches just the same.
	never := filepath.Join(t.TempDir(), "never-existed.txt")
	if err := s.Attach("alice", id, never); err != nil {
		t.Errorf("attaching a pointer to nothing: %v", err)
	}
}

// Deleting a Task collects nothing. The Task is marked deleted, an appended
// entry like any other, and what it pointed at is untouched.
func TestDeletingATaskCollectsNothing(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Throw it out")})

	file := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(file, []byte("still here"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	if err := s.Attach("alice", id, file); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := s.DeleteTask("alice", id); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("deleting the Task reached the file: %v", err)
	}
	if got := only(t, s, Query{IncludeDeleted: true}).Attachments; len(got) != 1 {
		t.Errorf("a deleted Task lost its pointers: %v", got)
	}
}

// Attaching is a write to the Task, so it goes through the same guard as any
// other, and detaching does too.
func TestAttachingNeedsTheLease(t *testing.T) {
	s := openTemp(t)
	id, err := s.AddTask("alice", Attributes{Title: Set("Sign the lease")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := s.Attach("alice", id, "https://example.com"); !errors.Is(err, ErrRefused) {
		t.Errorf("attaching without a Lease: %v", err)
	}
	if _, err := s.TakeLease("bob", id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	// The guard is symmetric and says only that the write needs a Lease this
	// Actor holds, which is what a Lease held by somebody else amounts to.
	if err := s.Attach("alice", id, "https://example.com"); !errors.Is(err, ErrRefused) {
		t.Errorf("attaching over another Actor's Lease: %v", err)
	}
	if err := s.Attach("bob", id, "https://example.com"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := s.Detach("alice", id, "https://example.com"); !errors.Is(err, ErrRefused) {
		t.Errorf("detaching over another Actor's Lease: %v", err)
	}
	if err := s.Detach("bob", id, "https://example.com"); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if got := only(t, s, Query{}).Attachments; len(got) != 0 {
		t.Errorf("the pointer survived detaching: %v", got)
	}
}

// The same pointer twice is one pointer, and a relative path is resolved
// where it was typed, so it means the same thing read from anywhere else.
func TestAPointerIsWrittenDownOnceAndInFull(t *testing.T) {
	s := openTemp(t)
	id := leased(t, s, "alice", Attributes{Title: Set("Tidy up")})

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("notes.md", nil, 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	for range 2 {
		if err := s.Attach("alice", id, "notes.md"); err != nil {
			t.Fatalf("Attach: %v", err)
		}
	}
	got := only(t, s, Query{}).Attachments
	if len(got) != 1 {
		t.Fatalf("the same pointer twice reads back as %v", got)
	}
	if !filepath.IsAbs(got[0]) || filepath.Base(got[0]) != "notes.md" {
		t.Errorf("a relative path was kept relative: %q", got[0])
	}
	if err := s.Attach("alice", id, "  "); err == nil {
		t.Error("a blank pointer was accepted")
	}
}
