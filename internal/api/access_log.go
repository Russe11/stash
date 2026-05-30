package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/stashapp/stash/pkg/session"
)

// apiKeyRedaction is the placeholder substituted for a real apikey value in
// logged request URIs.
const apiKeyRedaction = "redacted"

// redactAPIKeyInURI returns reqURI with the value of any "apikey" query
// parameter replaced by a placeholder. It operates purely on the raw
// request-line string (as found in http.Request.RequestURI), so it can
// sanitise what the access log records without disturbing the parsed r.URL
// that authentication and media-segment URL building rely on. A URI with no
// query, no apikey, or a malformed query is returned unchanged.
func redactAPIKeyInURI(reqURI string) string {
	qIdx := strings.IndexByte(reqURI, '?')
	if qIdx < 0 {
		return reqURI
	}

	path, rawQuery := reqURI[:qIdx], reqURI[qIdx+1:]
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Don't risk dropping a malformed query from the log; leave it as-is.
		return reqURI
	}
	if _, ok := values[session.ApiKeyParameter]; !ok {
		return reqURI
	}

	values.Set(session.ApiKeyParameter, apiKeyRedaction)
	return path + "?" + values.Encode()
}

// redactAPIKeyAccessLog rewrites r.RequestURI so the downstream access logger
// (go-chi/httplog records r.RequestURI verbatim) never persists the apikey
// credential to stash's access log, proxy logs, or shell history. r.URL is
// left untouched.
func redactAPIKeyAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "" {
			r.RequestURI = redactAPIKeyInURI(r.RequestURI)
		}
		next.ServeHTTP(w, r)
	})
}
