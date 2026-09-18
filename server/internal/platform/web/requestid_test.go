package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestResolveRequestID(t *testing.T) {
	cases := []struct {
		name       string
		trustProxy bool
		inbound    string // "" means the header is not set at all
		wantAdopts bool   // want the returned id to equal the inbound value verbatim
	}{
		{"untrusted: inbound header ignored, a fresh id is generated", false, "attacker-chosen-id", false},
		{"trusted: a sane inbound id is adopted", true, "abc-123.trace_id", true},
		{"trusted: an absent header still generates one", true, "", false},
		{"trusted: an inbound id with illegal characters is rejected", true, "not valid! id", false},
		{"trusted: an oversized inbound id is rejected", true, strings.Repeat("a", maxInboundRequestID+1), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Request{Header: make(http.Header)}
			if c.inbound != "" {
				r.Header.Set(RequestIDHeader, c.inbound)
			}
			got := resolveRequestID(r, c.trustProxy)
			if got == "" {
				t.Fatal("resolveRequestID returned an empty id")
			}
			adopted := got == c.inbound && c.inbound != ""
			if adopted != c.wantAdopts {
				t.Errorf("resolveRequestID() = %q (inbound %q) — adopted = %v, want %v",
					got, c.inbound, adopted, c.wantAdopts)
			}
		})
	}
}

func TestRequestID_ContextRoundtrip(t *testing.T) {
	if _, ok := RequestID(context.Background()); ok {
		t.Error("RequestID on a bare context should report ok=false")
	}

	ctx := context.WithValue(context.Background(), requestIDCtxKey{}, "the-id")
	got, ok := RequestID(ctx)
	if !ok || got != "the-id" {
		t.Errorf("RequestID() = (%q, %v), want (%q, true)", got, ok, "the-id")
	}
}
