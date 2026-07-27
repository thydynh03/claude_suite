package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func cloudCodeStub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := CloudCodeBase
	CloudCodeBase = srv.URL
	t.Cleanup(func() {
		CloudCodeBase = prev
		srv.Close()
	})
}

// The whole point of the project id. Google answers fetchAvailableModels
// without one by reporting every model at 100%, on accounts that are being
// rate-limited at that very moment. Sending the request anyway would put a full
// green bar over a dead account — the invented-percentage bug, back again with
// a real HTTP call behind it to make it look trustworthy.
func TestQuotaIsNotFetchedWithoutAProjectID(t *testing.T) {
	called := false
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"models":{"gemini-3-pro":{"quotaInfo":{"remainingFraction":1}}}}`))
	})

	_, err := FetchAvailableModels(context.Background(), "token", "")
	if !errors.Is(err, ErrNoProject) {
		t.Fatalf("err = %v, want ErrNoProject", err)
	}
	if called {
		t.Error("the request was sent anyway — Google would have answered 100% for everything")
	}
}

// A model Google listed without a quotaInfo is unknown, not full. Go would
// zero-value a missing float to 0 and a missing bool to false; neither is the
// honest reading, so the decoder uses pointers and this pins that.
func TestAModelWithoutQuotaInfoReportsNoData(t *testing.T) {
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":{
			"a-model":{"displayName":"A Model"},
			"b-model":{"displayName":"B Model","quotaInfo":{}},
			"c-model":{"displayName":"C Model","quotaInfo":{"remainingFraction":0,"isExhausted":true,"resetTime":"2026-07-28T09:20:41Z"}}
		}}`))
	})

	models, err := FetchAvailableModels(context.Background(), "token", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}

	byName := map[string]ModelQuotaInfo{}
	for _, m := range models {
		byName[m.DisplayName] = m
	}

	for _, name := range []string{"A Model", "B Model"} {
		if byName[name].HasQuota {
			t.Errorf("%s: HasQuota = true, want false — Google said nothing about it", name)
		}
		if byName[name].RemainingFraction != -1 {
			t.Errorf("%s: RemainingFraction = %v, want -1", name, byName[name].RemainingFraction)
		}
	}

	exhausted := byName["C Model"]
	if !exhausted.HasQuota || exhausted.RemainingFraction != 0 || !exhausted.IsExhausted {
		t.Errorf("C Model = %+v, want a real, exhausted reading", exhausted)
	}
	if exhausted.ResetTime.IsZero() {
		t.Error("C Model: reset time was not parsed")
	}
}

// The models are rendered in the order returned, and Go randomises map order.
func TestModelsComeBackInAStableOrder(t *testing.T) {
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":{"z":{"displayName":"Zeta"},"a":{"displayName":"Alpha"},"m":{"displayName":"Mu"}}}`))
	})

	for i := 0; i < 8; i++ {
		models, err := FetchAvailableModels(context.Background(), "token", "proj-1")
		if err != nil {
			t.Fatal(err)
		}
		if models[0].DisplayName != "Alpha" || models[2].DisplayName != "Zeta" {
			t.Fatalf("run %d: order = %v", i, []string{models[0].DisplayName, models[1].DisplayName, models[2].DisplayName})
		}
	}
}

func TestLoadCodeAssistSendsWhatTheEndpointExpects(t *testing.T) {
	var gotAuth, gotUA, gotAPIClient string
	var gotBody map[string]any
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAPIClient = r.Header.Get("X-Goog-Api-Client")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"cloudaicompanionProject":"proj-42","currentTier":{"id":"standard-tier","name":"Standard"}}`))
	})

	project, rawTier, err := LoadCodeAssist(context.Background(), "tok-abc")
	if err != nil {
		t.Fatal(err)
	}
	if project != "proj-42" || rawTier != "standard-tier" {
		t.Errorf("got project %q tier %q", project, rawTier)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	// v1internal answers differently to clients it does not recognise, so these
	// are load-bearing rather than cosmetic.
	if gotUA != "antigravity" {
		t.Errorf("User-Agent = %q, want antigravity", gotUA)
	}
	if gotAPIClient == "" {
		t.Error("X-Goog-Api-Client was not sent")
	}
	meta, _ := gotBody["metadata"].(map[string]any)
	if meta["ideType"] != "ANTIGRAVITY" || meta["pluginType"] != "GEMINI" {
		t.Errorf("metadata = %v", meta)
	}
}

// A tier nobody recognises must not be rounded to the nearest label. Showing
// PRO because a string was unfamiliar is the same class of lie as the constant.
func TestUnknownTiersStayUnknown(t *testing.T) {
	cases := map[string]string{
		"free-tier":            "FREE",
		"legacy-tier":          "UNKNOWN",
		"standard-tier":        "PRO",
		"ai-ultra":             "ULTRA",
		"Google AI Pro":        "PRO",
		"":                     "UNKNOWN",
		"some-new-plan-2027":   "UNKNOWN",
		"enterprise-unlimited": "PRO",
	}
	for raw, want := range cases {
		if got := NormalizeTier(raw); got != want {
			t.Errorf("NormalizeTier(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Losing a tier that was successfully read because the second call failed would
// make the button look broken on accounts it partly worked for.
func TestTierSurvivesAFailedQuotaCall(t *testing.T) {
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1internal:loadCodeAssist" {
			w.Write([]byte(`{"cloudaicompanionProject":"proj-9","currentTier":{"id":"ai-ultra"}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})

	plan, err := FetchAccountPlan(context.Background(), "tok")
	if err == nil {
		t.Fatal("want an error from the quota call")
	}
	if plan.Tier != "ULTRA" {
		t.Errorf("tier = %q, want ULTRA — it was read before the quota call failed", plan.Tier)
	}
	if len(plan.Models) != 0 {
		t.Error("models were filled in from a failed call")
	}
}

// An account with no project id gets no quota, and that must reach the caller
// as ErrNoProject rather than as an empty success.
func TestAccountWithoutProjectReportsWhyQuotaIsMissing(t *testing.T) {
	cloudCodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"currentTier":{"id":"free-tier"}}`))
	})

	plan, err := FetchAccountPlan(context.Background(), "tok")
	if !errors.Is(err, ErrNoProject) {
		t.Fatalf("err = %v, want ErrNoProject", err)
	}
	if plan.Tier != "FREE" {
		t.Errorf("tier = %q, want FREE", plan.Tier)
	}
}
