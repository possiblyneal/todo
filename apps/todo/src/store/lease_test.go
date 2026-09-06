package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const hour = time.Hour

// A Lease and a top-level Task to hang it on.
func taskFor(t *testing.T, s *Store, actor, title string) string {
	t.Helper()
	id, err := s.AddTask(actor, title)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	return id
}

func TestWriteWithNoLeaseIsRefused(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")

	if err := s.DescribeTask("alice", id, "Buy oat milk"); !errors.Is(err, ErrRefused) {
		t.Fatalf("DescribeTask with no Lease = %v, want ErrRefused", err)
	}
	// Refused, not silently applied.
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if tasks[0].Title != "Buy milk" {
		t.Errorf("the refused edit was applied anyway: title is %q", tasks[0].Title)
	}
}

func TestWriteUnderOwnLeaseIsAllowed(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")

	if _, err := s.TakeLease("alice", id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	if err := s.DescribeTask("alice", id, "Buy oat milk"); err != nil {
		t.Fatalf("DescribeTask under own Lease: %v", err)
	}
	tasks, _ := s.Tasks()
	if tasks[0].Title != "Buy oat milk" {
		t.Errorf("title is %q, want the edit applied", tasks[0].Title)
	}
}

func TestAnotherActorsLeaseRefusesBothTheLeaseAndTheWrite(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")
	if _, err := s.TakeLease("alice", id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	_, err := s.TakeLease("agent-7", id, hour)
	if !errors.Is(err, ErrHeld) {
		t.Errorf("TakeLease against a held Lease = %v, want ErrHeld", err)
	}
	// Symmetric: the refusal does not name the holder, and gives no way to
	// tell a person's Lease from an Agent's.
	if err != nil && strings.Contains(err.Error(), "alice") {
		t.Errorf("ErrHeld named the holder: %v", err)
	}
	if err := s.DescribeTask("agent-7", id, "Buy oat milk"); !errors.Is(err, ErrRefused) {
		t.Errorf("DescribeTask against another Actor's Lease = %v, want ErrRefused", err)
	}
}

func TestAnExpiredLeaseGuardsNothingAndIsNotSweptAway(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")
	if _, err := s.TakeLease("alice", id, -time.Second); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	if err := s.DescribeTask("alice", id, "Buy oat milk"); !errors.Is(err, ErrRefused) {
		t.Errorf("DescribeTask under an expired Lease = %v, want ErrRefused", err)
	}
	// No reader deletes anything: expiry is a clause, not a sweep.
	if _, err := s.Tasks(); err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	var rows int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lease`).Scan(&rows); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	if rows != 1 {
		t.Errorf("the expired Lease row was deleted by a read; %d rows remain, want 1", rows)
	}
}

func TestRetakingAStaleLeaseRecordsItBroken(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")
	if _, err := s.TakeLease("alice", id, -time.Second); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	if _, err := s.TakeLease("agent-7", id, hour); err != nil {
		t.Fatalf("taking over a stale Lease: %v", err)
	}
	if err := s.DescribeTask("agent-7", id, "Buy oat milk"); err != nil {
		t.Fatalf("DescribeTask under the taken-over Lease: %v", err)
	}

	kinds := historyKinds(t, s)
	want := []string{KindTaskAdded, KindLeaseTaken, KindLeaseBroken, KindLeaseTaken, KindTaskDescribed}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("the Change History reads %v, want %v", kinds, want)
	}
	// Expiry is not an event; only the taking over is.
	for _, k := range kinds {
		if k == "lease_expired" {
			t.Error("a Lease Expired entry was appended; expiry is a condition, not an event")
		}
	}
}

func TestOwnUnexpiredLeaseIsExtendedNotBroken(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")
	first, err := s.TakeLease("alice", id, time.Minute)
	if err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	second, err := s.TakeLease("alice", id, hour)
	if err != nil {
		t.Fatalf("re-taking own Lease: %v", err)
	}
	if !second.ExpiresAt.After(first.ExpiresAt) {
		t.Errorf("re-taking own Lease left expiry at %v, want it extended past %v", second.ExpiresAt, first.ExpiresAt)
	}
	for _, k := range historyKinds(t, s) {
		if k == KindLeaseBroken {
			t.Error("re-taking one's own unexpired Lease recorded it Broken")
		}
	}
}

func TestReleasedLeaseStopsGuardingWrites(t *testing.T) {
	s := openTemp(t)
	id := taskFor(t, s, "alice", "Buy milk")
	if _, err := s.TakeLease("alice", id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	if err := s.ReleaseLease("alice", id); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := s.DescribeTask("alice", id, "Buy oat milk"); !errors.Is(err, ErrRefused) {
		t.Errorf("DescribeTask after release = %v, want ErrRefused", err)
	}
	// The next writer takes it cleanly, with nothing to break.
	if _, err := s.TakeLease("agent-7", id, hour); err != nil {
		t.Errorf("TakeLease after a release: %v", err)
	}
}

// A Lease covers a top-level Task and its whole nested tree. It is never keyed
// on a Subtask, and that is enforced on write.
func TestALeaseOnASubtaskIsRejected(t *testing.T) {
	s := openTemp(t)
	root := taskFor(t, s, "alice", "Ship the thing")
	if _, err := s.TakeLease("alice", root, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	child, err := s.AddSubtask("alice", root, "Write the code")
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}

	_, err = s.TakeLease("alice", child, hour)
	if err == nil {
		t.Fatal("a Lease on a Subtask was accepted")
	}
	if !strings.Contains(err.Error(), "a Lease covers a top-level Task, not a subtask") {
		t.Errorf("TakeLease on a Subtask failed with %v, want the domain's own words", err)
	}
}

// The Lease covers the whole tree, so the root's Lease is what a write five
// levels down needs. This is the blocking cost ADR 0002 accepted deliberately.
func TestTheRootsLeaseGuardsTheWholeTree(t *testing.T) {
	s := openTemp(t)
	root := taskFor(t, s, "alice", "Ship the thing")
	if _, err := s.TakeLease("alice", root, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}

	id := root
	for depth := 1; depth < 5; depth++ {
		var err error
		if id, err = s.AddSubtask("alice", id, "level"); err != nil {
			t.Fatalf("AddSubtask at depth %d: %v", depth, err)
		}
	}
	if err := s.DescribeTask("alice", id, "the deepest one"); err != nil {
		t.Fatalf("editing at depth 5 under the root's Lease: %v", err)
	}
	// And another Actor is blocked that far down by the same Lease.
	if err := s.DescribeTask("agent-7", id, "not yours"); !errors.Is(err, ErrRefused) {
		t.Errorf("an unleased edit at depth 5 = %v, want ErrRefused", err)
	}
	// Adding a Subtask is a write to the tree, and needs the Lease too.
	if _, err := s.AddSubtask("agent-7", root, "not yours either"); !errors.Is(err, ErrRefused) {
		t.Errorf("an unleased AddSubtask = %v, want ErrRefused", err)
	}
}

func historyKinds(t *testing.T, s *Store) []string {
	t.Helper()
	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	kinds := make([]string, len(entries))
	for i, e := range entries {
		kinds[i] = e.Kind
	}
	return kinds
}
