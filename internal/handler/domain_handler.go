package handler

import (
	"net/http"
	"strconv"

	"domain-detection-go/internal/domain"
	"domain-detection-go/pkg/model"

	"github.com/gin-gonic/gin"
)

// DomainHandler handles domain-related HTTP requests
type DomainHandler struct {
	domainService *domain.DomainService
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(domainService *domain.DomainService) *DomainHandler {
	return &DomainHandler{
		domainService: domainService,
	}
}

// GetDomains handles GET /api/domains
func (h *DomainHandler) GetDomains(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	response, err := h.domainService.GetDomains(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch domains"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetDomain handles GET /api/domains/:id
func (h *DomainHandler) GetDomain(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	domainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}

	domain, err := h.domainService.GetDomain(domainID, userID)
	if err != nil {
		if err.Error() == "domain not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch domain"})
		return
	}

	c.JSON(http.StatusOK, domain)
}

// AddDomain handles POST /api/domains
func (h *DomainHandler) AddDomain(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.DomainAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domainID, err := h.domainService.AddDomain(userID, req)
	if err != nil {
		if err.Error() == "invalid domain name format" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain name format"})
			return
		}
		if err.Error() == "domain limit reached" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Domain limit reached"})
			return
		}
		if err.Error() == "domain already exists" {
			c.JSON(http.StatusConflict, gin.H{"error": "This domain is already being monitored"})
			return
		}
		if err.Error() == "interval must be 10, 20, or 30 minutes" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be 10, 20, or 30 minutes"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add domain"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Domain added successfully",
		"id":      domainID,
	})
}

// UpdateDomain handles PUT /api/domains/:id
func (h *DomainHandler) UpdateDomain(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	domainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}

	var req model.DomainUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.domainService.UpdateDomain(domainID, userID, req)
	if err != nil {
		if err.Error() == "domain not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
			return
		}
		if err.Error() == "interval must be 10, 20, or 30 minutes" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be 10, 20, or 30 minutes"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update domain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Domain updated successfully"})
}

// DeleteDomain handles DELETE /api/domains/:id
func (h *DomainHandler) DeleteDomain(c *gin.Context) {
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	domainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}

	err = h.domainService.DeleteDomain(domainID, userID)
	if err != nil {
		if err.Error() == "domain not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete domain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Domain deleted successfully"})
}

// UpdateDomainLimit handles PUT /api/settings/domain-limit
func (h *DomainHandler) UpdateDomainLimit(c *gin.Context) {
	// Admin only endpoint - check for admin role if you have it
	userID := c.GetInt("user_id") // Set by auth middleware
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		UserID int `json:"user_id" binding:"required"`
		Limit  int `json:"limit" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Check if requesting user has admin permissions

	err := h.domainService.UpdateDomainLimit(req.UserID, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update domain limit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Domain limit updated successfully"})
}
