package getazure

import (
	"net/http"
	"strings"
	"sync"

	ntlmssp "github.com/Azure/go-ntlmssp"
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

var installNTLMOnce sync.Once

// EnableNTLM routes both the Azure SDK's REST calls and go-git's clones through an NTLM
// negotiator. It is safe to call more than once and safe on a server that does not use
// Windows authentication: without an NTLM challenge the negotiator is a pass-through.
func EnableNTLM() {
	installNTLMOnce.Do(func() {
		http.DefaultTransport = ntlmssp.Negotiator{RoundTripper: http.DefaultTransport}

		// go-git builds its own client, so it needs the negotiator installed separately.
		client.InstallProtocol("https", githttp.NewClient(
			&http.Client{Transport: ntlmssp.Negotiator{RoundTripper: &http.Transport{}}}))
		client.InstallProtocol("http", githttp.NewClient(
			&http.Client{Transport: ntlmssp.Negotiator{RoundTripper: &http.Transport{}}}))
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
