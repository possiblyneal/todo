package store

import (
	"path/filepath"
	"testing"
)

// BenchmarkGuardedWriteOnALargeTree measures one guarded write at depth 5
// against 10,000 Tasks: ~235µs, against the ~150-230µs the seam-contract
// harness recorded for the guard alone. This one also appends the entry and
// runs the fold, which is the difference.
//
// The predicate must find the tree's root and then look the Lease up by its
// primary key. Joining `lease` against the walk instead re-runs the recursion
// per Lease row, which measured 755µs against 69µs on this fixture -- a result
// in the hundreds of microseconds for the predicate alone means that
// regressed.
//
// It is a benchmark rather than a test because `go test ./...` runs on shared
// CI runners where a timing assertion measures the runner, not the predicate.
// Re-check it with: go test ./src/store/ -run x -bench GuardedWrite
func BenchmarkGuardedWriteOnALargeTree(b *testing.B) {
	const (
		roots = 2000
		depth = 5
	)

	s, err := Open(filepath.Join(b.TempDir(), "todo.db"))
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	var deepest string
	for r := range roots {
		root, err := s.AddTask("alice", "root")
		if err != nil {
			b.Fatalf("AddTask: %v", err)
		}
		if _, err := s.TakeLease("alice", root, hour); err != nil {
			b.Fatalf("TakeLease: %v", err)
		}
		id := root
		for range depth - 1 {
			if id, err = s.AddSubtask("alice", id, "level"); err != nil {
				b.Fatalf("AddSubtask: %v", err)
			}
		}
		if r == roots-1 {
			deepest = id
		}
	}

	b.ResetTimer()
	for i := range b.N {
		if err := s.DescribeTask("alice", deepest, "edited"); err != nil {
			b.Fatalf("DescribeTask %d: %v", i, err)
		}
	}
}
