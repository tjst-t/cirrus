package config

import (
	"testing"
)

func TestParseAuthTokens(t *testing.T) {
	tests := []struct {
		input string
		want  map[string]string
	}{
		{"", nil},
		{"tok1=user1", map[string]string{"tok1": "user1"}},
		{"tok1=user1,tok2=user2", map[string]string{"tok1": "user1", "tok2": "user2"}},
	}

	for _, tt := range tests {
		got := ParseAuthTokens(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("ParseAuthTokens(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParseAuthTokens(%q) has %d entries, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("ParseAuthTokens(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}
