package getazure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newDisabledReposParams builds the minimal params fetchDisabledRepoIDs needs,
// pointing ApiURL at the given test server.
func newDisabledReposParams(apiURL string) ParamsProjectAzure {
	return ParamsProjectAzure{
		Context:     context.Background(),
		ApiURL:      apiURL,
		AccessToken: "token",
	}
}

func TestFetchDisabledRepoIDs_ParsesDisabledFlag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[
			{"id":"AAAAAAAA-1111-2222-3333-444444444444","isDisabled":true},
			{"id":"BBBBBBBB-1111-2222-3333-444444444444","isDisabled":false},
			{"id":"CCCCCCCC-1111-2222-3333-444444444444"}
		]}`))
	}))
	defer ts.Close()

	disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ")

	if len(disabled) != 1 {
		t.Fatalf("expected exactly 1 disabled repo, got %d: %v", len(disabled), disabled)
	}
	// IDs are keyed lowercase so lookups from git.GitRepository UUIDs match.
	if !disabled["aaaaaaaa-1111-2222-3333-444444444444"] {
		t.Errorf("disabled repo id not found (case-insensitive): %v", disabled)
	}
}

func TestFetchDisabledRepoIDs_Non200IsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	if disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ"); len(disabled) != 0 {
		t.Errorf("expected empty set on non-200 response, got %v", disabled)
	}
}

func TestFetchDisabledRepoIDs_BadJSONIsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	if disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ"); len(disabled) != 0 {
		t.Errorf("expected empty set on malformed JSON, got %v", disabled)
	}
}
