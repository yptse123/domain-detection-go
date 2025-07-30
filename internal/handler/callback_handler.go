package handler

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// CallbackHandler handles callback requests
type CallbackHandler struct{}

// NewCallbackHandler creates a new callback handler
func NewCallbackHandler() *CallbackHandler {
	return &CallbackHandler{}
}

// HandleCallback logs the incoming request and returns success
func (h *CallbackHandler) HandleCallback(c *gin.Context) {
	// Check for secret header
	secretHeader := c.GetHeader("X-Callback-Secret")
	expectedSecret := os.Getenv("CALLBACK_SECRET")

	// If no secret is configured, skip authentication
	if expectedSecret == "" {
		log.Printf("[CALLBACK] WARNING: No CALLBACK_SECRET configured, skipping authentication")
	} else if secretHeader != expectedSecret {
		log.Printf("[CALLBACK] UNAUTHORIZED: Invalid or missing secret header from IP: %s", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Unauthorized",
		})
		return
	}

	// Generate simple request ID
	requestID := time.Now().Format("20060102-150405-000")

	// Log basic request info
	log.Printf("[CALLBACK-%s] Method: %s, URL: %s", requestID, c.Request.Method, c.Request.URL.String())
	log.Printf("[CALLBACK-%s] Remote IP: %s", requestID, c.ClientIP())
	log.Printf("[CALLBACK-%s] Headers: %v", requestID, c.Request.Header)

	// Read and log the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("[CALLBACK-%s] ERROR reading body: %v", requestID, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Failed to read request body",
		})
		return
	}

	// Log the body
	log.Printf("[CALLBACK-%s] Body: %s", requestID, string(body))

	// Return simple success response
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Callback received",
		"timestamp": time.Now(),
	})
}
