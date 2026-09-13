package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A store written before the spelling settled carries `colour`. Opening it
// renames the column, keeps what was in it, and folds the next edit into the
// column the schema now names.
func TestAStoreSpeltColourIsRenamedOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id, err := s.AddTask("alice", Attributes{Title: Set("Paint the shed"), Color: Set("green")})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if _, err := s.AddList("alice", "Home", "blue"); err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Put the old spelling back on. SQLite rewrites the trigger bodies that
	// name the column, so this is the shape an older build left behind.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	for _, table := range []string{"task", "list", "tag"} {
		if _, err := db.Exec("ALTER TABLE " + table + " RENAME COLUMN color TO colour"); err != nil {
			t.Fatalf("un-rename %s: %v", table, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer s.Close()

	tasks, err := s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Color != "green" {
		t.Fatalf("the color did not survive the rename: %+v", tasks)
	}
	lists, err := s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if len(lists) != 1 || lists[0].Color != "blue" {
		t.Fatalf("the List's color did not survive the rename: %+v", lists)
	}

	// The refreshed folds read the payload key the current build writes.
	if _, err := s.TakeLease("alice", id, hour); err != nil {
		t.Fatalf("TakeLease: %v", err)
	}
	if err := s.EditTask("alice", id, Attributes{Color: Set("red")}); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	tasks, err = s.Tasks(Query{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if tasks[0].Color != "red" {
		t.Errorf("the edit folded to %q, want %q", tasks[0].Color, "red")
	}
	if err := s.DescribeList("alice", lists[0].ID, nil, Set("red")); err != nil {
		t.Fatalf("DescribeList: %v", err)
	}
	lists, err = s.Lists()
	if err != nil {
		t.Fatalf("Lists: %v", err)
	}
	if lists[0].Color != "red" {
		t.Errorf("the List's edit folded to %q, want %q", lists[0].Color, "red")
	}
}

// A color is one of the nine or it is nothing. Anything else is refused
// wherever one can be set, so a name that reached the store is one every
// surface knows how to paint.
func TestOnlyTheOfferedColorsAreTaken(t *testing.T) {
	s := openTemp(t)

	if len(Colors) != 9 {
		t.Errorf("%d colors are offered, want 9", len(Colors))
	}
	seen := map[string]string{}
	for _, c := range Colors {
		if was, twice := seen[c.ANSI]; twice {
			t.Errorf("%s and %s are the same color, %s", was, c.Name, c.ANSI)
		}
		seen[c.ANSI] = c.Name
		if _, err := s.AddTask("alice", Attributes{Title: Set(c.Name), Color: Set(c.Name)}); err != nil {
			t.Errorf("%s was refused: %v", c.Name, err)
		}
	}

	// The name is read whatever case it is typed in.
	if _, ok := ColorNamed("RED"); !ok {
		t.Error("RED is not red")
	}
	if _, err := s.AddTask("alice", Attributes{Title: Set("Paint"), Color: Set("banana")}); err == nil {
		t.Error("a color nothing can paint was taken")
	}
	if _, err := s.AddList("alice", "Home", "#ff8800"); err == nil {
		t.Error("a List took a color nothing can paint")
	}
	// Empty is how a color comes off.
	if _, err := s.AddTask("alice", Attributes{Title: Set("Plain"), Color: Set("")}); err != nil {
		t.Errorf("clearing a color was refused: %v", err)
	}
}
