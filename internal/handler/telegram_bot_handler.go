package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"domain-detection-go/internal/domain"
	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

type TelegramBotHandler struct {
	telegramService *notification.TelegramService
	domainService   *domain.DomainService
}

func NewTelegramBotHandler(telegramService *notification.TelegramService, domainService *domain.DomainService) *TelegramBotHandler {
	return &TelegramBotHandler{
		telegramService: telegramService,
		domainService:   domainService,
	}
}

// WebhookHandler handles incoming webhook requests from Telegram
func (h *TelegramBotHandler) WebhookHandler(c *gin.Context) {
	var update TelegramUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		log.Printf("Error parsing webhook: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Handle different types of updates
	if update.Message != nil {
		h.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		h.handleCallbackQuery(update.CallbackQuery)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleMessage processes incoming text messages
func (h *TelegramBotHandler) handleMessage(message *TelegramMessage) {
	if message.Text == "" {
		return
	}

	chatID := fmt.Sprintf("%d", message.Chat.ID)

	switch {
	case strings.HasPrefix(message.Text, "/rm"):
		h.handleRemoveCommand(chatID)
	case strings.HasPrefix(message.Text, "/start"):
		h.handleStartCommand(chatID)
	case strings.HasPrefix(message.Text, "/help"):
		h.handleHelpCommand(chatID)
	}
}

// handleRemoveCommand processes the /rm command
func (h *TelegramBotHandler) handleRemoveCommand(chatID string) {
	// Find user by chat ID
	userID, err := h.telegramService.GetUserIDByChatID(chatID)
	if err != nil {
		h.telegramService.SendMessage(chatID, "❌ You are not registered or your Telegram is not configured. Please configure your Telegram notifications first.")
		return
	}

	// Get user's domains
	domainResponse, err := h.domainService.GetDomains(userID)
	if err != nil {
		h.telegramService.SendMessage(chatID, "❌ Error retrieving your domains. Please try again later.")
		return
	}

	if len(domainResponse.Domains) == 0 {
		h.telegramService.SendMessage(chatID, "📭 You don't have any domains to remove.")
		return
	}

	// Create inline keyboard with domain options
	keyboard := h.createDomainSelectionKeyboard(domainResponse.Domains)

	message := "🗑️ **Select a domain to remove:**\n\n"
	for i, domain := range domainResponse.Domains {
		status := "🟢"
		if !domain.Available() {
			status = "🔴"
		}
		message += fmt.Sprintf("%d. %s %s (%s)\n", i+1, status, domain.Name, domain.Region)
	}

	h.telegramService.SendMessageWithKeyboard(chatID, message, keyboard)
}

// handleCallbackQuery processes inline keyboard button clicks
func (h *TelegramBotHandler) handleCallbackQuery(callback *TelegramCallbackQuery) {
	chatID := fmt.Sprintf("%d", callback.Message.Chat.ID)

	if strings.HasPrefix(callback.Data, "remove_domain_") {
		h.handleDomainRemoval(chatID, callback.Data, callback.ID)
	}
}

// handleDomainRemoval processes domain removal
func (h *TelegramBotHandler) handleDomainRemoval(chatID, callbackData, callbackQueryID string) {
	// Extract domain ID from callback data
	parts := strings.Split(callbackData, "_")
	if len(parts) != 3 {
		h.telegramService.AnswerCallbackQuery(callbackQueryID, "❌ Invalid selection")
		return
	}

	domainID, err := strconv.Atoi(parts[2])
	if err != nil {
		h.telegramService.AnswerCallbackQuery(callbackQueryID, "❌ Invalid domain ID")
		return
	}

	// Find user by chat ID
	userID, err := h.telegramService.GetUserIDByChatID(chatID)
	if err != nil {
		h.telegramService.AnswerCallbackQuery(callbackQueryID, "❌ User not found")
		return
	}

	// Get domain details before deletion
	domain, err := h.domainService.GetDomain(domainID, userID)
	if err != nil {
		h.telegramService.AnswerCallbackQuery(callbackQueryID, "❌ Domain not found")
		return
	}

	// Delete the domain
	err = h.domainService.DeleteDomain(userID, domainID)
	if err != nil {
		h.telegramService.AnswerCallbackQuery(callbackQueryID, "❌ Failed to delete domain")
		h.telegramService.SendMessage(chatID, fmt.Sprintf("❌ Failed to remove domain **%s** (%s): %s", domain.Name, domain.Region, err.Error()))
		return
	}

	// Success response
	h.telegramService.AnswerCallbackQuery(callbackQueryID, "✅ Domain removed successfully")
	h.telegramService.SendMessage(chatID, fmt.Sprintf("✅ Successfully removed domain **%s** (%s)", domain.Name, domain.Region))
}

// createDomainSelectionKeyboard creates an inline keyboard for domain selection
func (h *TelegramBotHandler) createDomainSelectionKeyboard(domains []model.Domain) [][]notification.TelegramInlineKeyboardButton {
	var keyboard [][]notification.TelegramInlineKeyboardButton

	for _, domain := range domains {
		status := "🟢"
		if !domain.Available() {
			status = "🔴"
		}

		buttonText := fmt.Sprintf("%s %s (%s)", status, domain.Name, domain.Region)
		callbackData := fmt.Sprintf("remove_domain_%d", domain.ID)

		button := notification.TelegramInlineKeyboardButton{
			Text:         buttonText,
			CallbackData: callbackData,
		}

		keyboard = append(keyboard, []notification.TelegramInlineKeyboardButton{button})
	}

	return keyboard
}

// handleStartCommand handles /start command
func (h *TelegramBotHandler) handleStartCommand(chatID string) {
	message := `👋 Welcome to Domain Monitor Bot!

Available commands:
/rm - Remove a domain from monitoring
/help - Show this help message

To get started, please configure your Telegram notifications in the web interface.`

	h.telegramService.SendMessage(chatID, message)
}

// handleHelpCommand handles /help command
func (h *TelegramBotHandler) handleHelpCommand(chatID string) {
	message := `🤖 Domain Monitor Bot Help

**Available Commands:**
/rm - Remove a domain from monitoring
/help - Show this help message

**How to use /rm:**
1. Type /rm
2. Select a domain from the list
3. Confirm removal

**Note:** You need to configure your Telegram notifications in the web interface first.`

	h.telegramService.SendMessage(chatID, message)
}

// Telegram webhook data structures
type TelegramUpdate struct {
	UpdateID      int                    `json:"update_id"`
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

type TelegramMessage struct {
	MessageID int           `json:"message_id"`
	From      *TelegramUser `json:"from,omitempty"`
	Chat      TelegramChat  `json:"chat"`
	Date      int           `json:"date"`
	Text      string        `json:"text,omitempty"`
}

type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
