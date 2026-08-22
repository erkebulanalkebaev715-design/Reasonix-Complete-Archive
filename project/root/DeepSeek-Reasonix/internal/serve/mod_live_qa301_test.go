package serve

import (
	"io"
	"strings"
	"testing"

	"reasonix/internal/event"
)

// QA3 regression: ordinary numeric answers (e.g. 17*24 = 408) must never be
// masked by the live protocol's credential redaction. Only credential-shaped
// values are masked.
func TestModLiveProtocolDoesNotMaskOrdinaryNumericAnswers(t *testing.T) {
	srv, ts := newModLiveTestServer(t)
	defer ts.Close()

	srv.observeCoreEvent(event.Event{Kind: event.Message, Text: "17*24 = 408"})
	srv.observeCoreEvent(event.Event{Kind: event.Message, Text: "The answer is 408."})

	resp, err := ts.Client().Get(ts.URL + "/mod/live/history?limit=100")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)

	for _, want := range []string{"17*24 = 408", "The answer is 408."} {
		if !strings.Contains(text, want) {
			t.Fatalf("ordinary numeric answer %q was masked or lost: %s", want, text)
		}
	}
	if strings.Contains(text, "****") {
		t.Fatalf("numeric answer was replaced by a masked credential: %s", text)
	}
}

// QA3: the live protocol must never mirror event.Reasoning as chat text; the
// honest UI shows "provider did not expose reasoning" instead.
func TestModLiveProtocolNeverMirrorsReasoningAsText(t *testing.T) {
	srv, ts := newModLiveTestServer(t)
	defer ts.Close()

	srv.observeCoreEvent(event.Event{Kind: event.Reasoning, Text: "POTENTIALLY_MASKED_CHAIN_*******"})
	srv.observeCoreEvent(event.Event{Kind: event.Message, Text: "done"})

	resp, err := ts.Client().Get(ts.URL + "/mod/live/history?limit=100")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)

	if strings.Contains(text, "POTENTIALLY_MASKED_CHAIN") {
		t.Fatalf("reasoning leaked into live history: %s", text)
	}
	if !strings.Contains(text, "live.chat.message") {
		t.Fatalf("chat message missing: %s", text)
	}
}
