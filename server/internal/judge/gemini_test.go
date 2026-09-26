package judge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	geminiTestKey    = "AIza-test-key-do-not-log"
	geminiTestModel  = "gemini-test-model"
	geminiTestPrompt = "a fox riding a bicycle"
)

// geminiStub is an httptest fake of the Generative Language API. It records the
// request we sent (so a test can assert its shape) and replays a scripted reply
// per call index. No test touches the network — that is what the injectable
// baseURL is for.
type geminiStub struct {
	mu    sync.Mutex
	sent  geminiSent
	reply func(call int, w http.ResponseWriter)
}

// geminiSent is what the stub recorded of the last request.
type geminiSent struct {
	calls  int
	method string
	path   string
	query  string
	apiKey string
	ctype  string
	body   []byte
}

func (s *geminiStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.sent.calls++
	call := s.sent.calls
	s.sent.method = r.Method
	s.sent.path = r.URL.Path
	s.sent.query = r.URL.RawQuery
	s.sent.apiKey = r.Header.Get(geminiAPIKeyHeader)
	s.sent.ctype = r.Header.Get("Content-Type")
	s.sent.body = body
	s.mu.Unlock()
	s.reply(call, w)
}

func (s *geminiStub) snapshot() geminiSent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
}

func (s *geminiStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent.calls
}

// newGeminiTestJudge points a GeminiJudge at a stub server. The retry backoff is
// shrunk so the retry tests cost milliseconds, not seconds.
func newGeminiTestJudge(t *testing.T, reply func(call int, w http.ResponseWriter)) (*GeminiJudge, *geminiStub) {
	t.Helper()
	stub := &geminiStub{reply: reply}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	j := NewGeminiJudge(geminiTestKey, geminiTestModel, srv.URL, 2*time.Second)
	j.RetryBase = time.Millisecond
	return j, stub
}

func geminiTestRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		Prompt: geminiTestPrompt,
		ImageA: pngCoverage(t, 8, 8, 0.6),
		ImageB: pngCoverage(t, 8, 8, 0.2),
	}
}

func geminiWrite(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// geminiEnvelope wraps a model output in a realistic generateContent response,
// extra fields and all — §10 requires us to tolerate unknown response fields.
func geminiEnvelope(modelOutput string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[{"text":` + strconv.Quote(modelOutput) +
		`}]},"finishReason":"STOP","index":0,"safetyRatings":[]}],` +
		`"usageMetadata":{"promptTokenCount":1544,"candidatesTokenCount":61,"totalTokenCount":1605},` +
		`"modelVersion":"gemini-test-model"}`
}

// --- tiny JSON accessors, so the request assertions read as prose ------------

func geminiObj(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: want a JSON object, got %T", what, v)
	}
	return m
}

func geminiArr(t *testing.T, v any, what string) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s: want a JSON array, got %T", what, v)
	}
	return a
}

func geminiStr(t *testing.T, v any, what string) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s: want a JSON string, got %T", what, v)
	}
	return s
}

// TestGeminiJudge_Score_HappyPath pins BOTH directions: the exact Result we
// derive from a realistic envelope, and the exact request we put on the wire.
func TestGeminiJudge_Score_HappyPath(t *testing.T) {
	const output = `{"scoreA":0.82,"scoreB":0.41,"winner":"A","reason":"The first drawing shows a fox clearly astride a bicycle; the second reads as an animal beside two circles."}`
	j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiEnvelope(output))
	})

	req := geminiTestRequest(t)
	res, err := j.Score(context.Background(), req)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	want := Result{
		ScoreA: 0.82,
		ScoreB: 0.41,
		Winner: WinnerA,
		Reason: "The first drawing shows a fox clearly astride a bicycle; the second reads as an animal beside two circles.",
	}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	if err := res.Validate(); err != nil {
		t.Errorf("result violates the §2 contract: %v", err)
	}

	sent := stub.snapshot()
	if sent.calls != 1 {
		t.Errorf("calls = %d, want 1 (one request per duel)", sent.calls)
	}
	if sent.method != http.MethodPost {
		t.Errorf("method = %s, want POST", sent.method)
	}
	if wantPath := "/models/" + geminiTestModel + ":generateContent"; sent.path != wantPath {
		t.Errorf("path = %q, want %q", sent.path, wantPath)
	}
	if !strings.HasPrefix(sent.ctype, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", sent.ctype)
	}

	// The credential travels in the header and nowhere else.
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

	// Structured output: JSON mime + a schema pinning exactly the §2 fields.
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
	if len(props) != 4 {
		t.Errorf("responseSchema has %d properties, want exactly the 4 contract fields", len(props))
	}
	for _, field := range []string{"scoreA", "scoreB", "winner", "reason"} {
		if _, ok := props[field]; !ok {
			t.Errorf("responseSchema is missing %q", field)
		}
	}
	required := geminiArr(t, schema["required"], "responseSchema.required")
	if len(required) != 4 {
		t.Errorf("responseSchema.required = %v, want all 4 fields required", required)
	}
	winnerEnum := geminiArr(t, geminiObj(t, props["winner"], "winner schema")["enum"], "winner.enum")
	gotEnum := make([]string, 0, len(winnerEnum))
	for _, v := range winnerEnum {
		gotEnum = append(gotEnum, geminiStr(t, v, "winner.enum entry"))
	}
	if strings.Join(gotEnum, ",") != WinnerA+","+WinnerB+","+WinnerTie {
		t.Errorf("winner enum = %v, want [A B tie] — a tie is first-class (§3)", gotEnum)
	}

	// The rules reach the model, and both drawings are attached in order.
	sysParts := geminiArr(t, geminiObj(t, body["systemInstruction"], "systemInstruction")["parts"], "systemInstruction.parts")
	if len(sysParts) == 0 || geminiStr(t, geminiObj(t, sysParts[0], "systemInstruction.parts[0]")["text"], "system text") == "" {
		t.Error("no system instruction was sent")
	}

	contents := geminiArr(t, body["contents"], "contents")
	if len(contents) != 1 {
		t.Fatalf("contents has %d turns, want 1 (one request per duel)", len(contents))
	}
	parts := geminiArr(t, geminiObj(t, contents[0], "contents[0]")["parts"], "contents[0].parts")
	var images [][]byte
	var texts []string
	for i, p := range parts {
		part := geminiObj(t, p, "part")
		if raw, ok := part["inlineData"]; ok {
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
		if text, ok := part["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	if len(images) != 2 {
		t.Fatalf("attached %d images, want both drawings in the one call", len(images))
	}
	if !bytes.Equal(images[0], req.ImageA) {
		t.Error("the first inline image is not imageA")
	}
	if !bytes.Equal(images[1], req.ImageB) {
		t.Error("the second inline image is not imageB")
	}
	if joined := strings.Join(texts, "\n"); !strings.Contains(joined, geminiTestPrompt) {
		t.Errorf("the match prompt never reached the model; text parts were %q", joined)
	}
}

// The verdict is authoritative and never re-derived from the scores (§3): a
// judge may legitimately call a near-equal pair a tie, or hand a decisive win to
// the drawing that did not score higher.
func TestGeminiJudge_Score_WinnerIsAuthoritative(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   Result
	}{
		{
			name:   "near-equal scores may still be a tie",
			output: `{"scoreA":0.71,"scoreB":0.70,"winner":"tie","reason":"Both drawings read as a fox on a bicycle; neither is meaningfully better."}`,
			want:   Result{ScoreA: 0.71, ScoreB: 0.70, Winner: WinnerTie, Reason: "Both drawings read as a fox on a bicycle; neither is meaningfully better."},
		},
		{
			name:   "equal scores with a decisive winner are taken as given",
			output: `{"scoreA":0.5,"scoreB":0.5,"winner":"B","reason":"The second drawing gets the bicycle across."}`,
			want:   Result{ScoreA: 0.5, ScoreB: 0.5, Winner: WinnerB, Reason: "The second drawing gets the bicycle across."},
		},
		{
			name:   "both may score high",
			output: `{"scoreA":0.9,"scoreB":0.88,"winner":"A","reason":"Two strong attempts; the first drawing shows the fox pedalling."}`,
			want:   Result{ScoreA: 0.9, ScoreB: 0.88, Winner: WinnerA, Reason: "Two strong attempts; the first drawing shows the fox pedalling."},
		},
		{
			name:   "an empty reason still validates",
			output: `{"scoreA":0.4,"scoreB":0.4,"winner":"tie","reason":"  "}`,
			want:   Result{ScoreA: 0.4, ScoreB: 0.4, Winner: WinnerTie, Reason: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, _ := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(tt.output))
			})
			res, err := j.Score(context.Background(), geminiTestRequest(t))
			if err != nil {
				t.Fatalf("Score: %v", err)
			}
			if res != tt.want {
				t.Errorf("result = %+v, want %+v", res, tt.want)
			}
		})
	}
}

// A 429 is the free tier's daily budget running out. It must be legible as
// exactly that, and must NOT be retried — the quota does not refill in 250ms.
func TestGeminiJudge_Score_QuotaExhausted(t *testing.T) {
	const body = `{"error":{"code":429,"message":"You exceeded your current quota, please check your plan and billing details.","status":"RESOURCE_EXHAUSTED"}}`
	j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusTooManyRequests, body)
	})

	_, err := j.Score(context.Background(), geminiTestRequest(t))
	if err == nil {
		t.Fatal("expected an error on 429")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("error %v does not satisfy errors.Is(err, ErrQuotaExhausted)", err)
	}
	if got := stub.callCount(); got != 1 {
		t.Errorf("calls = %d, want exactly 1 — a daily quota cannot be retried away", got)
	}
}

func TestGeminiJudge_Score_RetriesTransientFailures(t *testing.T) {
	const output = `{"scoreA":0.6,"scoreB":0.3,"winner":"A","reason":"The first drawing gets the fox across."}`
	tests := []struct {
		name      string
		failUntil int // calls before the 200
		status    int
		wantCalls int
		wantOK    bool
	}{
		{"503 recovers on the first retry", 1, http.StatusServiceUnavailable, 2, true},
		{"500 recovers on the second retry", 2, http.StatusInternalServerError, 3, true},
		{"a persistent 500 exhausts the budget", geminiMaxAttempts, http.StatusInternalServerError, geminiMaxAttempts, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, stub := newGeminiTestJudge(t, func(call int, w http.ResponseWriter) {
				if call <= tt.failUntil {
					geminiWrite(w, tt.status, `{"error":{"code":500,"message":"backend error","status":"INTERNAL"}}`)
					return
				}
				geminiWrite(w, http.StatusOK, geminiEnvelope(output))
			})

			res, err := j.Score(context.Background(), geminiTestRequest(t))
			switch {
			case tt.wantOK && err != nil:
				t.Fatalf("Score: %v", err)
			case tt.wantOK && res.Winner != WinnerA:
				t.Errorf("winner = %q, want %q after recovery", res.Winner, WinnerA)
			case !tt.wantOK && err == nil:
				t.Fatal("expected an error after a persistent 5xx")
			case !tt.wantOK && errors.Is(err, ErrQuotaExhausted):
				t.Error("a 5xx must not read as an exhausted quota")
			}
			if got := stub.callCount(); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d (1 try + up to 2 retries)", got, tt.wantCalls)
			}
		})
	}
}

func TestGeminiJudge_Score_NoRetryOn4xx(t *testing.T) {
	j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusBadRequest, `{"error":{"code":400,"message":"Invalid JSON payload","status":"INVALID_ARGUMENT"}}`)
	})
	_, err := j.Score(context.Background(), geminiTestRequest(t))
	if err == nil {
		t.Fatal("expected an error on 400")
	}
	if !strings.Contains(err.Error(), "INVALID_ARGUMENT") {
		t.Errorf("error %v should surface the API's own message", err)
	}
	if got := stub.callCount(); got != 1 {
		t.Errorf("calls = %d, want 1 — a 4xx means our request is wrong (§7)", got)
	}
}

// An over-long reason is CLAMPED, not rejected. It is the one place this impl
// normalizes instead of failing, and the asymmetry is deliberate: the scores and
// the winner decide the duel and are validated strictly, while the rationale is
// display text from a model we prompted in prose. Throwing away a duel both
// players finished, over forty characters of rationale, is the worse outcome.
// (HTTPJudge rejects the same overrun, because there it is a peer service
// breaking the agreed contract.)
func TestGeminiJudge_Score_ClampsAnOverlongReason(t *testing.T) {
	long := strings.Repeat("blah ", 200) + "final word"
	j, _ := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiEnvelope(`{"scoreA":0.4,"scoreB":0.3,"winner":"A","reason":"`+long+`"}`))
	})
	res, err := j.Score(context.Background(), geminiTestRequest(t))
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if n := utf8.RuneCountInString(res.Reason); n > maxReasonLen {
		t.Errorf("reason is %d runes, over the %d cap", n, maxReasonLen)
	}
	if err := res.Validate(); err != nil {
		t.Errorf("a clamped result must still satisfy the contract: %v", err)
	}
	if !strings.HasSuffix(res.Reason, "…") {
		t.Errorf("a clamped reason should end in an ellipsis, got %q", res.Reason[max(0, len(res.Reason)-20):])
	}
	// The verdict itself is untouched by the clamp.
	if res.ScoreA != 0.4 || res.ScoreB != 0.3 || res.Winner != WinnerA {
		t.Errorf("clamping changed the verdict: %+v", res)
	}
}

// A 200 whose payload violates the §2 contract is a failure, not a verdict — and
// not worth a retry either, since temperature 0 buys the same answer twice.
func TestGeminiJudge_Score_RejectsContractViolations(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"scoreA above 1", `{"scoreA":1.4,"scoreB":0.3,"winner":"A","reason":"ok"}`},
		{"scoreB below 0", `{"scoreA":0.4,"scoreB":-0.2,"winner":"A","reason":"ok"}`},
		{"winner not in the enum", `{"scoreA":0.4,"scoreB":0.3,"winner":"first","reason":"ok"}`},
		{"winner names a player", `{"scoreA":0.4,"scoreB":0.3,"winner":"alice","reason":"ok"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(tt.output))
			})
			res, err := j.Score(context.Background(), geminiTestRequest(t))
			if err == nil {
				t.Fatalf("expected a rejection, got verdict %+v", res)
			}
			if !errors.Is(err, ErrInvalidResult) {
				t.Errorf("error %v should wrap ErrInvalidResult", err)
			}
			if res != (Result{}) {
				t.Errorf("a rejected response must not leak a partial verdict, got %+v", res)
			}
			if got := stub.callCount(); got != 1 {
				t.Errorf("calls = %d, want 1 — a contract violation is not transient", got)
			}
		})
	}
}

// Everything the API can hand back that is not a verdict must produce a clean
// error rather than a panic or a zero-value "tie".
func TestGeminiJudge_Score_MalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSaid string
	}{
		{
			name:     "the text part is not JSON at all",
			body:     geminiEnvelope("I think the first drawing is nicer, honestly."),
			wantSaid: "not the JSON verdict",
		},
		{
			name:     "the text part is JSON but not an object",
			body:     geminiEnvelope(`["A"]`),
			wantSaid: "not the JSON verdict",
		},
		{
			name:     "no candidates, no explanation",
			body:     `{"candidates":[],"usageMetadata":{"promptTokenCount":1544}}`,
			wantSaid: "no candidates",
		},
		{
			name:     "no candidates because the request was blocked",
			body:     `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			wantSaid: "SAFETY",
		},
		{
			name:     "a candidate with no parts",
			body:     `{"candidates":[{"content":{"role":"model"},"finishReason":"MAX_TOKENS"}]}`,
			wantSaid: "MAX_TOKENS",
		},
		{
			name:     "an empty text part",
			body:     geminiEnvelope(""),
			wantSaid: "no text",
		},
		{
			name:     "the envelope itself is not JSON",
			body:     `<!DOCTYPE html><html><body>502 Bad Gateway</body></html>`,
			wantSaid: "decode response envelope",
		},
		{
			name:     "an empty body",
			body:     ``,
			wantSaid: "decode response envelope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, tt.body)
			})
			_, err := j.Score(context.Background(), geminiTestRequest(t))
			if err == nil {
				t.Fatal("expected an error, got a verdict")
			}
			if !strings.Contains(err.Error(), tt.wantSaid) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantSaid)
			}
			if got := stub.callCount(); got != 1 {
				t.Errorf("calls = %d, want 1 — a malformed 200 is not transient", got)
			}
		})
	}
}

// Bad input is caught before it costs a request: on a daily quota every wasted
// call is a duel nobody gets to play.
func TestGeminiJudge_Score_RejectsBadRequestWithoutCalling(t *testing.T) {
	good := pngCoverage(t, 8, 8, 0.5)
	tests := []struct {
		name string
		req  Request
	}{
		{"empty prompt", Request{Prompt: "  ", ImageA: good, ImageB: good}},
		{"missing imageA", Request{Prompt: geminiTestPrompt, ImageB: good}},
		{"missing imageB", Request{Prompt: geminiTestPrompt, ImageA: good}},
		{"imageA is not a PNG", Request{Prompt: geminiTestPrompt, ImageA: []byte("not a png"), ImageB: good}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiEnvelope(`{"scoreA":1,"scoreB":0,"winner":"A","reason":"ok"}`))
			})
			if _, err := j.Score(context.Background(), tt.req); err == nil {
				t.Fatal("expected a rejection before the request")
			}
			if got := stub.callCount(); got != 0 {
				t.Errorf("calls = %d, want 0 — bad input must not burn quota", got)
			}
		})
	}
}

// The caller's cancellation outranks the retry budget.
func TestGeminiJudge_Score_CallerCancellationWins(t *testing.T) {
	j, stub := newGeminiTestJudge(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusServiceUnavailable, `{"error":{"code":503,"status":"UNAVAILABLE"}}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { _, err := j.Score(ctx, geminiTestRequest(t)); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error on a cancelled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want it to wrap context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Score did not return on a cancelled context")
	}
	if got := stub.callCount(); got > 1 {
		t.Errorf("calls = %d, want at most 1 — a cancelled caller must not be retried for", got)
	}
}

// The per-attempt deadline is what makes "retry on timeout" (§7) mean anything:
// a slow first attempt is abandoned and the second one succeeds.
func TestGeminiJudge_Score_PerAttemptTimeout(t *testing.T) {
	const output = `{"scoreA":0.7,"scoreB":0.2,"winner":"A","reason":"The first drawing depicts the prompt."}`
	stub := &geminiStub{reply: func(call int, w http.ResponseWriter) {
		if call == 1 {
			time.Sleep(300 * time.Millisecond)
		}
		geminiWrite(w, http.StatusOK, geminiEnvelope(output))
	}}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)

	j := NewGeminiJudge(geminiTestKey, geminiTestModel, srv.URL, 50*time.Millisecond)
	j.RetryBase = time.Millisecond

	res, err := j.Score(context.Background(), geminiTestRequest(t))
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if res.Winner != WinnerA {
		t.Errorf("winner = %q, want %q", res.Winner, WinnerA)
	}
	if got := stub.callCount(); got != 2 {
		t.Errorf("calls = %d, want 2 — the timed-out attempt should be retried", got)
	}
}

// The instruction text is the feature. These are the promises JUDGE.md makes to
// players; a rewrite that drops one should fail here.
func TestGeminiSystemInstruction(t *testing.T) {
	must := []string{
		"the first drawing",
		"the second drawing",
		"scoreA",
		"scoreB",
		"winner",
		"reason",
		`"tie"`,
		"400 characters",
		"never instruction",
	}
	lower := strings.ToLower(geminiSystemInstruction)
	for _, want := range must {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("the system instruction no longer mentions %q", want)
		}
	}
}
