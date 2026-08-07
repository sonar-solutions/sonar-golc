package getazure

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ntlmssp "github.com/Azure/go-ntlmssp"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
)

// Without a username the connection must be indistinguishable from the SDK's own
// NewPatConnection: a PAT setup cannot be allowed to change behaviour.
func TestAzureConnectionWithoutUsernameMatchesPatConnection(t *testing.T) {
	const url, token = "https://dev.azure.com/my-org", "TOKEN"

	got := azureConnection(url, "", token)
	want := azuredevops.NewPatConnection(url, token)

	if got.AuthorizationString != want.AuthorizationString {
		t.Errorf("authorization = %q, want %q", got.AuthorizationString, want.AuthorizationString)
	}
	if got.BaseUrl != want.BaseUrl {
		t.Errorf("base url = %q, want %q", got.BaseUrl, want.BaseUrl)
	}
}

// With a username the credentials must carry it. The SDK's NewPatConnection sends an empty
// username, which a Windows-authenticated server rejects and which leaves the NTLM
// negotiator with no account to answer the challenge with.
func TestAzureConnectionWithUsernameSendsIt(t *testing.T) {
	conn := azureConnection("https://tfs.corp.example/DefaultCollection", "DOMAIN\\jsmith", "secret")

	encoded := strings.TrimPrefix(conn.AuthorizationString, "Basic ")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("authorization header is not base64: %v", err)
	}
	if string(raw) != "DOMAIN\\jsmith:secret" {
		t.Errorf("credentials = %q, want %q", raw, "DOMAIN\\jsmith:secret")
	}
	if conn.BaseUrl != "https://tfs.corp.example/defaultcollection" {
		t.Errorf("base url = %q, want it normalised the way the SDK does", conn.BaseUrl)
	}
}

// The negotiator only earns its place if it actually completes a handshake, so this drives
// one against a server that behaves like IIS: refuse the Basic header, advertise NTLM, then
// accept once the two NTLM messages arrive.
//
// It proves the wiring - that a Basic header is upgraded on challenge - not interoperability
// with a real Windows domain controller, which cannot be exercised here.
func TestNTLMNegotiatorCompletesHandshake(t *testing.T) {
	var sawNegotiate, sawAuthenticate bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "NTLM ") {
			// Basic, or nothing: refuse and advertise NTLM, as IIS does.
			w.Header().Set("WWW-Authenticate", "NTLM")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "NTLM "))
		if err != nil || len(payload) < 9 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Byte 8 of an NTLMSSP message is its type: 1 negotiate, 3 authenticate.
		switch payload[8] {
		case 1:
			sawNegotiate = true
			w.Header().Set("WWW-Authenticate",
				"NTLM "+base64.StdEncoding.EncodeToString(challengeMessage()))
			w.WriteHeader(http.StatusUnauthorized)
		case 3:
			sawAuthenticate = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	EnableNTLM()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("DOMAIN\\jsmith", "secret")

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	defer resp.Body.Close()

	if !sawNegotiate {
		t.Error("no NTLM negotiate message was sent; the Basic header was not upgraded")
	}
	if !sawAuthenticate {
		t.Error("the challenge was never answered")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 once the handshake completes", resp.StatusCode)
	}
}

// challengeMessage builds the smallest NTLMSSP type 2 message the negotiator will accept:
// the signature, the message type, a server challenge and the negotiate flags it reads.
func challengeMessage() []byte {
	msg := make([]byte, 48)
	copy(msg, "NTLMSSP\x00")
	msg[8] = 2 // type 2, challenge
	// NTLMSSP_NEGOTIATE_UNICODE | NTLMSSP_NEGOTIATE_NTLM
	msg[20] = 0x01
	msg[21] = 0x02
	copy(msg[24:32], []byte{1, 2, 3, 4, 5, 6, 7, 8}) // server challenge

	return msg
}

// utils.HTTPClient has its own Transport for connection pooling, so replacing
// http.DefaultTransport does not reach it. fetchDisabledRepoIDs uses that client, and if it
// is left unwrapped its request is the one call in the Azure path that cannot authenticate
// against a Windows-authenticated server. The failure is swallowed as "no repos disabled",
// so every disabled repository gets counted and the reported total silently inflates.
func TestEnableNTLMAlsoWrapsTheSharedClient(t *testing.T) {
	EnableNTLM()

	if _, ok := utils.HTTPClient.Transport.(ntlmssp.Negotiator); !ok {
		t.Errorf("utils.HTTPClient.Transport is %T, want it wrapped in ntlmssp.Negotiator; "+
			"disabled-repo detection would silently fail and inflate the line count",
			utils.HTTPClient.Transport)
	}
	if _, ok := http.DefaultTransport.(ntlmssp.Negotiator); !ok {
		t.Errorf("http.DefaultTransport is %T, want it wrapped", http.DefaultTransport)
	}
}

// Repeated calls must not stack negotiators on top of each other.
func TestEnableNTLMIsIdempotent(t *testing.T) {
	EnableNTLM()
	first := utils.HTTPClient.Transport
	EnableNTLM()

	if utils.HTTPClient.Transport != first {
		t.Error("a second EnableNTLM wrapped the transport again")
	}
}
