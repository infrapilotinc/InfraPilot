package license

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Middleware adds license to request context
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		lic := Current()
		ctx := WithContext(c.Request.Context(), lic)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-InfraPilot-Edition", string(lic.Edition))
		c.Next()
	}
}

// LicenseInfoHandler returns edition information
func LicenseInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		lic := FromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{
			"edition":  lic.Edition,
			"features": lic.Features,
		})
	}
}
