package session

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIsSecureRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("uses tls when present", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest("GET", "https://example.com", nil)
		req.TLS = &tls.ConnectionState{}
		c.Request = req

		if !isSecureRequest(c) {
			t.Fatal("expected TLS request to be secure")
		}
	})

	t.Run("uses forwarded proto", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest("GET", "http://example.com", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		c.Request = req

		if !isSecureRequest(c) {
			t.Fatal("expected forwarded https request to be secure")
		}
	})

	t.Run("defaults to insecure", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "http://example.com", nil)

		if isSecureRequest(c) {
			t.Fatal("expected plain http request to be insecure")
		}
	})
}
