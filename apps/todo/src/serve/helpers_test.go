package serve

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

// screen collects what the far end drew. The renderer writes from its own
// goroutine while the test reads, so this is guarded.
type screen struct {
	mu sync.Mutex
	b  []byte
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.b)
}

// waitFor polls until the condition holds. A session is two processes' worth
// of asynchrony even in one process: nothing here happens on the test's call.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(what)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// escapes are the control sequences a renderer writes around what it draws.
// They cost no cells, so a width measured with them in is not a width.
var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b[]P][^\x07\x1b]*(\x07|\x1b\\)?|\x1b[()][A-Za-z0-9]|\x1b[=>()#][0-9A-Za-z]?`)

// widest is the longest line the far end drew, in cells.
func widest(s string) int {
	got := 0
	for _, line := range strings.Split(escapes.ReplaceAllString(s, ""), "\n") {
		got = max(got, lipgloss.Width(strings.TrimRight(line, "\r")))
	}
	return got
}

func (s *screen) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b = nil
}
