package openai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSendPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
	}))
	defer srv.Close()

	c := &Client{APIKey: "test", HTTPClient: srv.Client()}
	// override URL via environment variable (not in client yet). We'll mimic by patching constant, but constant not defined.
	// So simply set custom server endpoint by new request - we can't patch constant elegantly.
	// Instead we patch by using a variable for endpoint.
	_ = os.Setenv("OPENAI_API_URL", srv.URL)
	reply, err := c.SendPrompt("hi")
	if err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}
