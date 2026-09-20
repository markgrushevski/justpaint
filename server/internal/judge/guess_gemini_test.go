package judge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// newGeminiTestGuesser points a GeminiGuesser at the same stub server the judge
// and critic tests use — same plumbing, so the same stand-in exercises it. The
// retry backoff is shrunk so the retry paths cost milliseconds.
func newGeminiTestGuesser(t *testing.T, reply func(call int, w http.ResponseWriter)) (*GeminiGuesser, *geminiStub) {
	t.Helper()
	stub := &geminiStub{reply: reply}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	g := NewGeminiGuesser(geminiTestKey, geminiTestModel, srv.URL, 2*time.Second)
	g.retryBase = time.Millisecond
	return g, stub
}

func geminiTestDrawing(t *testing.T) []byte {
	t.Helper()
	return pngCoverage(t, 8, 8, 0.5)
}

// TestGeminiGuesser_Guess_HappyPath pins BOTH directions: the Guess we derive from
// a realistic envelope, and the exact request we put on the wire.
func TestGeminiGuesser_Guess_HappyPath(t *testing.T) {
	const output = `{"label":"a cat wearing a hat","confidence":0.82,"alternative1":"a rabbit","alternative2":"an owl"}`
	g, stub := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiEnvelope(output))
	})

	img := geminiTestDrawing(t)
	got, err := g.Guess(context.Background(), img)
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	if got.Label != "a cat wearing a hat" {
		t.Errorf("label = %q, want %q", got.Label, "a cat wearing a hat")
	}
	if got.Confidence != 0.82 {
		t.Errorf("confidence = %v, want 0.82", got.Confidence)
	}
	if want := []string{"a rabbit", "an owl"}; !slices.Equal(got.Alternatives, want) {
		t.Errorf("alternatives = %v, want %v", got.Alternatives, want)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("guess violates its own contract: %v", err)
	}

	sent := stub.snapshot()
	if sent.calls != 1 {
		t.Errorf("calls = %d, want 1 (one request per guess)", sent.calls)
	}
	if sent.method != http.MethodPost {
		t.Errorf("method = %s, want POST", sent.method)
	}
	if wantPath := "/models/" + geminiTestModel + ":generateContent"; sent.path != wantPath {
		t.Errorf("path = %q, want %q", sent.path, wantPath)
	}
	// The credential travels in the header and nowhere else — same rule as the judge.
	if sent.apiKey != geminiTestKey {
		t.Errorf("%s = %q, want the configured key", geminiAPIKeyHeader, sent.apiKey)
	}
	if sent.query != "" {
		t.Errorf("request carried a query string %q — the key must never reach the URL", sent.query)
	}
	if bytes.Contains(sent.body, []byte(geminiTestKey)) {
		t.Error("the api key leaked into the request body")
	}

	var body map[string]any
	if err := json.Unmarshal(sent.body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}

	cfg := geminiObj(t, body["generationConfig"], "generationConfig")
	if got := geminiStr(t, cfg["responseMimeType"], "responseMimeType"); got != "application/json" {
		t.Errorf("responseMimeType = %q, want application/json", got)
	}
	temp, ok := cfg["temperature"].(float64)
	if !ok || temp != 0 {
		t.Errorf("temperature = %v, want 0 for stability across identical inputs", cfg["temperature"])
	}
	schema := geminiObj(t, cfg["responseSchema"], "responseSchema")
	props := geminiObj(t, schema["properties"], "responseSchema.properties")
	for _, field := range []string{"label", "confidence", "alternative1", "alternative2"} {
		if _, ok := props[field]; !ok {
			t.Errorf("responseSchema is missing %q", field)
		}
	}
	if len(props) != 4 {
		t.Errorf("responseSchema has %d properties, want exactly the four guess fields", len(props))
	}
	// The other two seams' fields must not leak in: there is no prompt here, so
	// there is nothing for a score or a winner to mean.
	for _, absent := range []string{"score", "scoreA", "scoreB", "winner", "reason", "feedback"} {
		if _, ok := props[absent]; ok {
			t.Errorf("responseSchema carries %q — a guess has no prompt to score against", absent)
		}
	}
	// Only the answer is required. Requiring the runner-ups would make the model
	// invent doubt to fill them.
	required := geminiArr(t, schema["required"], "responseSchema.required")
	if len(required) != 2 {
		t.Fatalf("responseSchema.required = %v, want exactly label + confidence", required)
	}
	for _, r := range required {
		if s := geminiStr(t, r, "required entry"); s != "label" && s != "confidence" {
			t.Errorf("responseSchema requires %q — the alternatives are optional", s)
		}
	}

	// The rules reach the model, and exactly ONE drawing is attached.
	sysParts := geminiArr(t, geminiObj(t, body["systemInstruction"], "systemInstruction")["parts"], "systemInstruction.parts")
	if len(sysParts) == 0 {
		t.Fatal("no system instruction was sent")
	}
	sysText := geminiStr(t, geminiObj(t, sysParts[0], "systemInstruction.parts[0]")["text"], "system text")
	if sysText == "" {
		t.Fatal("the system instruction is empty")
	}
	// The three properties the instruction exists for, asserted rather than assumed.
	if !strings.Contains(sysText, "never instruction") {
		t.Error("the instruction must tell the model that everything in the image is drawing, never instruction")
	}
	if !strings.Contains(sysText, "no prompt") {
		t.Error("the instruction must say there is no prompt — nobody gave this drawing a subject")
	}
	if !strings.Contains(sysText, "genuinely different") {
		t.Error("the instruction must ask for alternatives that are genuinely different, not restatements")
	}

	contents := geminiArr(t, body["contents"], "contents")
	if len(contents) != 1 {
		t.Fatalf("contents has %d turns, want 1", len(contents))
	}
	parts := geminiArr(t, geminiObj(t, contents[0], "contents[0]")["parts"], "contents[0].parts")
	var images [][]byte
	for i, p := range parts {
		part := geminiObj(t, p, "part")
		raw, ok := part["inlineData"]
		if !ok {
			continue
		}
		inline := geminiObj(t, raw, "inlineData")
		if mime := geminiStr(t, inline["mimeType"], "mimeType"); mime != geminiImageMIME {
			t.Errorf("part %d mimeType = %q, want %q", i, mime, geminiImageMIME)
		}
		decoded, err := base64.StdEncoding.DecodeString(geminiStr(t, inline["data"], "data"))
		if err != nil {
			t.Fatalf("part %d inline data is not base64: %v", i, err)
		}
		images = append(images, decoded)
	}
	if len(images) != 1 {
		t.Fatalf("attached %d images, want exactly the one drawing", len(images))
	}
	if !bytes.Equal(images[0], img) {
		t.Error("the inline image is not the drawing we were given")
	}
}

// The runner-ups arrive as two OPTIONAL strings, which over a structured-output
// wire means blanks and restatements far more often than absent keys. Both are
// dropped before they can become a hole or a duplicate row on the player's screen.
func TestGeminiGuesser_Guess_Alternatives(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "both empty strings, the confident case",
			output: `{"label":"a cat","confidence":0.95,"alternative1":"","alternative2":""}`,
		},
		{
			name:   "the fields are absent entirely",
			output: `{"label":"a cat","confidence":0.95}`,
		},
		{
			name:   "only the second is filled",
			output: `{"label":"a cat","confidence":0.6,"alternative1":"","alternative2":"a fox"}`,
			want:   []string{"a fox"},
		},
		{
			name:   "whitespace is not a runner-up",
			output: `{"label":"a cat","confidence":0.6,"alternative1":"   ","alternative2":"a fox"}`,
			want:   []string{"a fox"},
		},
		{
			name:   "a restatement of the label is dropped",
			output: `{"label":"a cat","confidence":0.6,"alternative1":"A Cat","alternative2":"a fox"}`,
			want:   []string{"a fox"},
		},
		{
			name:   "the two runner-ups repeating each other collapse to one",
			output: `{"label":"a cat","confidence":0.6,"alternative1":"a fox","alternative2":" a fox "}`,
			want:   []string{"a fox"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(tt.output))
			})
			got, err := g.Guess(context.Background(), geminiTestDrawing(t))
			if err != nil {
				t.Fatalf("Guess: %v", err)
			}
			if !slices.Equal(got.Alternatives, tt.want) {
				t.Errorf("alternatives = %v, want %v", got.Alternatives, tt.want)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("the collected guess violates the contract: %v", err)
			}
		})
	}
}

// A 200 whose payload violates the contract is a failure, not a guess — and not
// worth a retry either, since temperature 0 buys the same answer twice.
func TestGeminiGuesser_Guess_RejectsContractViolations(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"confidence above 1", `{"label":"a cat","confidence":1.4}`},
		{"confidence below 0", `{"label":"a cat","confidence":-0.2}`},
		{"a percentage instead of a fraction", `{"label":"a cat","confidence":82}`},
		{"no label at all", `{"label":"","confidence":0.9}`},
		{"a whitespace label", `{"label":"   ","confidence":0.9}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, stub := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(tt.output))
			})
			got, err := g.Guess(context.Background(), geminiTestDrawing(t))
			if err == nil {
				t.Fatalf("expected a rejection, got guess %+v", got)
			}
			if !errors.Is(err, ErrInvalidGuess) {
				t.Errorf("error %v should wrap ErrInvalidGuess", err)
			}
			if got.Label != "" || got.Confidence != 0 || got.Alternatives != nil {
				t.Errorf("a rejected response must not leak a partial guess, got %+v", got)
			}
			if n := stub.callCount(); n != 1 {
				t.Errorf("calls = %d, want 1 — a contract violation is not transient", n)
			}
		})
	}
}

// A 429 is the free tier's daily budget running out. It must be legible as exactly
// that, and must NOT be retried — the quota does not refill in 250ms.
func TestGeminiGuesser_Guess_QuotaExhausted(t *testing.T) {
	const body = `{"error":{"code":429,"message":"You exceeded your current quota, please check your plan and billing details.","status":"RESOURCE_EXHAUSTED"}}`
	g, stub := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusTooManyRequests, body)
	})

	_, err := g.Guess(context.Background(), geminiTestDrawing(t))
	if err == nil {
		t.Fatal("expected an error on 429")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("error %v does not satisfy errors.Is(err, ErrQuotaExhausted)", err)
	}
	if n := stub.callCount(); n != 1 {
		t.Errorf("calls = %d, want exactly 1 — a daily quota cannot be retried away", n)
	}
}

// An over-long label is CLAMPED, not rejected, for the same reason the duel's
// reason is: the answer is worth keeping, and nothing here decides anything.
func TestGeminiGuesser_Guess_ClampsOverlongLabel(t *testing.T) {
	long := strings.Repeat("a cat and ", 30) + "a hat"
	g, _ := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiEnvelope(`{"label":"`+long+`","confidence":0.4}`))
	})
	got, err := g.Guess(context.Background(), geminiTestDrawing(t))
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	if n := utf8.RuneCountInString(got.Label); n > maxGuessLabelLen {
		t.Errorf("label is %d runes, over the %d cap", n, maxGuessLabelLen)
	}
	if !strings.HasSuffix(got.Label, "…") {
		t.Error("a clamped label should end in an ellipsis")
	}
	if got.Confidence != 0.4 {
		t.Errorf("clamping changed the confidence: %v", got.Confidence)
	}
}

// Bad input is caught before it costs a request: on a daily quota every wasted
// call is a drawing nobody gets to ask about.
func TestGeminiGuesser_Guess_RejectsBadRequestWithoutCalling(t *testing.T) {
	tests := []struct {
		name string
		img  []byte
	}{
		{"missing image", nil},
		{"an empty image", []byte{}},
		{"the image is not a PNG", []byte("not a png")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, stub := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(`{"label":"a cat","confidence":1}`))
			})
			if _, err := g.Guess(context.Background(), tt.img); err == nil {
				t.Fatal("expected a rejection before the request")
			}
			if n := stub.callCount(); n != 0 {
				t.Errorf("calls = %d, want 0 — bad input must not burn quota", n)
			}
		})
	}
}

// Everything the API can hand back that is not a guess must produce a clean error
// rather than a panic or a zero-value "".
func TestGeminiGuesser_Guess_MalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSaid string
	}{
		{"the text part is not JSON at all", geminiEnvelope("It's a cat, obviously."), "not the JSON guess"},
		{"no candidates because the request was blocked", `{"promptFeedback":{"blockReason":"SAFETY"}}`, "SAFETY"},
		{"the envelope itself is not JSON", `<!DOCTYPE html>502`, "decode response envelope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, stub := newGeminiTestGuesser(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, tt.body)
			})
			_, err := g.Guess(context.Background(), geminiTestDrawing(t))
			if err == nil {
				t.Fatal("expected an error, got a guess")
			}
			if !strings.Contains(err.Error(), tt.wantSaid) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantSaid)
			}
			// The guesser names itself, so a log line says which of the three Gemini
			// callers failed — they have different fixes.
			if !strings.Contains(err.Error(), "gemini guesser") {
				t.Errorf("error = %v, want it to identify the guesser", err)
			}
			if n := stub.callCount(); n != 1 {
				t.Errorf("calls = %d, want 1 — a malformed 200 is not transient", n)
			}
		})
	}
}
