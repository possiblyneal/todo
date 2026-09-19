package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// browsed runs one request against the whole handler rooted at a tree of this
// test's own, which is what keeps the listing off the machine the tests run on.
func browsed(t *testing.T, root, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/files"+query, nil)
	w := httptest.NewRecorder()
	Handler(openTemp(t), Options{Browse: root}).ServeHTTP(w, r)
	return w
}

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// EvalSymlinks because a temp directory is under /tmp, which is a symlink
	// on some machines: the route answers resolved paths and the test compares
	// against them.
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	for _, dir := range []string{"notes", "zzz"} {
		if err := os.Mkdir(filepath.Join(real, dir), 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
	}
	for _, name := range []string{"a.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(real, name), nil, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return real
}

func decodeFiles(t *testing.T, w *httptest.ResponseRecorder) filesBody {
	t.Helper()
	var body filesBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return body
}

func TestBrowsingListsDirectoriesFirstAndHidesNothing(t *testing.T) {
	root := tree(t)

	w := browsed(t, root, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	body := decodeFiles(t, w)
	if body.Path != root {
		t.Errorf("path = %q, want %q", body.Path, root)
	}
	// Nowhere further up: the root is as far as this listener looks, and an
	// empty parent is what says so.
	if body.Parent != "" {
		t.Errorf("parent = %q, want empty at the root", body.Parent)
	}
	want := []listed{
		{Name: "notes", Dir: true},
		{Name: "zzz", Dir: true},
		{Name: ".hidden", Dir: false},
		{Name: "a.txt", Dir: false},
	}
	if len(body.Entries) != len(want) {
		t.Fatalf("entries = %v, want %v", body.Entries, want)
	}
	for i, one := range want {
		if body.Entries[i] != one {
			t.Errorf("entry %d = %v, want %v", i, body.Entries[i], one)
		}
	}
}

func TestBrowsingADirectoryUnderTheRootSaysWhereToGoBack(t *testing.T) {
	root := tree(t)

	body := decodeFiles(t, browsed(t, root, "?path=notes"))
	if body.Path != filepath.Join(root, "notes") {
		t.Errorf("path = %q, want the notes directory", body.Path)
	}
	if body.Parent != root {
		t.Errorf("parent = %q, want %q", body.Parent, root)
	}
	if len(body.Entries) != 0 {
		t.Errorf("entries = %v, want none", body.Entries)
	}
}

// Containment is by where a path lands rather than by how it is spelled, so a
// symlink out of the root is refused the same way `..` is.
func TestBrowsingRefusesAnythingOutsideTheRoot(t *testing.T) {
	root := tree(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "away")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	for _, asked := range []string{"..", outside, "away"} {
		w := browsed(t, root, "?path="+asked)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status for %q = %d, want 400 (%s)", asked, w.Code, w.Body.String())
		}
	}
}

func TestBrowsingRefusesAPathThatNamesNothing(t *testing.T) {
	w := browsed(t, tree(t), "?path=nowhere")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// The root is resolved the way the paths judged against it are, so a listener
// pointed at a symlink lists what it points at rather than refusing all of it.
func TestBrowsingARootReachedThroughASymlink(t *testing.T) {
	root := tree(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	body := decodeFiles(t, browsed(t, link, "?path=notes"))
	if body.Path != filepath.Join(root, "notes") {
		t.Errorf("path = %q, want the notes directory", body.Path)
	}
	if body.Parent != root {
		t.Errorf("parent = %q, want %q", body.Parent, root)
	}
}

// A symlink says it is a symlink rather than what it points at, and a home
// directory holds enough of them that a picker treating one as a file cannot
// reach half the machine. A dangling one is a file: there is nothing to open.
func TestBrowsingFollowsASymlinkToSayWhetherItIsADirectory(t *testing.T) {
	root := tree(t)
	if err := os.Symlink(filepath.Join(root, "notes"), filepath.Join(root, "to-notes")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	dirs := map[string]bool{}
	for _, one := range decodeFiles(t, browsed(t, root, "")).Entries {
		dirs[one.Name] = one.Dir
	}
	if !dirs["to-notes"] {
		t.Errorf("to-notes = file, want a directory: a link to one is somewhere to go")
	}
	if dirs["dangling"] {
		t.Errorf("dangling = directory, want a file: there is nothing to go into")
	}
}

// Everything outside the root is refused in one sentence. Two of them would
// answer whether an arbitrary path on the host exists to anybody who can reach
// the route.
func TestBrowsingSaysTheSameThingAboutEveryPathOutsideTheRoot(t *testing.T) {
	root := tree(t)
	outside := t.TempDir()

	there := browsed(t, root, "?path="+outside)
	absent := browsed(t, root, "?path="+filepath.Join(outside, "nothing-here"))
	if there.Code != http.StatusBadRequest || absent.Code != http.StatusBadRequest {
		t.Fatalf("statuses = %d and %d, want 400 each", there.Code, absent.Code)
	}
	// The path asked for is in each sentence, so they differ by that and by
	// nothing else: what is compared is the sentence with it taken out.
	said := func(w *httptest.ResponseRecorder, asked string) string {
		return strings.Replace(w.Body.String(), asked, "<asked>", 1)
	}
	if said(there, outside) != said(absent, filepath.Join(outside, "nothing-here")) {
		t.Errorf("a path that is there says %s; one that is not says %s", there.Body, absent.Body)
	}
}
