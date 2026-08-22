package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func patchJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func deleteURL(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestModQueueUsesNativeDurableInboxAndTaskBudgetMetadata(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	ts, _ := newPersistentModServer(t, root)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/mod/queue/pause", `{}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pause=%d", resp.StatusCode)
	}

	resp = postJSON(t, ts.URL+"/mod/queue/items", `{
      "input":"offline queued task",
      "idempotencyKey":"apk-q-1",
      "taskBudget":{"tokenLimit":1234,"wallSeconds":30}
    }`)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("enqueue=%d %s", resp.StatusCode, body)
	}
	var admitted struct {
		Receipt struct {
			ItemID string `json:"itemId"`
		} `json:"receipt"`
	}
	if err := json.Unmarshal(body, &admitted); err != nil || admitted.Receipt.ItemID == "" {
		t.Fatalf("enqueue body=%s err=%v", body, err)
	}

	resp, err := http.Get(ts.URL + "/mod/queue")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"backend":"reasonix-sessioninbox"`) ||
		!strings.Contains(string(body), `"providerCallsForListing":false`) ||
		!strings.Contains(string(body), `"tokenLimit":1234`) || !strings.Contains(string(body), `"wallSeconds":30`) {
		t.Fatalf("queue=%d %s", resp.StatusCode, body)
	}

	resp = patchJSON(t, ts.URL+"/mod/queue/items/"+admitted.Receipt.ItemID, `{"input":"updated offline queued task"}`)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update=%d %s", resp.StatusCode, body)
	}

	resp = deleteURL(t, ts.URL+"/mod/queue/items/"+admitted.Receipt.ItemID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete=%d", resp.StatusCode)
	}
	resp, err = http.Get(ts.URL + "/mod/queue")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"count":0`) {
		t.Fatalf("queue after delete=%s", body)
	}
}

func TestModQueueKZTBudgetFailsClosedWithoutPricedProvider(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	ts, _ := newPersistentModServer(t, t.TempDir())
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/mod/queue/pause", `{}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pause=%d", resp.StatusCode)
	}
	resp = postJSON(t, ts.URL+"/mod/queue/items", `{"input":"budgeted","taskBudget":{"budgetKzt":100}}`)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "cannot enforce task KZT budget") {
		t.Fatalf("budget admission=%d %s", resp.StatusCode, body)
	}
}

func TestModQueueRecoveryRejectsBlindRetry(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	ts, _ := newPersistentModServer(t, t.TempDir())
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/mod/queue/pause", `{}`)
	resp.Body.Close()
	resp = postJSON(t, ts.URL+"/mod/queue/items", `{"input":"queued"}`)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("enqueue=%d %s", resp.StatusCode, body)
	}
	var admitted struct {
		Receipt struct {
			ItemID string `json:"itemId"`
		} `json:"receipt"`
	}
	_ = json.Unmarshal(body, &admitted)
	resp = postJSON(t, ts.URL+"/mod/queue/recovery/retry", `{"ids":[`+jsonQuote(admitted.Receipt.ItemID)+`],"resume":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("blind recovery retry=%d, want 409", resp.StatusCode)
	}
}
