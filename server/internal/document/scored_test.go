package document

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateScored(t *testing.T) {
	doc := func(w, h int) []byte {
		return fmt.Appendf(nil,
			`{"version":1,"width":%d,"height":%d,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`,
			w, h)
	}
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"the scored square", doc(ScoredCanvasSize, ScoredCanvasSize), false},
		{"a valid free-draw canvas", doc(1280, 720), true},
		{"square but not the scored size", doc(1024, 1024), true},
		{"not a document", []byte(`{"version":1}`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateScored(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			var ve *ValidationError
			if err != nil && !errors.As(err, &ve) {
				t.Fatalf("err = %T, want *ValidationError", err)
			}
		})
	}
}
