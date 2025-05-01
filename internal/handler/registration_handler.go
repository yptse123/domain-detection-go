package handler

import (
	"domain-detection-go/pkg/model"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := h.authService.RegisterUser(req)
	if err != nil {
		if err.Error() == "username already exists" || err.Error() == "email already exists" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err.Error() == "invalid region" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid region selected"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	c.JSON(http.StatusCreated, model.RegistrationResponse{
		Message: "Registration successful",
		UserID:  userID,
	})
}

// GetRegions returns all active regions
func (h *AuthHandler) GetRegions(c *gin.Context) {
	regions, err := h.authService.GetRegions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch regions"})
		return
	}

	c.JSON(http.StatusOK, regions)
}
