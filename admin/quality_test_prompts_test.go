package admin

import (
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
)

func qualityTestPromptCall(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
