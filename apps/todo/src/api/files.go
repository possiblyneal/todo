package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// browse answers the entries of one directory under the root, or says why not.
// It is the one route reading anything outside the store, and it lists rather
// than opens: the tracker holds pointers and keeps no copy of what they point
// at, so a route serving file contents would be the second thing this repo has
// said it does not do.
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
	slices.SortStableFunc(out.Entries, func(a, b listed) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	send(w, http.StatusOK, out)
}

// within resolves what was asked for against the root and refuses anything
// outside it. It is judged twice: on how it is spelled, and then on where it
// lands, because looking for `..` in the text alone would miss a link out of
// the root and would turn down a directory honestly named `..foo`.
//
// The spelling is judged first so that a path outside the root is refused
// before the machine is touched. Resolving it first and reporting what that
// said would answer whether an arbitrary absolute path exists, and whether its
// parent can be read, to anybody who can reach this route — which is a
// question about the host rather than about the tracker. Everything outside
// gets the one sentence, whether it is there or not. Inside the root, why a
// path cannot be listed is said plainly: that is a directory the caller could
// have listed anyway.
func within(root, asked string) (string, error) {
	if strings.TrimSpace(asked) == "" {
		return root, nil
	}
	if !filepath.IsAbs(asked) {
		asked = filepath.Join(root, asked)
	}
	clean := filepath.Clean(asked)
	if !under(root, clean) {
		return "", outside(asked, root)
	}
	at, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", usage{fmt.Errorf("cannot list %s: %w", asked, err)}
	}
	if !under(root, at) {
		return "", outside(asked, root)
	}
	return at, nil
}

// under is the containment comparison, on the path boundary rather than on the
// characters: a root of `/home/ne` does not contain `/home/neal`.
func under(root, at string) bool {
	rel, err := filepath.Rel(root, at)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// outside is the one sentence every path the listener will not look at gets.
func outside(asked, root string) error {
	return usage{fmt.Errorf("%s is outside %s, which is as far as this listener will look", asked, root)}
}

// rooted resolves the root the same way `within` resolves what it judges.
// Both sides of that comparison have to be the same kind of path, or a root
// reached through a symlink would refuse every path under itself. One that
// cannot be resolved is left as it was given, which refuses rather than
// widens.
func rooted(dir string) string {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return real
}

// home is where a listener browses from when it was told nowhere else. An
// empty answer is a listener that cannot browse, and says so when asked rather
// than at startup.
func home() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
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
