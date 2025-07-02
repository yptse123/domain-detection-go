package handler

import (
	"net/http"
	"strconv"

	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

// EmailHandler handles email configuration requests
type EmailHandler struct {
	emailService *notification.EmailService
}

// NewEmailHandler creates a new email handler
func NewEmailHandler(emailService *notification.EmailService) *EmailHandler {
	return &EmailHandler{
		emailService: emailService,
	}
}

// AddEmailConfig adds a new email notification configuration
func (h *EmailHandler) AddEmailConfig(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.EmailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configID, err := h.emailService.AddEmailConfig(
		userID,
		req.EmailAddress,
		req.EmailName,
		req.Language,
		req.NotifyOnDown,
		req.NotifyOnUp,
		req.IsActive,
		req.MonitorRegions,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      configID,
		"message": "Email configuration added successfully",
	})
}

// GetEmailConfigs retrieves all email configurations for a user
func (h *EmailHandler) GetEmailConfigs(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	configs, err := h.emailService.GetEmailConfigsForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configs": configs,
	})
}

// UpdateEmailConfig updates a specific email configuration
func (h *EmailHandler) UpdateEmailConfig(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	configIDStr := c.Param("id")
	configID, err := strconv.Atoi(configIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	var req model.EmailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.emailService.UpdateEmailConfig(
		configID,
		userID,
		req.EmailAddress,
		req.EmailName,
		req.Language,
		req.NotifyOnDown,
		req.NotifyOnUp,
		req.IsActive,
		req.MonitorRegions,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email configuration updated successfully",
	})
}

// DeleteEmailConfig deletes an email configuration
func (h *EmailHandler) DeleteEmailConfig(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	configIDStr := c.Param("id")
	configID, err := strconv.Atoi(configIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	err = h.emailService.DeleteEmailConfig(configID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email configuration deleted successfully",
	})
}

// SendTestEmail sends a test email to a specific email configuration
func (h *EmailHandler) SendTestEmail(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	configIDStr := c.Param("id")
	configID, err := strconv.Atoi(configIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	// Get user's email configs
	configs, err := h.emailService.GetEmailConfigsForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve configurations"})
		return
	}

	var targetConfig *model.EmailConfig
	for _, config := range configs {
		if config.ID == configID {
			targetConfig = &config
			break
		}
	}

	if targetConfig == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Configuration not found or not owned by you"})
		return
	}

	// Send test email
	err = h.emailService.SendTestEmail(*targetConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send test email: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Test email sent successfully",
	})
}
