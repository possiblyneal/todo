package cli

import "testing"

func TestModeOf(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Mode
	}{
		{"bare todo says what it is", nil, ModeUsage},
		{"api listens on HTTP", []string{"api"}, ModeAPI},
		{"a verb acts and exits", []string{"add", "Buy milk"}, ModeVerb},
		{"api with flags is still api", []string{"api", "--addr", ":9000"}, ModeAPI},
		// serve is gone, so the word is an unknown verb rather than a mode.
		{"serve is not a mode any more", []string{"serve"}, ModeVerb},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModeOf(c.args); got != c.want {
				t.Errorf("ModeOf(%q) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
