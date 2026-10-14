package admin

import (
	"github.com/gin-gonic/gin"

	"net/http"
	"net/http/httptest"
	"strings"
)

func pricingUpdateTestContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/model-pricing", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}
