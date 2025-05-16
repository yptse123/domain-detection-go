package domain

import (
	"database/sql"
	"errors"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"domain-detection-go/pkg/model"

	"fmt"

	"github.com/jmoiron/sqlx"
)

// DomainService handles domain operations
type DomainService struct {
	db            *sqlx.DB
	monitorClient MonitorClient // Use interface instead of concrete type
}

// NewDomainService creates a new domain service
func NewDomainService(db *sqlx.DB, monitorClient MonitorClient) *DomainService {
	return &DomainService{
		db:            db,
		monitorClient: monitorClient,
	}
}

// DEFAULT_DOMAIN_LIMIT defines the default number of domains a user can add
const DEFAULT_DOMAIN_LIMIT = 100

// DEFAULT_INTERVAL defines the default interval in minutes
const DEFAULT_INTERVAL = 20

// GetDomainLimit returns the domain limit for a user
func (s *DomainService) GetDomainLimit(userID int) (int, error) {
	var limit int
	err := s.db.Get(&limit, `
        SELECT COALESCE(
            (SELECT domain_limit FROM user_settings WHERE user_id = $1),
            $2
        )`, userID, DEFAULT_DOMAIN_LIMIT)

	if err != nil {
		return DEFAULT_DOMAIN_LIMIT, err
	}
	return limit, nil
}

// ValidateDomainName checks if a domain name or URL is valid
func (s *DomainService) ValidateDomainName(input string) bool {
	// Check if the input is a URL with scheme
	parsedURL, err := url.Parse(input)
	if err != nil {
		return false
	}

	var domain string
	if parsedURL.Scheme == "http" || parsedURL.Scheme == "https" {
		// This is a URL with a scheme, extract the host
		domain = parsedURL.Hostname()
	} else {
		// This is likely just a domain name
		domain = input
	}

	// Basic validation
	if len(domain) < 3 || len(domain) > 253 {
		return false
	}

	// Remove trailing dot if present
	domain = strings.TrimSuffix(domain, ".")

	// Check domain pattern
	pattern := `^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	match, _ := regexp.MatchString(pattern, domain)

	return match
}

// AddDomain adds a new domain to monitor
func (s *DomainService) AddDomain(userID int, req model.DomainAddRequest) (int, error) {
	// Validate domain name
	if !s.ValidateDomainName(req.Name) {
		return 0, errors.New("invalid domain name format")
	}

	// Check if user has reached the domain limit
	var count int
	err := s.db.Get(&count, "SELECT COUNT(*) FROM domains WHERE user_id = $1", userID)
	if err != nil {
		return 0, err
	}

	limit, err := s.GetDomainLimit(userID)
	if err != nil {
		return 0, err
	}

	if count >= limit {
		return 0, errors.New("domain limit reached")
	}

	// Check if the domain already exists for this user
	err = s.db.Get(&count, "SELECT COUNT(*) FROM domains WHERE user_id = $1 AND name = $2", userID, req.Name)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		return 0, errors.New("domain already exists")
	}

	// Get user region
	var userRegion string
	err = s.db.Get(&userRegion, "SELECT region FROM users WHERE id = $1", userID)
	if err != nil {
		return 0, fmt.Errorf("error getting user region: %w", err)
	}

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = DEFAULT_INTERVAL
	} else if interval != 10 && interval != 20 && interval != 30 {
		return 0, errors.New("interval must be 10, 20, or 30 minutes")
	}

	// Insert the domain without monitor GUID initially
	var domainID int
	err = s.db.QueryRow(`
        INSERT INTO domains (user_id, name, interval, monitor_guid, active, created_at, updated_at)
        VALUES ($1, $2, $3, '', true, $4, $4)
        RETURNING id
    `, userID, req.Name, interval, time.Now()).Scan(&domainID)

	if err != nil {
		return 0, err
	}

	// Create the monitor asynchronously in the background
	go s.createMonitorAsync(domainID, req.Name, userRegion)

	return domainID, nil
}

// AddBatchDomains adds multiple domains in a batch
func (s *DomainService) AddBatchDomains(userID int, req model.DomainBatchAddRequest) model.DomainBatchAddResponse {
	response := model.DomainBatchAddResponse{
		Success: []model.DomainAddResult{},
		Failed:  []model.DomainAddResult{},
		Total:   len(req.Domains),
	}

	// Check if user has reached the domain limit
	var currentCount int
	err := s.db.Get(&currentCount, "SELECT COUNT(*) FROM domains WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("Error checking domain count: %v", err)
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Internal server error: could not check domain count",
			})
		}
		return response
	}

	limit, err := s.GetDomainLimit(userID)
	if err != nil {
		log.Printf("Error getting domain limit: %v", err)
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Internal server error: could not check domain limit",
			})
		}
		return response
	}

	// Check how many domains we can still add
	availableSlots := limit - currentCount
	if availableSlots <= 0 {
		// User has reached their domain limit
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Domain limit reached",
			})
		}
		return response
	}

	// Get user region
	var userRegion string
	err = s.db.Get(&userRegion, "SELECT region FROM users WHERE id = $1", userID)
	if err != nil {
		log.Printf("Error getting user region: %v", err)
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Internal server error: could not retrieve user region",
			})
		}
		return response
	}

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = DEFAULT_INTERVAL
	} else if interval != 10 && interval != 20 && interval != 30 {
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Invalid interval - must be 10, 20, or 30 minutes",
			})
		}
		return response
	}

	// Get existing domains for this user to avoid duplicates
	existingDomains := make(map[string]bool)
	rows, err := s.db.Query("SELECT name FROM domains WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("Error checking existing domains: %v", err)
		for _, domain := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domain,
				Reason: "Internal server error: could not check existing domains",
			})
		}
		return response
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		existingDomains[strings.ToLower(name)] = true
	}

	// Process each domain
	for _, domainInput := range req.Domains {
		// Skip if we've reached the limit
		if response.Added >= availableSlots {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainInput,
				Reason: "Domain limit reached",
			})
			continue
		}

		// Normalize input
		domainInput = strings.TrimSpace(domainInput)

		// Validate domain or URL
		if !s.ValidateDomainName(domainInput) {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainInput,
				Reason: "Invalid domain name format",
			})
			continue
		}

		// Parse URL to ensure consistent storage
		parsedURL, err := url.Parse(domainInput)
		if err != nil {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainInput,
				Reason: "Invalid URL format",
			})
			continue
		}

		// Ensure there's a scheme, default to https if not specified
		fullURL := domainInput
		if parsedURL.Scheme == "" {
			fullURL = "https://" + domainInput
			parsedURL, _ = url.Parse(fullURL)
		}

		// Check if domain already exists (case insensitive)
		// We compare by hostname to avoid duplicates with different protocols
		hostname := parsedURL.Hostname()
		if existingDomains[strings.ToLower(hostname)] {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainInput,
				Reason: "Domain already exists",
			})
			continue
		}

		// Insert the full URL in the database
		var domainID int
		err = s.db.QueryRow(`
        INSERT INTO domains (user_id, name, interval, monitor_guid, active, created_at, updated_at)
        VALUES ($1, $2, $3, '', true, $4, $4)
        RETURNING id
    `, userID, fullURL, interval, time.Now()).Scan(&domainID)

		if err != nil {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainInput,
				Reason: "Failed to insert domain: " + err.Error(),
			})
			continue
		}

		// Create monitor asynchronously with the full URL
		go s.createMonitorAsync(domainID, fullURL, userRegion)

		// Mark domain as successfully added
		response.Success = append(response.Success, model.DomainAddResult{
			Name: fullURL, // Return the full URL with protocol
			ID:   domainID,
		})
		response.Added++

		// Add to our existing domains map to prevent duplicates within the batch
		existingDomains[strings.ToLower(hostname)] = true
	}

	return response
}

// createMonitorAsync creates a monitor in Uptrends and updates the domain record
func (s *DomainService) createMonitorAsync(domainID int, fullURL, userRegion string) {
	// Add some delay to prevent overwhelming the Uptrends API
	time.Sleep(100 * time.Millisecond)

	// Extract domain name for the monitor name
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		log.Printf("Failed to parse URL for monitor creation: %v", err)
		return
	}

	monitorName := fmt.Sprintf("Domain Check - %s", parsedURL)

	// Create monitor in monitoring service
	monitorGuid, err := s.monitorClient.CreateMonitor(fullURL, monitorName, userRegion)
	if err != nil {
		log.Printf("Failed to create monitor for domain %d (%s): %v", domainID, fullURL, err)
		// We'll try again during the next system check, but for now just exit
		return
	}

	// Update the domain with the monitor GUID
	_, err = s.db.Exec(`
        UPDATE domains 
        SET monitor_guid = $1, updated_at = NOW() 
        WHERE id = $2
    `, monitorGuid, domainID)

	if err != nil {
		log.Printf("Failed to update domain %d with monitor GUID: %v", domainID, err)
		// Consider deleting the created monitor to avoid orphaned monitors
		if monitorGuid != "" {
			if delErr := s.monitorClient.DeleteMonitor(monitorGuid); delErr != nil {
				log.Printf("Failed to delete orphaned monitor %s: %v", monitorGuid, delErr)
			}
		}
	} else {
		log.Printf("Successfully created and linked monitor for domain %d (%s)", domainID, fullURL)
	}
}

// GetDomains gets all domains for a user
func (s *DomainService) GetDomains(userID int) (model.DomainListResponse, error) {
	var domains []model.Domain

	// Update this query to handle NULL values in the active column
	err := s.db.Select(&domains, `
        SELECT 
            d.id, 
            d.user_id, 
            d.name, 
            COALESCE(d.active, false) AS active, -- Use COALESCE to handle NULL
            d.last_status, 
            d.error_code, 
            d.error_description, 
            d.last_check, 
            d.monitor_guid, 
            d.interval,
            d.total_time
        FROM domains d
        WHERE d.user_id = $1
        ORDER BY d.created_at DESC
    `, userID)

	if err != nil {
		return model.DomainListResponse{}, err
	}

	// Get domain count and limit
	var count int
	err = s.db.Get(&count, "SELECT COUNT(*) FROM domains WHERE user_id = $1", userID)
	if err != nil {
		return model.DomainListResponse{}, err
	}

	limit, err := s.GetDomainLimit(userID)
	if err != nil {
		return model.DomainListResponse{}, err
	}

	return model.DomainListResponse{
		Domains:      domains,
		TotalDomains: count,
		DomainLimit:  limit,
	}, nil
}

// GetDomain gets a single domain by ID
func (s *DomainService) GetDomain(domainID, userID int) (*model.Domain, error) {
	var domain model.Domain
	err := s.db.Get(&domain, `
        SELECT id, user_id, name, active, interval, last_status, error_code,
               total_time, error_description, monitor_guid, last_check, 
               created_at, updated_at
        FROM domains
        WHERE id = $1 AND user_id = $2
    `, domainID, userID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("domain not found")
		}
		return nil, err
	}

	return &domain, nil
}

// UpdateDomain updates domain settings
func (s *DomainService) UpdateDomain(domainID, userID int, req model.DomainUpdateRequest) error {
	// First check if domain exists and belongs to user
	var domain model.Domain
	err := s.db.Get(&domain, "SELECT * FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("domain not found")
		}
		return err
	}

	// Build update query
	query := "UPDATE domains SET updated_at = NOW()"
	params := []interface{}{}
	paramIndex := 1

	// Add the fields to update conditionally
	if req.Active != nil {
		query += fmt.Sprintf(", active = $%d", paramIndex)
		params = append(params, *req.Active)
		paramIndex++
	}

	if req.Interval != nil {
		// Validate interval
		if *req.Interval != 10 && *req.Interval != 20 && *req.Interval != 30 {
			return errors.New("interval must be 10, 20, or 30 minutes")
		}

		query += fmt.Sprintf(", interval = $%d", paramIndex)
		params = append(params, *req.Interval)
		paramIndex++
	}

	// Add WHERE clause
	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", paramIndex, paramIndex+1)
	params = append(params, domainID, userID)

	// Execute update
	log.Printf("Executing query: %s with params: %v", query, params)
	_, err = s.db.Exec(query, params...)
	if err != nil {
		return err
	}

	// If monitor_guid exists, update monitor status in Uptrends
	if domain.MonitorGuid != "" {
		if err := s.monitorClient.UpdateMonitorStatus(domain.MonitorGuid, *req.Active); err != nil {
			log.Printf("Failed to update monitor status: %v", err)
			// Continue despite the error - we've updated the database already
		}
	}

	return nil
}

// UpdateAllUserDomains updates settings for all domains of a user
func (s *DomainService) UpdateAllUserDomains(userID int, req model.DomainUpdateRequest) error {
	// Get all user's domains with their monitor_guids
	var domains []model.Domain
	err := s.db.Select(&domains, "SELECT id, monitor_guid FROM domains WHERE user_id = $1", userID)
	if err != nil {
		return err
	}

	// Build dynamic SQL update query based on which fields are provided
	query := "UPDATE domains SET updated_at = NOW()"
	params := []interface{}{}
	paramIndex := 1

	// Add active field if provided
	if req.Active != nil {
		query += fmt.Sprintf(", active = $%d", paramIndex)
		params = append(params, *req.Active)
		paramIndex++
	}

	// Add interval field if provided
	if req.Interval != nil {
		// Validate interval
		if *req.Interval != 10 && *req.Interval != 20 && *req.Interval != 30 {
			return errors.New("interval must be 10, 20, or 30 minutes")
		}

		query += fmt.Sprintf(", interval = $%d", paramIndex)
		params = append(params, *req.Interval)
		paramIndex++
	}

	// Add WHERE clause
	query += fmt.Sprintf(" WHERE user_id = $%d", paramIndex)
	params = append(params, userID)

	// Execute the update if we have at least one field to update
	if paramIndex > 1 {
		log.Printf("Executing query: %s with params: %v", query, params)
		_, err = s.db.Exec(query, params...)
		if err != nil {
			return err
		}
	}

	// Only update monitors in Uptrends if active status is changing
	if req.Active != nil {
		for _, domain := range domains {
			if domain.MonitorGuid != "" {
				if err := s.monitorClient.UpdateMonitorStatus(domain.MonitorGuid, *req.Active); err != nil {
					log.Printf("Failed to update monitor status for domain %d: %v", domain.ID, err)
					// Continue with other domains despite errors
				}
			}
		}
	}

	return nil
}

// DeleteDomain deletes a domain
func (s *DomainService) DeleteDomain(userID, domainID int) error {
	// First get the domain to retrieve its monitor_guid
	var domain model.Domain
	err := s.db.Get(&domain, "SELECT * FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Domain not found for user %d: %d", userID, domainID)
			return errors.New("domain not found or not owned by user")
		}
		return err
	}

	// If there's a monitor GUID, delete the monitor in Uptrends
	if domain.MonitorGuid != "" {
		if err := s.monitorClient.DeleteMonitor(domain.MonitorGuid); err != nil {
			// Log the error but continue with domain deletion
			log.Printf("Failed to delete monitor %s: %v", domain.MonitorGuid, err)
		}
	}

	// Delete domain from database
	_, err = s.db.Exec("DELETE FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	if err != nil {
		return err
	}

	return nil
}

// UpdateDomainLimit updates the domain limit for a user
func (s *DomainService) UpdateDomainLimit(userID int, limit int) error {
	if limit < 1 {
		return errors.New("limit must be at least 1")
	}

	// Upsert the user settings
	_, err := s.db.Exec(`
        INSERT INTO user_settings (user_id, domain_limit, updated_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (user_id)
        DO UPDATE SET domain_limit = $2, updated_at = NOW()
    `, userID, limit)

	return err
}

// GetAllActiveDomains gets all active domains across all users
func (s *DomainService) GetAllActiveDomains() ([]model.Domain, error) {
	var domains []model.Domain

	query := `
        SELECT id, user_id, name, active, interval, last_status, last_check, created_at, updated_at
        FROM domains
        WHERE active = true
    `

	err := s.db.Select(&domains, query)
	return domains, err
}

// GetAllActiveDomainsWithMonitors gets all active domains with monitor GUIDs
func (s *DomainService) GetAllActiveDomainsWithMonitors() ([]model.Domain, error) {
	var domains []model.Domain

	query := `
        SELECT id, user_id, name, active, interval, monitor_guid, last_status,
               error_code, total_time, error_description, last_check, 
               created_at, updated_at
        FROM domains 
        WHERE active = true
        AND monitor_guid IS NOT NULL 
        AND monitor_guid != ''
    `

	err := s.db.Select(&domains, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching active domains with monitors: %w", err)
	}

	return domains, nil
}

// UpdateDomainStatus updates the status of a domain
func (s *DomainService) UpdateDomainStatus(domainID int, statusCode, errorCode, totalTime int, errorDescription string) error {
	// Update last_status in domains table
	_, err := s.db.Exec(`
        UPDATE domains 
        SET last_status = $1, last_check = NOW(), updated_at = NOW()
        WHERE id = $2
    `, statusCode, domainID)
	if err != nil {
		return err
	}

	// Update domain status details
	_, err = s.db.Exec(`
		UPDATE domains
		SET 
			last_status = $1,
			error_code = $2,
			total_time = $3,
			error_description = $4,
			last_check = NOW(),
			updated_at = NOW()
		WHERE id = $5
		`, statusCode, errorCode, totalTime, errorDescription, domainID)

	return err
}

// GetAllActiveDomainsWithUserRegions gets all active domains with their user regions
func (s *DomainService) GetAllActiveDomainsWithUserRegions() ([]model.DomainWithRegion, error) {
	var domains []model.DomainWithRegion

	query := `
        SELECT d.id, d.user_id, d.name, d.active, d.interval, 
               d.last_status, d.last_check, d.created_at, d.updated_at,
               u.region as user_region
        FROM domains d
        JOIN users u ON d.user_id = u.id
        WHERE d.active = true
    `

	err := s.db.Select(&domains, query)
	return domains, err
}

// GetAllDomainsWithMonitors gets all domains that have associated monitors
func (s *DomainService) GetAllDomainsWithMonitors() ([]model.Domain, error) {
	var domains []model.Domain

	// Query to get all domains with non-null monitor_guid
	query := `
        SELECT id, user_id, name, active, interval, monitor_guid, last_status, last_check, created_at, updated_at
        FROM domains 
        WHERE monitor_guid IS NOT NULL AND monitor_guid != ''
    `

	err := s.db.Select(&domains, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching domains with monitors: %w", err)
	}

	return domains, nil
}
