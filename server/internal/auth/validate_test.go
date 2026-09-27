package auth

import (
	"strings"
	"testing"
)

func TestValidLogin(t *testing.T) {
	tests := []struct {
		name  string
		login string
		want  bool
	}{
		{"a nickname", "painter_42", true},
		{"an email", "ann@example.com", true},
		{"non-Latin letters", "художник", true},
		{"three characters", "abc", true},
		{"254 characters", strings.Repeat("a", 254), true},
		{"two characters", "ab", false},
		{"255 characters", strings.Repeat("a", 255), false},
		{"a space inside", "ann smith", false},
		{"a tab", "ann\tsmith", false},
		{"a no-break space", "ann smith", false},
		{"a zero-width space", "ann​smith", false},
		{"a right-to-left override", "ann‮smith", false},
		{"a control character", "ann\x07", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validLogin(tt.login); got != tt.want {
				t.Errorf("validLogin(%q) = %v, want %v", tt.login, got, tt.want)
			}
		})
	}
}

func TestNormalizeDisplayName(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name   string
		in     *string
		want   *string
		wantOK bool
	}{
		{"absent", nil, nil, true},
		{"blank", str("   "), nil, true},
		{"trimmed", str("  Ann Smith "), str("Ann Smith"), true},
		{"64 characters", str(strings.Repeat("я", 64)), str(strings.Repeat("я", 64)), true},
		{"65 characters", str(strings.Repeat("я", 65)), nil, false},
		{"a right-to-left override", str("Ann‮Smith"), nil, false},
		{"a zero-width joiner", str("Ann‍Smith"), nil, false},
		{"a newline", str("Ann\nSmith"), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeDisplayName(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v, want %v", deref(got), deref(tt.want))
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
