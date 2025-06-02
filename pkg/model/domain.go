package model

import (
	"time"
)

// Domain represents a domain to be monitored
type Domain struct {
	ID                int       `json:"id" db:"id"`
	UserID            int       `json:"user_id" db:"user_id"`
	Name              string    `json:"name" db:"name"`
	Active            bool      `json:"active" db:"active"`
	Interval          int       `json:"interval" db:"interval"` // Interval in minutes
	Region            string    `json:"region" db:"region"`     // Region for this domain
	MonitorGuid       *string   `json:"monitor_guid" db:"monitor_guid"`
	Site24x7MonitorID *string   `json:"site24x7_monitor_id" db:"site24x7_monitor_id"` // Add this field
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
	LastStatus        int       `json:"last_status" db:"last_status"`
	LastCheck         time.Time `json:"last_check,omitempty" db:"last_check"`
	ErrorCode         int       `json:"error_code" db:"error_code"`
	TotalTime         int       `json:"total_time" db:"total_time"`
	ErrorDescription  string    `json:"error_description" db:"error_description"`
}

// GetMonitorGuid returns the monitor GUID as a string (empty if nil)
func (d Domain) GetMonitorGuid() string {
	if d.MonitorGuid != nil {
		return *d.MonitorGuid
	}
	return ""
}

// GetSite24x7MonitorID returns the Site24x7 monitor ID as a string (empty if nil)
func (d Domain) GetSite24x7MonitorID() string {
	if d.Site24x7MonitorID != nil {
		return *d.Site24x7MonitorID
	}
	return ""
}

// DomainAddRequest represents the request to add a new domain
type DomainAddRequest struct {
	Name     string `json:"name" binding:"required"`
	Interval int    `json:"interval"`                  // If not provided, default will be used
	Region   string `json:"region" binding:"required"` // NEW: Required region field
}

// DomainListResponse represents the response for domain listing
type DomainListResponse struct {
	Domains      []Domain `json:"domains"`
	TotalDomains int      `json:"total_domains"`
	DomainLimit  int      `json:"domain_limit"`
}

// DomainStatusResponse represents the response for domain status
type DomainStatusResponse struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Active       bool      `json:"active"`
	LastStatus   int       `json:"last_status"`
	LastCheck    time.Time `json:"last_check"`
	ResponseTime int       `json:"response_time"` // in milliseconds
}

// DomainBatchItem represents a single domain in a batch request
type DomainBatchItem struct {
	Name   string `json:"name" binding:"required"`
	Region string `json:"region" binding:"required"`
}

// DomainBatchAddRequest represents a batch request to add multiple domains
type DomainBatchAddRequest struct {
	Domains  []DomainBatchItem `json:"domains"`  // List of domain items with per-domain regions
	Interval int               `json:"interval"` // Optional, will use default if not provided
}

// DomainBatchAddResponse represents the response for a batch domain add operation
type DomainBatchAddResponse struct {
	Success []DomainAddResult `json:"success"` // Successfully added domains
	Failed  []DomainAddResult `json:"failed"`  // Failed domains with reasons
	Added   int               `json:"added"`   // Count of successfully added domains
	Total   int               `json:"total"`   // Total domains processed
}

// DomainAddResult represents the result for a single domain in batch operation
type DomainAddResult struct {
	Name   string `json:"name"`
	ID     int    `json:"id,omitempty"`     // Only set for successful additions
	Reason string `json:"reason,omitempty"` // Only set for failed additions
}

// DomainUpdateRequest represents the request to update domain settings
type DomainUpdateRequest struct {
	Active   *bool   `json:"active"`
	Interval *int    `json:"interval"` // Interval in minutes
	Region   *string `json:"region"`   // NEW: Optional region field for updates
}

// DomainWithRegion extends Domain with user region info
type DomainWithRegion struct {
	Domain
	UserRegion string `db:"user_region" json:"user_region"`
}

// Available checks if the domain is currently available based on last status
func (d Domain) Available() bool {
	if d.LastCheck.IsZero() {
		return false
	}

	// Consider successful if status is between 200-399
	return d.LastStatus >= 200 && d.LastStatus < 400
}
