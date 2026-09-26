package document_test

import (
	"os"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestEditorDocument validates a document drawn by the editor's own tools. The
// fixture is written by packages/editor/test/server-fixture.test.ts.
func TestEditorDocument(t *testing.T) {
	raw, err := os.ReadFile("testdata/editor-document.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.ParseAndValidate(raw); err != nil {
		t.Fatalf("the server rejects the editor's document: %v", err)
	}
}
