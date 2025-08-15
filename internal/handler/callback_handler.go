package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"domain-detection-go/internal/deepcheck"
	"domain-detection-go/internal/domain"
	"domain-detection-go/internal/notification"
	"domain-detection-go/internal/service"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

// CallbackHandler handles callback requests
type CallbackHandler struct {
	domainService    *domain.DomainService
	telegramService  *notification.TelegramService
	emailService     *notification.EmailService
	deepCheckService *service.DeepCheckService
}

// NewCallbackHandler creates a new callback handler
func NewCallbackHandler(
	domainService *domain.DomainService,
	telegramService *notification.TelegramService,
	emailService *notification.EmailService,
	deepCheckService *service.DeepCheckService,
) *CallbackHandler {
	return &CallbackHandler{
		domainService:    domainService,
		telegramService:  telegramService,
		emailService:     emailService,
		deepCheckService: deepCheckService,
	}
}

// HandleCallback logs the incoming request and processes deep check callbacks
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

	// Try to parse as deep check callback
	var deepCheckCallback deepcheck.DeepCheckCallbackRequest
	if err := json.Unmarshal(body, &deepCheckCallback); err == nil && deepCheckCallback.OrderID != "" {
		// This is a deep check callback
		log.Printf("[CALLBACK-%s] Processing deep check callback for order: %s", requestID, deepCheckCallback.OrderID)
		h.processDeepCheckCallback(requestID, &deepCheckCallback)
	} else {
		log.Printf("[CALLBACK-%s] Not a deep check callback, logging only", requestID)
	}

	// Return simple success response
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Callback received",
		"timestamp": time.Now(),
	})
}

// processDeepCheckCallback processes the deep check results and sends notifications
func (h *CallbackHandler) processDeepCheckCallback(requestID string, callback *deepcheck.DeepCheckCallbackRequest) {
	log.Printf("[CALLBACK-%s] Processing deep check results - OrderID: %s, Records: %d",
		requestID, callback.OrderID, callback.Count)

	if h.deepCheckService == nil {
		log.Printf("[CALLBACK-%s] ERROR: Deep check service not available", requestID)
		return
	}

	// Look up the order in database
	order, err := h.deepCheckService.GetDeepCheckOrderByOrderID(callback.OrderID)
	if err != nil {
		log.Printf("[CALLBACK-%s] ERROR: Failed to find deep check order %s: %v",
			requestID, callback.OrderID, err)
		return
	}

	log.Printf("[CALLBACK-%s] Found deep check order: UserID=%d, DomainID=%d, Domain=%s",
		requestID, order.UserID, order.DomainID, order.DomainName)

	// Update the order with callback data
	if err := h.deepCheckService.UpdateDeepCheckOrderCallback(callback.OrderID, callback); err != nil {
		log.Printf("[CALLBACK-%s] ERROR: Failed to update deep check order: %v", requestID, err)
		// Continue with notifications even if we can't update the database
	}

	// Get the current domain information
	domain, err := h.domainService.GetDomain(order.DomainID, order.UserID)
	if err != nil {
		log.Printf("[CALLBACK-%s] ERROR: Failed to get domain %d for user %d: %v",
			requestID, order.DomainID, order.UserID, err)
		return
	}

	log.Printf("[CALLBACK-%s] Retrieved domain: %s (User: %d)", requestID, domain.Name, domain.UserID)

	// Send notifications using the domain information
	h.sendDeepCheckNotifications(requestID, *domain, callback, order.DomainName)
}

// Update the sendDeepCheckNotifications method
func (h *CallbackHandler) sendDeepCheckNotifications(requestID string, domain model.Domain, callback *deepcheck.DeepCheckCallbackRequest, targetDomain string) {
	log.Printf("[CALLBACK-%s] Sending deep check notifications for domain %s (User: %d)",
		requestID, domain.Name, domain.UserID)

	// Send Telegram notification (multiple messages with language support)
	if h.telegramService != nil {
		// Get user's Telegram configurations to determine languages
		configs, err := h.telegramService.GetTelegramConfigsForUser(domain.UserID)
		if err != nil {
			log.Printf("[CALLBACK-%s] ERROR: Failed to get Telegram configs: %v", requestID, err)
		} else if len(configs) > 0 {
			// Send to each config with their preferred language
			for _, config := range configs {
				if !config.IsActive {
					continue
				}

				language := config.Language
				if language == "" {
					language = "en" // Default to English
				}

				log.Printf("[CALLBACK-%s] Formatting Telegram messages for language: %s", requestID, language)
				telegramMessages := callback.FormatTelegramMessage(targetDomain, language)

				// Send messages to this specific config
				if err := h.telegramService.SendMultipleMessagesToConfig(config, telegramMessages); err != nil {
					log.Printf("[CALLBACK-%s] ERROR: Failed to send Telegram messages to config %d: %v",
						requestID, config.ID, err)
				} else {
					log.Printf("[CALLBACK-%s] Successfully sent %d Telegram messages to config %d (%s)",
						requestID, len(telegramMessages), config.ID, language)
				}
			}
		} else {
			log.Printf("[CALLBACK-%s] No active Telegram configs found for user %d", requestID, domain.UserID)
		}
	}

	// Send Email notification (unchanged - emails can be longer)
	if h.emailService != nil {
		subject, htmlBody := callback.FormatEmailMessage(targetDomain)
		if err := h.emailService.SendCustomHTMLMessage(domain.UserID, subject, htmlBody); err != nil {
			log.Printf("[CALLBACK-%s] ERROR: Failed to send email notification: %v", requestID, err)
		} else {
			log.Printf("[CALLBACK-%s] Successfully sent email notification", requestID)
		}
	}
}
