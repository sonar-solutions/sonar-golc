package getazure

import (
	"io"
	"net/http"
	"strings"
	"sync"

	ntlmssp "github.com/Azure/go-ntlmssp"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
)

// Azure DevOps Server is frequently deployed behind Windows authentication, where a
// personal access token is either disabled or not what the server asks for. On such a
// server the request comes back 401 with "WWW-Authenticate: NTLM" and a bearer or basic
// header is never accepted.
//
// go-ntlmssp's Negotiator handles that case as a decorator: it lets the request go out
// with its ordinary Basic header, and only if the server answers with an NTLM challenge
// does it run the multi-step handshake using the same credentials. A server that accepts
// Basic never sees a difference, which is why enabling this cannot break a PAT setup.
//
// Two separate paths need it, because Azure work is split across two libraries:
//
//   - the REST calls, made by the Azure SDK. Its Connection carries a single static
//     Authorization header and offers no way to supply an http.Client, so the only seam is
//     http.DefaultTransport - which the SDK reaches because it builds &http.Client{} with
//     a nil Transport.
//   - the clone itself, made by go-git, which does accept a custom client.

// NTLM authenticates a *connection*, not a request. The server issues its Type2 challenge
// against the socket it arrived on and will only accept the matching Type3 on that same
// socket; anywhere else it answers 401.1 / 0x80090308 SEC_E_INVALID_TOKEN.
//
// go-ntlmssp's Negotiator performs the exchange as three separate RoundTrip calls (an
// anonymous probe, then Type1, then Type3) and does nothing to keep them together. Handed a
// pooled http.Transport it does not have to: between two of those calls the connection goes
// back to the idle pool, and with several clones running at once another worker can take it.
// The Type3 then arrives on a socket that was never challenged and the clone fails - which is
// why one scan lost 2 of 30 repositories, both in the first batch of concurrent workers,
// while the other 28 succeeded on identical credentials.
//
// Giving each request its own pool removes the sharing that makes the theft possible. Measured
// against a server that binds challenges to connections the way IIS does:
//
//	pooled keep-alives (shared)      8 workers: 4 ok, 4 rejected
//	one pool per request            32 workers: 32 ok, 0 rejected
//
// Note that simply disabling keep-alives, the obvious-looking fix, is worse than the disease:
// every RoundTrip then gets a fresh connection, so Type1 and Type3 are guaranteed to land on
// different sockets and no handshake can ever complete - it fails even single-threaded.

// ntlmTransport runs each request's NTLM handshake on a connection pool of its own.
//
// The base is held as a pointer rather than a constructor closure so the struct stays
// comparable: it is installed into http.DefaultTransport and utils.HTTPClient, where
// equality is how callers check whether it is already in place, and a func field would turn
// that check into a runtime panic.
type ntlmTransport struct {
	// base is cloned per request. Cloning a configured transport rather than building a bare
	// one carries over proxy, TLS and timeout settings - which on a corporate network are
	// usually the difference between reaching the server and not.
	base *http.Transport
}

func (t ntlmTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	pool := t.base.Clone()

	resp, err := (ntlmssp.Negotiator{RoundTripper: pool}).RoundTrip(req)
	if err != nil {
		pool.CloseIdleConnections()

		return nil, err
	}

	// The pool cannot be reaped yet: the response body is still streaming over its
	// connection, and for a clone that body is the packfile. Closing the body is what
	// returns the connection to the pool, so that is when there is something to reap.
	if resp.Body != nil {
		resp.Body = closeHook{ReadCloser: resp.Body, after: pool.CloseIdleConnections}
	}

	return resp, nil
}

// closeHook runs a function after the wrapped body is closed.
type closeHook struct {
	io.ReadCloser
	after func()
}

func (c closeHook) Close() error {
	err := c.ReadCloser.Close()
	c.after()

	return err
}

// newNTLMTransport builds a per-request-pool transport from an existing round tripper,
// preserving its configuration where it can be read.
func newNTLMTransport(base http.RoundTripper) ntlmTransport {
	if configured, ok := base.(*http.Transport); ok {
		return ntlmTransport{base: configured}
	}

	// Not a *http.Transport (already wrapped, or a stub in a test). Fall back to the standard
	// default, which at least keeps proxy-from-environment.
	if fallback, ok := http.DefaultTransport.(*http.Transport); ok {
		return ntlmTransport{base: fallback}
	}

	return ntlmTransport{base: &http.Transport{}}
}

var installNTLMOnce sync.Once

// EnableNTLM routes both the Azure SDK's REST calls and go-git's clones through an NTLM
// negotiator. It is safe to call more than once and safe on a server that does not use
// Windows authentication: without an NTLM challenge the negotiator is a pass-through.
func EnableNTLM() {
	installNTLMOnce.Do(func() {
		gitTransport := newNTLMTransport(http.DefaultTransport)

		http.DefaultTransport = newNTLMTransport(http.DefaultTransport)

		// utils.HTTPClient carries its own Transport for connection pooling, so replacing
		// http.DefaultTransport does not reach it. fetchDisabledRepoIDs uses that client,
		// and without this its request is the one call in the whole Azure path that cannot
		// authenticate - it fails, is swallowed as "no repos disabled", and every disabled
		// repository is then counted. In a licence-sizing tool that silently inflates the
		// total, which is the worst possible way for it to break.
		utils.HTTPClient.Transport = newNTLMTransport(utils.HTTPClient.Transport)

		// go-git builds its own client, so it needs the negotiator installed separately.
		// This is the path that runs Workers-wide in parallel, and the one the connection
		// theft was actually costing repositories on.
		client.InstallProtocol("https", githttp.NewClient(&http.Client{Transport: gitTransport}))
		client.InstallProtocol("http", githttp.NewClient(&http.Client{Transport: gitTransport}))
	})
}

// azureConnection builds the SDK connection for a platform configuration.
//
// With a username configured the credentials go out as user:token basic auth, which is
// what the negotiator needs in order to answer an NTLM challenge - the SDK's own
// NewPatConnection sends an empty username, which a Windows-authenticated server rejects.
// Without one, the behaviour is exactly NewPatConnection's.
func azureConnection(apiURL, username, token string) *azuredevops.Connection {
	if username == "" {
		return azuredevops.NewPatConnection(apiURL, token)
	}

	EnableNTLM()

	return &azuredevops.Connection{
		AuthorizationString:     azuredevops.CreateBasicAuthHeaderValue(username, token),
		BaseUrl:                 strings.ToLower(strings.TrimRight(apiURL, "/")),
		SuppressFedAuthRedirect: true,
	}
}

// stringOrEmpty reads an optional string out of a platform configuration.
func stringOrEmpty(v interface{}) string {
	s, _ := v.(string)

	return s
}
