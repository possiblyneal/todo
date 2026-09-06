package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

const (
	envChildDB    = "TODO_TEST_CHILD_DB"
	envChildActor = "TODO_TEST_CHILD_ACTOR"
	envChildCount = "TODO_TEST_CHILD_COUNT"
)

// TestMain turns this test binary into the child process the concurrency test
// spawns. Four real OS processes are the point: goroutines share one *sql.DB
// and one pool, and would not exercise the file lock the deferred-BEGIN defect
// hides behind.
func TestMain(m *testing.M) {
	if path := os.Getenv(envChildDB); path != "" {
		os.Exit(appendFromChild(path))
	}
	os.Exit(m.Run())
}

func appendFromChild(path string) int {
	actor := os.Getenv(envChildActor)
	count, err := strconv.Atoi(os.Getenv(envChildCount))
	if err != nil {
		fmt.Fprintf(os.Stderr, "child: %v\n", err)
		return 2
	}
	s, err := Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "child %s: %v\n", actor, err)
		return 1
	}
	defer func() { _ = s.Close() }()

	for i := range count {
		if _, err := s.AddTask(actor, fmt.Sprintf("%s task %d", actor, i)); err != nil {
			fmt.Fprintf(os.Stderr, "child %s task %d: %v\n", actor, i, err)
			return 1
		}
	}
	return 0
}

func TestConcurrentProcessesAppendGaplesslyAndFoldExactly(t *testing.T) {
	const (
		writers = 4
		each    = 100
		want    = writers * each
	)

	path := filepath.Join(t.TempDir(), "todo.db")
	// Create the schema once up front so the children race on appends rather
	// than on CREATE TABLE.
	seed, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}

	done := make(chan error, writers)
	for w := range writers {
		go func() {
			cmd := exec.Command(self)
			cmd.Env = append(os.Environ(),
				envChildDB+"="+path,
				envChildActor+"=agent-"+strconv.Itoa(w),
				envChildCount+"="+strconv.Itoa(each),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				err = fmt.Errorf("writer %d: %w: %s", w, err, out)
			}
			done <- err
		}()
	}
	for range writers {
		if err := <-done; err != nil {
			t.Fatalf("%v", err)
		}
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	entries, err := s.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != want {
		t.Errorf("the Change History holds %d entries, want %d: appends were lost", len(entries), want)
	}
	// Gapless: max(seq) = count, and every position between is occupied.
	for i, e := range entries {
		if e.Seq != int64(i+1) {
			t.Fatalf("entry %d carries seq %d: the sequence has a gap", i, e.Seq)
		}
	}

	// An exact fold, with no drift: one Task per Task Added, and no Task the
	// Change History cannot account for.
	tasks, err := s.Tasks()
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != len(entries) {
		t.Errorf("the fold holds %d tasks against %d entries: the fold drifted", len(tasks), len(entries))
	}
	subjects := make(map[string]bool, len(entries))
	for _, e := range entries {
		subjects[e.Subject] = true
	}
	for _, task := range tasks {
		if !subjects[task.ID] {
			t.Errorf("task %s is in the fold with no entry appending it", task.ID)
		}
	}
}
