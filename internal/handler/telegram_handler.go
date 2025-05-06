package handler

import (
	"net/http"
	"strconv"

	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

// TelegramHandler handles telegram configuration requests
type TelegramHandler struct {
	telegramService *notification.TelegramService
}

// NewTelegramHandler creates a new telegram handler
func NewTelegramHandler(telegramService *notification.TelegramService) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
	}
}

// GetBotInfo returns information about the configured Telegram bot
func (h *TelegramHandler) GetBotInfo(c *gin.Context) {
	bot, err := h.telegramService.SetupBot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get bot information: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bot": bot,
	})
}

// AddTelegramConfig adds a new Telegram notification configuration
func (h *TelegramHandler) AddTelegramConfig(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.TelegramConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Call service method with updated parameters (no domains)
	configID, err := h.telegramService.AddTelegramConfig(
		userID,
		req.ChatID,
		req.ChatName,
		req.NotifyOnDown,
		req.NotifyOnUp,
		req.IsActive, // This should match the field name from your TelegramConfigRequest struct
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      configID,
		"message": "Telegram configuration added successfully",
	})
}

// GetTelegramConfigs retrieves all Telegram configurations for a user
func (h *TelegramHandler) GetTelegramConfigs(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	configs, err := h.telegramService.GetTelegramConfigsForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configs": configs,
	})
}

// UpdateTelegramConfig updates a specific Telegram configuration
func (h *TelegramHandler) UpdateTelegramConfig(c *gin.Context) {
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

	var req model.TelegramConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Call service method with updated parameters (no domains)
	err = h.telegramService.UpdateTelegramConfig(
		configID,
		userID,
		req.ChatID,
		req.ChatName,
		req.NotifyOnDown,
		req.NotifyOnUp,
		req.IsActive,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Telegram configuration updated successfully",
	})
}

// DeleteTelegramConfig deletes a Telegram configuration
func (h *TelegramHandler) DeleteTelegramConfig(c *gin.Context) {
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

	err = h.telegramService.DeleteTelegramConfig(configID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Telegram configuration deleted successfully",
	})
}
