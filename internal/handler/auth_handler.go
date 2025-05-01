package handler

import (
	"net/http"

	"domain-detection-go/internal/auth"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication related HTTP requests
type AuthHandler struct {
	authService *auth.AuthService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *auth.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var creds model.UserCredentials
	if err := c.ShouldBindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	user, token, err := h.authService.Login(creds)
	if err != nil {
		if err.Error() == "2fa_required" {
			// Special case: 2FA is enabled, but code not provided
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":       "2FA code required",
				"require_2fa": true,
			})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"region":   user.Region,
		},
	})
}

// SetupTwoFactor initiates 2FA setup for a user
func (h *AuthHandler) SetupTwoFactor(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	setupData, err := h.authService.SetupTwoFactor(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to setup 2FA"})
		return
	}

	c.JSON(http.StatusOK, setupData)
}

// VerifyTwoFactor verifies and enables 2FA
func (h *AuthHandler) VerifyTwoFactor(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.TwoFactorVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	err := h.authService.VerifyAndEnableTwoFactor(userID, req.TOTPCode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication enabled"})
}

// DisableTwoFactor disables 2FA for a user
func (h *AuthHandler) DisableTwoFactor(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := h.authService.DisableTwoFactor(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable 2FA"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication disabled"})
}

// GetUserProfile returns the current user's profile data
func (h *AuthHandler) GetUserProfile(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user data"})
		return
	}

	// Return only needed fields (don't send sensitive data)
	c.JSON(http.StatusOK, gin.H{
		"username":         user.Username,
		"email":            user.Email,
		"twoFactorEnabled": user.TwoFactorEnabled,
		"region":           user.Region,
	})
}
