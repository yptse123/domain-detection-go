package handler

import (
	"domain-detection-go/internal/service"
	"domain-detection-go/pkg/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type TelegramPromptHandler struct {
	promptService *service.TelegramPromptService
}

func NewTelegramPromptHandler(promptService *service.TelegramPromptService) *TelegramPromptHandler {
	return &TelegramPromptHandler{promptService: promptService}
}

// GetPrompts with pagination - GET /api/telegram-prompts
func (h *TelegramPromptHandler) GetPrompts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "updated_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}

	response, err := h.promptService.GetPrompts(page, perPage, search, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetPrompt - GET /api/telegram-prompts/:id
func (h *TelegramPromptHandler) GetPrompt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	prompt, err := h.promptService.GetPromptByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Prompt not found"})
		return
	}

	c.JSON(http.StatusOK, prompt)
}

// CreatePrompt - POST /api/telegram-prompts
func (h *TelegramPromptHandler) CreatePrompt(c *gin.Context) {
	var req model.TelegramPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prompt, err := h.promptService.CreatePrompt(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, prompt)
}

// UpdatePrompt - PUT /api/telegram-prompts/:id
func (h *TelegramPromptHandler) UpdatePrompt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req model.TelegramPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prompt, err := h.promptService.UpdatePrompt(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, prompt)
}

// DeletePrompt - DELETE /api/telegram-prompts/:id
func (h *TelegramPromptHandler) DeletePrompt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	err = h.promptService.DeletePrompt(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Prompt deleted successfully"})
}
