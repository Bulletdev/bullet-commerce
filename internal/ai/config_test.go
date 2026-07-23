package ai

import "testing"

func TestConfigActive(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"off by default", Config{}, false},
		{"flag on but no key", Config{Enabled: true}, false},
		{"key but flag off", Config{APIKey: "sk-test"}, false},
		{"flag on and key present", Config{Enabled: true, APIKey: "sk-test"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Active(); got != tc.want {
				t.Fatalf("Active() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewClaudeProviderRequiresKey(t *testing.T) {
	// Missing key must fail at construction (init-time), not request-time.
	if _, err := NewClaudeProvider(Config{Enabled: true}); err == nil {
		t.Fatal("expected error when APIKey is empty")
	}
	if _, err := NewClaudeProvider(Config{Enabled: true, APIKey: "sk-test"}); err != nil {
		t.Fatalf("unexpected error with key present: %v", err)
	}
}
