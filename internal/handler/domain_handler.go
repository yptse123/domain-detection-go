package handler

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

	// Log user ID for debugging
	log.Printf("Fetching domains for user ID: %d", userID)

	response, err := h.domainService.GetDomains(userID)
	if err != nil {
		// Log the actual error for debugging
		log.Printf("Error fetching domains: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch domains: " + err.Error()})
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
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.DomainAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate region
	if req.Region == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Region is required"})
		return
	}

	// Log the request for debugging
	log.Printf("AddDomain request: %+v for user: %d", req, userID)

	domainID, err := h.domainService.AddDomain(userID, req)
	if err != nil {
		// Log the detailed error
		log.Printf("Error adding domain: %v", err)

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
		if err.Error() == "interval must be 10, 20, 30, 60 or 120 minutes" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be 10, 20, 30, 60 or 120 minutes"})
			return
		}
		if err.Error() == "invalid region" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid region"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add domain: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Domain added successfully",
		"id":      domainID,
	})
}

// AddBatchDomains handles the addition of multiple domains in one request
func (h *DomainHandler) AddBatchDomains(c *gin.Context) {
	// Get user ID from context
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request body
	var req model.DomainBatchAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate request
	if len(req.Domains) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No domains provided"})
		return
	}

	// Limit the number of domains that can be processed in a single request
	const MAX_BATCH_SIZE = 100
	if len(req.Domains) > MAX_BATCH_SIZE {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Too many domains in batch. Maximum allowed is %d", MAX_BATCH_SIZE),
		})
		return
	}

	// Make sure unique domains are considered per region
	uniqueDomains := make(map[string]bool)
	var filteredDomains []model.DomainBatchItem
	for _, domainItem := range req.Domains {
		// Normalize domain (convert to lowercase, trim spaces)
		domainName := strings.TrimSpace(domainItem.Name)
		if domainName == "" {
			continue
		}

		// Extract hostname for duplicate checking
		parsedURL, err := url.Parse(domainName)
		var normalizedKey string

		if err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") {
			// Use hostname+region as the key for URLs with protocol
			normalizedKey = strings.ToLower(parsedURL.Hostname()) + ":" + domainItem.Region
		} else {
			// Use domain+region as key for plain domains
			normalizedKey = strings.ToLower(domainName) + ":" + domainItem.Region
		}

		if !uniqueDomains[normalizedKey] {
			uniqueDomains[normalizedKey] = true
			filteredDomains = append(filteredDomains, model.DomainBatchItem{
				Name:   domainName,
				Region: domainItem.Region,
			})
		}
	}
	req.Domains = filteredDomains

	// Log the batch add request
	log.Printf("Batch add request for user %d: %d domains", userID, len(req.Domains))

	// Process batch domain addition
	response := h.domainService.AddBatchDomains(userID, req)

	// Return appropriate status code based on results
	statusCode := http.StatusOK
	if response.Added == 0 {
		statusCode = http.StatusBadRequest
	} else if len(response.Failed) > 0 {
		statusCode = http.StatusPartialContent // 206 Partial Content
	}

	c.JSON(statusCode, response)
}

// UpdateDomain handles PUT /api/domains/:id
func (h *DomainHandler) UpdateDomain(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	domainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}

	// Log the domain ID and user ID
	log.Printf("Updating domain ID: %d for user ID: %d", domainID, userID)

	var req model.DomainUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Invalid request format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Log the update request
	log.Printf("Update request: %+v", req)

	err = h.domainService.UpdateDomain(domainID, userID, req)
	if err != nil {
		// Log the actual error
		log.Printf("Error updating domain: %v", err)

		if err.Error() == "domain not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
			return
		}
		if err.Error() == "interval must be 10, 20, 30, 60 or 120 minutes" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be 10, 20, 30, 60 or 120 minutes"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update domain: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Domain updated successfully"})
}

// UpdateAllDomains handles PUT /api/domains/batch
func (h *DomainHandler) UpdateAllDomains(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.DomainUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Invalid request format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Log the batch update request
	log.Printf("Batch update request for user %d: %+v", userID, req)

	err := h.domainService.UpdateAllUserDomains(userID, req)
	if err != nil {
		// Log the actual error
		log.Printf("Error batch updating domains: %v", err)

		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
			return
		}

		if err.Error() == "interval must be 10, 20, 30, 60 or 120 minutes" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be 10, 20, 30, 60 or 120 minutes"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update domains: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All domains updated successfully"})
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

	err = h.domainService.DeleteDomain(userID, domainID)
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

// DeleteAllDomains deletes all domains for the authenticated user
func (h *DomainHandler) DeleteAllDomains(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get current domains count for response
	domainsResponse, err := h.domainService.GetDomains(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get current domains"})
		return
	}

	domainsCount := len(domainsResponse.Domains)

	if domainsCount == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":       "No domains to delete",
			"deleted_count": 0,
		})
		return
	}

	// Delete all domains
	err = h.domainService.DeleteAllDomains(userID)
	if err != nil {
		log.Printf("Failed to delete all domains for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete domains"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("Successfully deleted all %d domains", domainsCount),
		"deleted_count": domainsCount,
	})
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

// DeleteBatchDomains handles DELETE /api/domains/batch with domain IDs
func (h *DomainHandler) DeleteBatchDomains(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.DomainBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate domain IDs
	if len(req.DomainIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No domain IDs provided"})
		return
	}

	if len(req.DomainIDs) > 100 { // Reasonable limit
		c.JSON(http.StatusBadRequest, gin.H{"error": "Too many domains. Maximum 100 domains per batch"})
		return
	}

	// Validate all IDs are positive integers
	for _, id := range req.DomainIDs {
		if id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "All domain IDs must be positive integers"})
			return
		}
	}

	// Remove duplicates
	seen := make(map[int]bool)
	uniqueIDs := []int{}
	for _, id := range req.DomainIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	// Delete domains
	response, err := h.domainService.DeleteBatchDomains(userID, uniqueIDs)
	if err != nil {
		log.Printf("Failed to delete batch domains for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete domains"})
		return
	}

	// Return appropriate status code
	statusCode := http.StatusOK
	if response.DeletedCount == 0 {
		statusCode = http.StatusNotFound
	} else if len(response.Failed) > 0 {
		statusCode = http.StatusPartialContent // 206 for partial success
	}

	c.JSON(statusCode, response)
}
