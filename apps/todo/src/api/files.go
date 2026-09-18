package api

// The one route that reads anything outside the store, and the narrowest one
// that could answer the question it exists for.
//
// A pointer naming a file names it on the machine `todo api` runs on, because
// that is the machine `store.pointer` resolves a relative path against. A
// browser's own file input cannot help with that: it answers with a bare
// filename and no directory, so a file chosen on a phone is a path the host
// cannot resolve and one chosen at the desk resolves only by luck. So the
// picker browses the host, and this is what it reads.
//
// It lists and never opens. The tracker holds pointers and keeps no copy of
// what they point at, and a route serving file contents would be the second
// thing this repo has said it does not do.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// browse answers the entries of one directory under the root, or says why not.
func browse(root string, w http.ResponseWriter, r *http.Request) {
	if root == "" {
		fail(w, fmt.Errorf("this listener cannot browse: it could not find a home directory to browse from"))
		return
	}
	at, err := within(root, r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	read, err := os.ReadDir(at)
	if err != nil {
		fail(w, usage{fmt.Errorf("cannot list %s: %w", at, err)})
		return
	}

	out := filesBody{Path: at, Entries: make([]listed, 0, len(read))}
	if at != root {
		out.Parent = filepath.Dir(at)
	}
	for _, each := range read {
		dir := each.IsDir()
		// A symlink says it is a symlink and not what it points at, and a home
		// directory holds enough of them pointing at directories that a picker
		// refusing to open one is a picker that cannot reach half the machine.
		// Following it here decides nothing: opening it comes back through the
		// same containment every other path does.
		if each.Type()&os.ModeSymlink != 0 {
			if stat, err := os.Stat(filepath.Join(at, each.Name())); err == nil {
				dir = stat.IsDir()
			}
		}
		out.Entries = append(out.Entries, listed{Name: each.Name(), Dir: dir})
	}
	// Directories first, so somewhere to go is not mixed in with somewhere to
	// stop. Nothing is hidden: a dotfile left out is a picker that cannot reach
	// a config directory, and this is somebody browsing their own machine.
	sort.SliceStable(out.Entries, func(i, j int) bool {
		if out.Entries[i].Dir != out.Entries[j].Dir {
			return out.Entries[i].Dir
		}
		return out.Entries[i].Name < out.Entries[j].Name
	})

	send(w, http.StatusOK, out)
}

// within resolves what was asked for against the root and refuses anything
// outside it. Symlinks are followed first: a path is judged by where it lands
// rather than by how it is spelled, so looking for `..` in the text would miss
// a link out of the root and turn down a directory honestly named `..foo`.
func within(root, asked string) (string, error) {
	if strings.TrimSpace(asked) == "" {
		return root, nil
	}
	if !filepath.IsAbs(asked) {
		asked = filepath.Join(root, asked)
	}
	at, err := filepath.EvalSymlinks(filepath.Clean(asked))
	if err != nil {
		return "", usage{fmt.Errorf("cannot list %s: %w", asked, err)}
	}
	rel, err := filepath.Rel(root, at)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", usage{fmt.Errorf("%s is outside %s, which is as far as this listener will look", asked, root)}
	}
	return at, nil
}

// home is the root a listener browses from, resolved once. Symlinks are taken
// off it here so every path compared against it below is compared against what
// it really is; an empty answer is a listener that cannot browse, and says so
// when asked rather than at startup.
func home() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	return real
}

// filesBody is one directory as the picker reads it. Parent is empty at the
// root, which is what says there is nowhere further up to go.
type filesBody struct {
	Path    string   `json:"path"`
	Parent  string   `json:"parent"`
	Entries []listed `json:"entries"`
}

// listed is one name in it. Whether it is a directory is the whole of what a
// picker needs: one is somewhere to go and the other is something to point at.
type listed struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}
