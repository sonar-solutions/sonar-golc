package getazure

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// htmlDecodeError reproduces what the Azure SDK produces when a server answers an API call
// with its HTML sign-in page: the JSON decoder reads the '<' of "<html>" and stops.
func htmlDecodeError() error {
	var v interface{}

	err := json.Unmarshal([]byte("<html><body>Sign In</body></html>"), &v)
	if err == nil {
		panic("expected decoding HTML as JSON to fail")
	}

	return err
}

// The message a user sees has to name the cause. "invalid character '<'" sends them looking
// for a corrupt response; the problem is that their credentials were refused.
func TestDescribeAzureErrorExplainsARejectedLogin(t *testing.T) {
	got := describeAzureError(htmlDecodeError(), "https://tfs.corp.example/DefaultCollection", "DOMAIN\\jsmith")

	message := got.Error()
	for _, want := range []string{"not authenticated", "https://tfs.corp.example/DefaultCollection", "collection"} {
		if !strings.Contains(message, want) {
			t.Errorf("message does not mention %q; got:\n%s", want, message)
		}
	}
	if !strings.Contains(message, `Windows authentication as "DOMAIN\\jsmith"`) {
		t.Errorf("message does not name the authentication actually attempted; got:\n%s", message)
	}
}

// Without a username the attempt was a PAT, and saying "Windows authentication" would send
// someone to configure a domain account they do not need.
func TestDescribeAzureErrorNamesPatWhenNoUsernameIsSet(t *testing.T) {
	message := describeAzureError(htmlDecodeError(), "https://dev.azure.com/my-org", "").Error()

	if !strings.Contains(message, "a personal access token") {
		t.Errorf("message does not name the PAT; got:\n%s", message)
	}
	if strings.Contains(message, "Windows authentication") {
		t.Errorf("message blames Windows authentication when none was attempted; got:\n%s", message)
	}
}

// The original has to stay reachable: it is the only evidence of what actually came back, and
// errors.Is/As on the wrapped value is how anything downstream inspects it.
func TestDescribeAzureErrorKeepsTheUnderlyingError(t *testing.T) {
	original := htmlDecodeError()

	got := describeAzureError(original, "https://tfs.corp.example", "")

	if !errors.Is(got, original) {
		t.Error("the underlying error was not wrapped, so the real response is unrecoverable")
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(got, &syntaxErr) {
		t.Error("errors.As no longer reaches the decoder error through the wrapper")
	}
}

// Errors that already read clearly must pass through untouched. Rewriting them would bury a
// precise message under a guess.
func TestDescribeAzureErrorLeavesOtherErrorsAlone(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"network", errors.New("dial tcp 10.0.0.1:443: connect: connection refused")},
		{"explicit 401", errors.New("azure devops returned 401 Unauthorized")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeAzureError(tc.err, "https://tfs.corp.example", ""); got != tc.err {
				t.Errorf("error was rewritten: %v", got)
			}
		})
	}
}

// A JSON error that is not about a '<' means the response was JSON-ish but broken - truncated,
// or an unexpected shape. Calling that an authentication failure swaps one misleading message
// for another, so the type check alone is not enough to act on.
func TestDescribeAzureErrorDoesNotBlameAuthForOtherJSONErrors(t *testing.T) {
	var v interface{}
	truncated := json.Unmarshal([]byte(`{"value": [`), &v)
	if truncated == nil {
		t.Fatal("expected truncated JSON to fail decoding")
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(truncated, &syntaxErr) {
		t.Skip("this Go version does not report truncated JSON as a *json.SyntaxError")
	}

	if got := describeAzureError(truncated, "https://tfs.corp.example", ""); got != truncated {
		t.Errorf("a truncated JSON response was reported as an authentication failure:\n%v", got)
	}
}

// The SDK flattens the decoder error into a plain fmt.Errorf on some paths, losing the type.
// The text is all that survives there, so it has to be enough.
func TestDescribeAzureErrorRecognisesTheFlattenedForm(t *testing.T) {
	if !isHTMLResponse(errors.New(
		"unable to parse response: invalid character '<' looking for beginning of value")) {
		t.Error("the flattened form was not recognised, so the message stays unexplained")
	}
}
