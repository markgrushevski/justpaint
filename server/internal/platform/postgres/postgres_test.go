package postgres

import (
	"errors"
	"strings"
	"testing"
)

// TestConnectHint pins when the IPv6 hint fires. The case it exists for cost a
// real deploy: Supabase's direct host is IPv6-only without the paid add-on,
// Render egresses IPv4, and pgx reports only `connect: network is unreachable`
// against a bare IPv6 literal — accurate and unusable.
func TestConnectHint(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint bool
	}{
		{
			name:     "ipv6 dial failure",
			err:      errors.New("failed to connect to `user=postgres database=postgres`:\n\t[2a05:d018:cb7:ae02::1]:5432 (db.example.supabase.co): dial error: dial tcp [2a05:d018:cb7:ae02::1]:5432: connect: network is unreachable"),
			wantHint: true,
		},
		{
			name:     "no route to host, ipv6",
			err:      errors.New("dial tcp [2a05::1]:5432: connect: no route to host"),
			wantHint: true,
		},
		{
			name: "ipv4 refused is a different problem",
			err:  errors.New("dial tcp 10.0.0.5:5432: connect: connection refused"),
		},
		{
			name: "bad password says so itself",
			err:  errors.New("failed to connect: FATAL: password authentication failed (SQLSTATE 28P01)"),
		},
		{
			name: "unreachable without an address literal",
			err:  errors.New("connect: network is unreachable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connectHint(tt.err)
			if tt.wantHint && got == "" {
				t.Fatal("expected a hint, got none")
			}
			if !tt.wantHint && got != "" {
				t.Fatalf("expected no hint, got %q", got)
			}
			if tt.wantHint && !strings.Contains(got, "pooler") {
				t.Errorf("hint does not name the pooler: %q", got)
			}
		})
	}
}
