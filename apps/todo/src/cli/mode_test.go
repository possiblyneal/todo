package cli

import "testing"

func TestModeOf(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Mode
	}{
		{"bare todo opens the TUI", nil, ModeTUI},
		{"serve listens on SSH", []string{"serve"}, ModeServe},
		{"a verb acts and exits", []string{"add", "Buy milk"}, ModeVerb},
		{"serve with flags is still serve", []string{"serve", "--port", "2222"}, ModeServe},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModeOf(c.args); got != c.want {
				t.Errorf("ModeOf(%q) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
