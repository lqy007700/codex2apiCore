package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

func qualityTestHTTPRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/accounts/42/quality-test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
