package handlers

import (
	"net/http"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// SearchConfig reports whether optional search features are available.
func SearchConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"query_expansion_enabled": services.QueryExpansionEnabled(),
	})
}
