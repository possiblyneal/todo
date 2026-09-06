package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDispatchesEveryMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bare todo", nil, "tui"},
		{"a verb", []string{"add", "Buy milk"}, "verb"},
		{"serve", []string{"serve"}, "serve"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := Run(c.args, &out, &errOut); code != 0 {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %s", c.args, code, errOut.String())
			}
			if !strings.Contains(out.String(), c.want) {
				t.Errorf("Run(%q) wrote %q, want it to name mode %q", c.args, out.String(), c.want)
			}
		})
	}
}
