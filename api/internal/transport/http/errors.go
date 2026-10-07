package httptransport

import (
	"github.com/gin-gonic/gin"
)

// writeJSONError writes an error response in {"error": msg} JSON format.
func writeJSONError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

