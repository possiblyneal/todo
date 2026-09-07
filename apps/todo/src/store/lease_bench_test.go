package store

import (
	"path/filepath"
	"testing"
)

// BenchmarkGuardedWriteOnALargeTree measures one guarded write at depth 5
// against 10,000 Tasks. No number is claimed as passing: the point is that a
// guarded write on a large tree stays cheap, and what "cheap" measures at
// depends on the machine running it.
//
// What it is watching for is a shape, not a figure. The predicate must find
// the tree's root and then look the Lease up by its primary key. Joining
// `lease` against the walk instead re-runs the recursion per Lease row, and
// that reads as an order of magnitude on the same fixture rather than as a
// few percent.
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
		root, err := s.AddTask("alice", Attributes{Title: Set("root")})
		if err != nil {
			b.Fatalf("AddTask: %v", err)
		}
		if _, err := s.TakeLease("alice", root, hour); err != nil {
			b.Fatalf("TakeLease: %v", err)
		}
		id := root
		for range depth - 1 {
			if id, err = s.AddSubtask("alice", id, Attributes{Title: Set("level")}); err != nil {
				b.Fatalf("AddSubtask: %v", err)
			}
		}
		if r == roots-1 {
			deepest = id
		}
	}

	b.ResetTimer()
	for i := range b.N {
		if err := s.EditTask("alice", deepest, Attributes{Title: Set("edited")}); err != nil {
			b.Fatalf("EditTask %d: %v", i, err)
		}
	}
}
