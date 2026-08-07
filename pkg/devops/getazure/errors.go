package getazure

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// When authentication fails, Azure DevOps Server does not answer with JSON. IIS returns its
// HTML sign-in page, and the Azure SDK - which assumes every response is JSON - feeds that
// page to a decoder. What reaches the user is:
//
//	invalid character '<' looking for beginning of value
//
// which describes the first byte of "<html>" and says nothing about credentials. Someone
// reading it looks for a corrupt response or a bug in the parser; the actual problem is that
// the server rejected the login. The SDK cannot be fixed from here, but the error passes
// through this package, and this is the point at which what was being attempted is still
// known - so it is the point at which it can be said.
//
// The same signature also appears when the URL is not an Azure DevOps API endpoint at all
// (a wrong collection, a reverse proxy serving its own error page), so the message names
// both causes rather than asserting the more common one.

// describeAzureError rewrites the HTML-decoded-as-JSON error into one that names the likely
// cause. Any other error is returned unchanged - guessing about errors that already read
// clearly would only add noise.
func describeAzureError(err error, apiURL, username string) error {
	if err == nil || !isHTMLResponse(err) {
		return err
	}

	authentication := "a personal access token"
	if username != "" {
		authentication = fmt.Sprintf("Windows authentication as %q", username)
	}

	return fmt.Errorf("%s returned an HTML page where the API should return JSON, which "+
		"almost always means the request was not authenticated: the credentials were "+
		"rejected and the sign-in page came back instead. Check that %s is accepted by this "+
		"server, and that the URL points at the collection (the segment before the project) "+
		"rather than at the server root. Underlying error: %w",
		apiURL, authentication, err)
}

// isHTMLResponse reports whether err is a JSON decoder complaining about a '<' - the opening
// byte of an HTML document.
//
// The '<' has to be part of the test. A *json.SyntaxError on its own only says the response
// was not valid JSON, which a truncated or corrupt API reply is too; claiming those were
// authentication failures would replace one misleading message with another. So the type is
// checked to establish it came from the JSON decoder, and the character to establish what it
// choked on. The bare string check catches the same failure once the SDK has flattened it
// into a plain fmt.Errorf, which it does on some paths.
func isHTMLResponse(err error) bool {
	const htmlOpening = "invalid character '<'"

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return strings.Contains(syntaxErr.Error(), htmlOpening)
	}

	return strings.Contains(err.Error(), htmlOpening)
}
