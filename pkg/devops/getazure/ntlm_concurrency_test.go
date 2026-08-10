package getazure

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// connectionBoundNTLMServer behaves the way IIS does under Windows authentication: the Type2
// challenge is issued against the connection it arrived on, and a Type3 presented on any other
// connection is refused with 401.1 / 0x80090308 SEC_E_INVALID_TOKEN.
//
// That binding is the whole reason the bug exists, so a test server that ignored it would
// pass no matter how the transport pooled connections.
type connectionBoundNTLMServer struct {
	mu         sync.Mutex
	challenged map[string]bool // keyed by client ip:port, i.e. one entry per TCP connection
	rejected   int

	// body is returned on success, standing in for a packfile: the response is streamed over
	// the same connection the handshake used, so reading it proves the pool was not torn down
	// underneath it.
	body string
}

func (s *connectionBoundNTLMServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "NTLM ") {
		w.Header().Set("WWW-Authenticate", "NTLM")
		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "NTLM "))
	if err != nil || len(payload) < 9 {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch payload[8] {
	case 1: // negotiate: issue a challenge bound to this connection
		s.challenged[r.RemoteAddr] = true
		w.Header().Set("WWW-Authenticate",
			"NTLM "+base64.StdEncoding.EncodeToString(challengeMessage()))
		w.WriteHeader(http.StatusUnauthorized)
	case 3: // authenticate: only valid on the connection that was challenged
		if !s.challenged[r.RemoteAddr] {
			s.rejected++
			w.Header().Set("WWW-Authenticate", "NTLM")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "HTTP Error 401.1 - Unauthorized\nError Code: 0x80090308")

			return
		}
		delete(s.challenged, r.RemoteAddr)
		_, _ = io.WriteString(w, s.body)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// authenticate runs one full handshake and reports whether it succeeded, reading the body to
// completion the way go-git reads a packfile.
func authenticate(t *testing.T, client *http.Client, url string) (bool, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, ""
	}
	req.SetBasicAuth("DOMAIN\\jsmith", "secret")

	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, ""
	}

	return resp.StatusCode == http.StatusOK, string(body)
}

// The regression. Clones run Workers-wide in parallel, and with a shared pool the connection
// carrying a half-finished handshake can be taken by another worker between the challenge and
// the response - the Type3 then lands on a socket that was never challenged. That is what cost
// a real scan 2 of 30 repositories, both in the first batch of concurrent workers, on
// credentials that worked for the other 28.
//
// A per-request pool is what makes this deterministic rather than a matter of timing.
func TestNTLMHandshakesSurviveConcurrentRequests(t *testing.T) {
	const workers = 32

	server := &connectionBoundNTLMServer{challenged: map[string]bool{}, body: "PACK"}
	srv := httptest.NewServer(server)
	defer srv.Close()

	client := &http.Client{Transport: newNTLMTransport(http.DefaultTransport)}

	var wg sync.WaitGroup
	failures := make(chan string, workers)
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release them together, so the pool is under real contention

			ok, body := authenticate(t, client, srv.URL)
			if !ok {
				failures <- "handshake rejected"
			} else if body != "PACK" {
				failures <- "body was " + body
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)

	if n := len(failures); n > 0 {
		t.Errorf("%d of %d concurrent handshakes failed (%d rejected as SEC_E_INVALID_TOKEN); "+
			"a repository whose clone is refused this way is dropped from the line count",
			n, workers, server.rejected)
	}
}

// The pool is torn down when the response body is closed, so it must survive long enough to
// read the body - for a clone that body is the packfile. Reaping it any earlier would trade an
// intermittent auth failure for a truncated clone, which is harder to notice.
func TestNTLMTransportStreamsTheBodyBeforeReapingTheConnection(t *testing.T) {
	payload := strings.Repeat("packfile-", 100000) // large enough not to fit in one buffer

	server := &connectionBoundNTLMServer{challenged: map[string]bool{}, body: payload}
	srv := httptest.NewServer(server)
	defer srv.Close()

	client := &http.Client{Transport: newNTLMTransport(http.DefaultTransport)}

	ok, body := authenticate(t, client, srv.URL)
	if !ok {
		t.Fatal("handshake failed")
	}
	if len(body) != len(payload) {
		t.Errorf("read %d bytes of %d; the connection was closed while the body was still "+
			"streaming", len(body), len(payload))
	}
}

// Cloning the base is what carries proxy, TLS and timeout settings into each per-request pool.
// Building a bare transport instead would drop them silently - and on a corporate network,
// losing the proxy means losing the server.
func TestNTLMTransportPreservesTheBaseConfiguration(t *testing.T) {
	base := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 42,
		DisableCompression:  true,
	}

	pool := newNTLMTransport(base).base.Clone()

	if pool.Proxy == nil {
		t.Error("proxy configuration was dropped; a proxied network would become unreachable")
	}
	if pool.MaxIdleConnsPerHost != 42 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 42", pool.MaxIdleConnsPerHost)
	}
	if !pool.DisableCompression {
		t.Error("DisableCompression was dropped")
	}
}

// utils.HTTPClient's transport is a *http.Transport, so it is cloned directly. Anything else -
// an already-wrapped transport, or a stub - must still yield a working transport rather than a
// nil base that panics on the first request.
func TestNewNTLMTransportFallsBackForANonStandardBase(t *testing.T) {
	got := newNTLMTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	}))

	if got.base == nil {
		t.Fatal("base is nil; the first request would panic")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
