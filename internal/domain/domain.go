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
	db             *sqlx.DB
	uptrendsClient MonitorClient
	site24x7Client MonitorClient
}

// NewDomainService creates a new domain service
func NewDomainService(db *sqlx.DB, uptrendsClient MonitorClient, site24x7Client MonitorClient) *DomainService {
	return &DomainService{
		db:             db,
		uptrendsClient: uptrendsClient,
		site24x7Client: site24x7Client,
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

	// Validate the region
	var isValidRegion bool
	err = s.db.Get(&isValidRegion, "SELECT EXISTS(SELECT 1 FROM regions WHERE code = $1 AND is_active = TRUE)", req.Region)
	if err != nil {
		return 0, fmt.Errorf("error verifying region: %w", err)
	}
	if !isValidRegion {
		return 0, errors.New("invalid region")
	}

	// Parse URL to ensure consistent storage
	parsedURL, err := url.Parse(req.Name)
	if err != nil {
		return 0, fmt.Errorf("invalid URL format: %w", err)
	}

	// Ensure there's a scheme, default to https if not specified
	fullURL := req.Name
	if parsedURL.Scheme == "" {
		fullURL = "https://" + req.Name
	}

	err = s.db.Get(&count, `
    SELECT COUNT(*) FROM domains 
    WHERE user_id = $1 AND LOWER(name) = LOWER($2) AND region = $3
`, userID, fullURL, req.Region)

	if err != nil {
		return 0, err
	}

	if count > 0 {
		return 0, errors.New("domain already exists in this region")
	}

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = DEFAULT_INTERVAL
	} else if interval != 10 && interval != 20 && interval != 30 && interval != 60 && interval != 120 {
		return 0, errors.New("interval must be 10, 20, 30, 60 or 120 minutes")
	}

	// Insert the domain with the region and is_deep_check specified in the request
	var domainID int
	if !req.IsDeepCheck {
		req.IsDeepCheck = false // Ensure it's set to false if not specified
	}
	err = s.db.QueryRow(`
        INSERT INTO domains (user_id, name, interval, monitor_guid, active, region, is_deep_check, created_at, updated_at)
        VALUES ($1, $2, $3, '', true, $4, $5, $6, $6)
        RETURNING id
    `, userID, fullURL, interval, req.Region, req.IsDeepCheck, time.Now()).Scan(&domainID)

	if err != nil {
		return 0, err
	}

	// Create the monitor asynchronously in the background using the domain's region
	go s.createMonitorAsync(domainID, fullURL, req.Region)

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
		for _, domainItem := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Internal server error: could not check domain count",
			})
		}
		return response
	}

	limit, err := s.GetDomainLimit(userID)
	if err != nil {
		log.Printf("Error getting domain limit: %v", err)
		for _, domainItem := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Internal server error: could not check domain limit",
			})
		}
		return response
	}

	// Check how many domains we can still add
	availableSlots := limit - currentCount
	if availableSlots <= 0 {
		// User has reached their domain limit
		for _, domainItem := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Domain limit reached",
			})
		}
		return response
	}

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = DEFAULT_INTERVAL
	} else if interval != 10 && interval != 20 && interval != 30 && interval != 60 && interval != 120 {
		for _, domainItem := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Invalid interval - must be 10, 20, 30, 60 or 120 minutes",
			})
		}
		return response
	}

	// Get existing domains for this user to avoid duplicates
	existingDomains := make(map[string]bool)
	rows, err := s.db.Query("SELECT name, region FROM domains WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("Error checking existing domains: %v", err)
		for _, domainItem := range req.Domains {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Internal server error: could not check existing domains",
			})
		}
		return response
	}
	defer rows.Close()

	// Store normalized hostnames with regions for duplicate detection
	for rows.Next() {
		var fullURL, region string
		if err := rows.Scan(&fullURL, &region); err != nil {
			continue
		}

		// Extract hostname from URL if it contains protocol
		parsedURL, err := url.Parse(fullURL)
		if err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") {
			// Use hostname+region as the key
			existingDomains[strings.ToLower(parsedURL.Hostname())+":"+region] = true
		} else {
			existingDomains[strings.ToLower(fullURL)+":"+region] = true
		}
	}

	// Process each domain
	for _, domainItem := range req.Domains {
		// Skip if we've reached the limit
		if response.Added >= availableSlots {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Domain limit reached",
			})
			continue
		}

		// Normalize input
		domainInput := strings.TrimSpace(domainItem.Name)

		// Validate domain or URL
		if !s.ValidateDomainName(domainInput) {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Invalid domain name format",
			})
			continue
		}

		// Validate region
		var isValidRegion bool
		err = s.db.Get(&isValidRegion, "SELECT EXISTS(SELECT 1 FROM regions WHERE code = $1 AND is_active = TRUE)", domainItem.Region)
		if err != nil {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Internal server error: could not verify region",
			})
			continue
		}

		if !isValidRegion {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Invalid region: " + domainItem.Region,
			})
			continue
		}

		// Parse URL to ensure consistent storage
		parsedURL, err := url.Parse(domainInput)
		if err != nil {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Invalid URL format",
			})
			continue
		}

		// Ensure there's a scheme, default to https if not specified
		fullURL := domainInput
		if parsedURL.Scheme == "" {
			fullURL = "https://" + domainInput
		}

		// Create combined key with domain+region for duplicate checking
		domainKey := strings.ToLower(fullURL) + ":" + domainItem.Region

		// Check if this domain+region combination already exists
		if existingDomains[domainKey] {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Domain already exists in this region",
			})
			continue
		}

		// Insert the domain with the per-domain region
		var domainID int
		if !domainItem.IsDeepCheck {
			domainItem.IsDeepCheck = false // Ensure it's set to false if not specified
		}
		err = s.db.QueryRow(`
			INSERT INTO domains (user_id, name, interval, monitor_guid, active, region, is_deep_check, created_at, updated_at)
			VALUES ($1, $2, $3, '', true, $4, $5, $6, $6)
			RETURNING id
		`, userID, fullURL, interval, domainItem.Region, domainItem.IsDeepCheck, time.Now()).Scan(&domainID)

		if err != nil {
			response.Failed = append(response.Failed, model.DomainAddResult{
				Name:   domainItem.Name,
				Reason: "Failed to insert domain: " + err.Error(),
			})
			continue
		}

		// Create monitor asynchronously using domain-specific region
		go s.createMonitorAsync(domainID, fullURL, domainItem.Region)

		// Mark domain as successfully added
		response.Success = append(response.Success, model.DomainAddResult{
			Name: domainItem.Name,
			ID:   domainID,
		})
		response.Added++

		// Add to our existing domains map to prevent duplicates within the batch
		existingDomains[strings.ToLower(fullURL)+":"+domainItem.Region] = true
	}

	return response
}

// createMonitorAsync creates a monitor in Uptrends and updates the domain record
func (s *DomainService) createMonitorAsync(domainID int, fullURL, domainRegion string) {
	// Add some delay to prevent overwhelming the APIs
	time.Sleep(100 * time.Millisecond)

	// Extract domain name for the monitor name
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		log.Printf("Failed to parse URL for monitor creation: %v", err)
		return
	}

	displayName := parsedURL.Hostname()
	monitorName := fmt.Sprintf("Domain Check - %s", displayName)

	// Create array of regions to use (primary + fallbacks)
	regions := []string{domainRegion}

	// Add fallback regions based on primary region
	switch domainRegion {
	case "TH", "ID", "KR":
		regions = append(regions, "VN") // Add Vietnam
		log.Printf("Adding Vietnam fallback region for domain %d with primary region %s", domainID, domainRegion)
	case "VN":
		regions = append(regions, "TH") // Add Thailand
		log.Printf("Adding Thailand fallback region for domain %d with primary region %s", domainID, domainRegion)
	}

	var uptrendsGuid, site24x7ID string
	var uptrendsErr, site24x7Err error

	// Create monitor in Uptrends
	if s.uptrendsClient != nil {
		uptrendsGuid, uptrendsErr = s.uptrendsClient.CreateMonitor(fullURL, monitorName, regions)
		if uptrendsErr != nil {
			log.Printf("Failed to create Uptrends monitor for domain %d (%s): %v", domainID, fullURL, uptrendsErr)
		} else {
			log.Printf("Successfully created Uptrends monitor %s for domain %d", uptrendsGuid, domainID)
		}
	}

	// Create monitor in Site24x7
	if s.site24x7Client != nil {
		site24x7ID, site24x7Err = s.site24x7Client.CreateMonitor(fullURL, monitorName, regions)
		if site24x7Err != nil {
			log.Printf("Failed to create Site24x7 monitor for domain %d (%s): %v", domainID, fullURL, site24x7Err)
		} else {
			log.Printf("Successfully created Site24x7 monitor %s for domain %d", site24x7ID, domainID)
		}
	}

	// Handle NULL values properly for database update
	var uptrendsParam, site24x7Param interface{}

	if uptrendsGuid == "" {
		uptrendsParam = nil
	} else {
		uptrendsParam = uptrendsGuid
	}

	if site24x7ID == "" {
		site24x7Param = nil
	} else {
		site24x7Param = site24x7ID
	}

	// Update the domain with both monitor IDs
	_, err = s.db.Exec(`
        UPDATE domains 
        SET monitor_guid = $1, site24x7_monitor_id = $2, updated_at = NOW() 
        WHERE id = $3
    `, uptrendsParam, site24x7Param, domainID)

	if err != nil {
		log.Printf("Failed to update domain %d with monitor IDs: %v", domainID, err)

		// Clean up created monitors if database update failed
		if uptrendsGuid != "" && s.uptrendsClient != nil {
			if delErr := s.uptrendsClient.DeleteMonitor(uptrendsGuid); delErr != nil {
				log.Printf("Failed to delete orphaned Uptrends monitor %s: %v", uptrendsGuid, delErr)
			}
		}
		if site24x7ID != "" && s.site24x7Client != nil {
			if delErr := s.site24x7Client.DeleteMonitor(site24x7ID); delErr != nil {
				log.Printf("Failed to delete orphaned Site24x7 monitor %s: %v", site24x7ID, delErr)
			}
		}
	} else {
		log.Printf("Successfully created and linked monitors for domain %d (%s)", domainID, fullURL)
	}
}

// GetDomain gets a single domain by ID
func (s *DomainService) GetDomain(domainID, userID int) (*model.Domain, error) {
	var domain model.Domain
	err := s.db.Get(&domain, `
        SELECT id, user_id, name, active, interval, region, last_status, error_code,
               total_time, error_description, monitor_guid, site24x7_monitor_id, 
               is_deep_check, last_check, created_at, updated_at
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
		if *req.Interval != 10 && *req.Interval != 20 && *req.Interval != 30 && *req.Interval != 60 && *req.Interval != 120 {
			return errors.New("interval must be 10, 20, 30, 60, 120 minutes")
		}

		query += fmt.Sprintf(", interval = $%d", paramIndex)
		params = append(params, *req.Interval)
		paramIndex++
	}

	if req.IsDeepCheck != nil {
		query += fmt.Sprintf(", is_deep_check = $%d", paramIndex)
		params = append(params, *req.IsDeepCheck)
		paramIndex++
	}

	// Add region field if provided
	if req.Region != nil && *req.Region != "" {
		// Validate region
		var isValidRegion bool
		err := s.db.Get(&isValidRegion, "SELECT EXISTS(SELECT 1 FROM regions WHERE code = $1 AND is_active = TRUE)", *req.Region)
		if err != nil {
			return fmt.Errorf("error verifying region: %w", err)
		}
		if !isValidRegion {
			return errors.New("invalid region")
		}

		query += fmt.Sprintf(", region = $%d", paramIndex)
		params = append(params, *req.Region)
		paramIndex++

		// If region changed and monitors exist, recreate them
		if domain.Region != *req.Region {
			// Delete existing monitors using helper methods
			if domain.GetMonitorGuid() != "" && s.uptrendsClient != nil {
				if err := s.uptrendsClient.DeleteMonitor(domain.GetMonitorGuid()); err != nil {
					log.Printf("Failed to delete Uptrends monitor for region change: %v", err)
				}
			}
			if domain.GetSite24x7MonitorID() != "" && s.site24x7Client != nil {
				if err := s.site24x7Client.DeleteMonitor(domain.GetSite24x7MonitorID()); err != nil {
					log.Printf("Failed to delete Site24x7 monitor for region change: %v", err)
				}
			}

			// Schedule creation of new monitors
			go s.createMonitorAsync(domainID, domain.Name, *req.Region)
		}
	}

	// Add WHERE clause
	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", paramIndex, paramIndex+1)
	params = append(params, domainID, userID)

	// Execute update if we have fields to update
	if paramIndex > 1 {
		log.Printf("Executing query: %s with params: %v", query, params)
		_, err = s.db.Exec(query, params...)
		if err != nil {
			return err
		}
	}

	// Update monitor statuses if active status changed using helper methods
	if req.Active != nil && req.Region == nil {
		if domain.GetMonitorGuid() != "" && s.uptrendsClient != nil {
			if err := s.uptrendsClient.UpdateMonitorStatus(domain.GetMonitorGuid(), *req.Active); err != nil {
				log.Printf("Failed to update Uptrends monitor status: %v", err)
			}
		}
		if domain.GetSite24x7MonitorID() != "" && s.site24x7Client != nil {
			if err := s.site24x7Client.UpdateMonitorStatus(domain.GetSite24x7MonitorID(), *req.Active); err != nil {
				log.Printf("Failed to update Site24x7 monitor status: %v", err)
			}
		}
	}

	return nil
}

// GetDomains gets all domains for a user
func (s *DomainService) GetDomains(userID int) (model.DomainListResponse, error) {
	var domains []model.Domain

	err := s.db.Select(&domains, `
        SELECT 
            d.id, 
            d.user_id, 
            d.name, 
            COALESCE(d.active, false) AS active,
            d.region,  
            d.last_status, 
            d.error_code, 
            d.error_description, 
            d.last_check, 
            d.monitor_guid,
            d.site24x7_monitor_id,
            d.interval,
            d.total_time,
            COALESCE(d.is_deep_check, false) AS is_deep_check
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

// GetAllActiveDomainsWithMonitors gets all active domains with monitor IDs
func (s *DomainService) GetAllActiveDomainsWithMonitors() ([]model.Domain, error) {
	var domains []model.Domain

	query := `
        SELECT id, user_id, name, active, interval, monitor_guid, site24x7_monitor_id, 
               last_status, error_code, total_time, error_description, last_check, 
               created_at, updated_at, region, COALESCE(is_deep_check, false) AS is_deep_check
        FROM domains 
        WHERE active = true
        AND (monitor_guid IS NOT NULL AND monitor_guid != '') 
        OR (site24x7_monitor_id IS NOT NULL AND site24x7_monitor_id != '')
    `

	err := s.db.Select(&domains, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching active domains with monitors: %w", err)
	}

	return domains, nil
}

// UpdateAllUserDomains updates settings for domains of a user in a specific region
func (s *DomainService) UpdateAllUserDomains(userID int, req model.DomainUpdateRequest) error {
	// Get domain information for this user, filtered by region if provided
	var domains []model.Domain
	var params []interface{}
	var query string

	if req.Region != nil && *req.Region != "" {
		query = "SELECT id, name, monitor_guid, site24x7_monitor_id, region, COALESCE(is_deep_check, false) AS is_deep_check FROM domains WHERE user_id = $1 AND region = $2"
		params = []interface{}{userID, *req.Region}
	} else {
		query = "SELECT id, name, monitor_guid, site24x7_monitor_id, region, COALESCE(is_deep_check, false) AS is_deep_check FROM domains WHERE user_id = $1"
		params = []interface{}{userID}
	}

	err := s.db.Select(&domains, query, params...)
	if err != nil {
		return err
	}

	if len(domains) == 0 {
		if req.Region != nil && *req.Region != "" {
			return fmt.Errorf("no domains found in region %s", *req.Region)
		}
		return nil
	}

	// Build dynamic SQL update query
	updateQuery := "UPDATE domains SET updated_at = NOW()"
	updateParams := []interface{}{}
	paramIndex := 1

	if req.Active != nil {
		updateQuery += fmt.Sprintf(", active = $%d", paramIndex)
		updateParams = append(updateParams, *req.Active)
		paramIndex++
	}

	if req.Interval != nil {
		if *req.Interval != 10 && *req.Interval != 20 && *req.Interval != 30 && *req.Interval != 60 && *req.Interval != 120 {
			return errors.New("interval must be 10, 20, 30, 60 or 120 minutes")
		}

		updateQuery += fmt.Sprintf(", interval = $%d", paramIndex)
		updateParams = append(updateParams, *req.Interval)
		paramIndex++
	}

	if req.IsDeepCheck != nil {
		updateQuery += fmt.Sprintf(", is_deep_check = $%d", paramIndex)
		updateParams = append(updateParams, *req.IsDeepCheck)
		paramIndex++
	}

	// Add WHERE clause with region filter if provided
	if req.Region != nil && *req.Region != "" {
		updateQuery += fmt.Sprintf(" WHERE user_id = $%d AND region = $%d", paramIndex, paramIndex+1)
		updateParams = append(updateParams, userID, *req.Region)
	} else {
		updateQuery += fmt.Sprintf(" WHERE user_id = $%d", paramIndex)
		updateParams = append(updateParams, userID)
	}

	// Execute the update if we have fields to update
	if paramIndex > 1 {
		log.Printf("Executing query: %s with params: %v", updateQuery, updateParams)
		_, err = s.db.Exec(updateQuery, updateParams...)
		if err != nil {
			return err
		}
	}

	// Update monitors in both services if active status is changing using helper methods
	if req.Active != nil {
		for _, domain := range domains {
			if domain.GetMonitorGuid() != "" && s.uptrendsClient != nil {
				if err := s.uptrendsClient.UpdateMonitorStatus(domain.GetMonitorGuid(), *req.Active); err != nil {
					log.Printf("Failed to update Uptrends monitor status for domain %d: %v", domain.ID, err)
				}
			}
			if domain.GetSite24x7MonitorID() != "" && s.site24x7Client != nil {
				if err := s.site24x7Client.UpdateMonitorStatus(domain.GetSite24x7MonitorID(), *req.Active); err != nil {
					log.Printf("Failed to update Site24x7 monitor status for domain %d: %v", domain.ID, err)
				}
			}
		}
	}

	return nil
}

// DeleteDomain deletes a domain
func (s *DomainService) DeleteDomain(userID, domainID int) error {
	// First get the domain to retrieve its monitor IDs
	var domain model.Domain
	err := s.db.Get(&domain, "SELECT * FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Domain not found for user %d: %d", userID, domainID)
			return errors.New("domain not found or not owned by user")
		}
		return err
	}

	// Delete monitors from both services using helper methods
	if domain.GetMonitorGuid() != "" && s.uptrendsClient != nil {
		if err := s.uptrendsClient.DeleteMonitor(domain.GetMonitorGuid()); err != nil {
			log.Printf("Failed to delete Uptrends monitor %s: %v", domain.GetMonitorGuid(), err)
		}
	}

	if domain.GetSite24x7MonitorID() != "" && s.site24x7Client != nil {
		if err := s.site24x7Client.DeleteMonitor(domain.GetSite24x7MonitorID()); err != nil {
			log.Printf("Failed to delete Site24x7 monitor %s: %v", domain.GetSite24x7MonitorID(), err)
		}
	}

	// Delete domain from database
	_, err = s.db.Exec("DELETE FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	if err != nil {
		return err
	}

	return nil
}

// DeleteAllDomains deletes all domains for a user
func (s *DomainService) DeleteAllDomains(userID int) error {
	// Get all domains for the user first (for cleanup)
	domains, err := s.GetDomains(userID)
	if err != nil {
		return fmt.Errorf("failed to get domains for cleanup: %w", err)
	}

	// Start transaction
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete from external monitoring services
	for _, domain := range domains.Domains {
		// Delete from Uptrends if monitor ID exists
		if domain.GetMonitorGuid() != "" {
			if deleteErr := s.uptrendsClient.DeleteMonitor(domain.GetMonitorGuid()); deleteErr != nil {
				log.Printf("Warning: Failed to delete Uptrends monitor %s: %v", domain.GetMonitorGuid(), deleteErr)
				// Continue with deletion even if external service fails
			}
		}

		// Delete from Site24x7 if monitor ID exists
		if domain.GetSite24x7MonitorID() != "" {
			if deleteErr := s.site24x7Client.DeleteMonitor(domain.GetSite24x7MonitorID()); deleteErr != nil {
				log.Printf("Warning: Failed to delete Site24x7 monitor %s: %v", domain.GetSite24x7MonitorID(), deleteErr)
				// Continue with deletion even if external service fails
			}
		}
	}

	// Delete all notification history for user's domains
	_, err = tx.Exec(`
        DELETE FROM notification_history 
        WHERE domain_id IN (
            SELECT id FROM domains WHERE user_id = $1
        )
    `, userID)
	if err != nil {
		return fmt.Errorf("failed to delete notification history: %w", err)
	}

	// Delete all domains for the user
	result, err := tx.Exec(`
        DELETE FROM domains 
        WHERE user_id = $1
    `, userID)
	if err != nil {
		return fmt.Errorf("failed to delete domains: %w", err)
	}

	// Check how many domains were deleted
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully deleted %d domains for user %d", rowsAffected, userID)
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
        SELECT id, user_id, name, active, interval, last_status, last_check, 
               created_at, updated_at, region, COALESCE(is_deep_check, false) AS is_deep_check
        FROM domains
        WHERE active = true
    `

	err := s.db.Select(&domains, query)
	return domains, err
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

	query := `
        SELECT id, user_id, name, active, interval, monitor_guid, site24x7_monitor_id,
               last_status, error_code, total_time, error_description, last_check,
               created_at, updated_at, region, COALESCE(is_deep_check, false) AS is_deep_check
        FROM domains 
        WHERE (monitor_guid IS NOT NULL AND monitor_guid != '') 
           OR (site24x7_monitor_id IS NOT NULL AND site24x7_monitor_id != '')
    `

	err := s.db.Select(&domains, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching domains with monitors: %w", err)
	}

	return domains, nil
}

// UpdateDomainUptrendsGUID updates only the Uptrends monitor GUID for a domain
func (s *DomainService) UpdateDomainUptrendsGUID(domainID int, uptrendsGuid string) (int, error) {
	var uptrendsParam interface{}

	if uptrendsGuid == "" {
		uptrendsParam = nil
	} else {
		uptrendsParam = uptrendsGuid
	}

	result, err := s.db.Exec(`
        UPDATE domains 
        SET monitor_guid = $1, updated_at = NOW() 
        WHERE id = $2
    `, uptrendsParam, domainID)

	if err != nil {
		return 0, fmt.Errorf("failed to update domain Uptrends monitor GUID: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return 0, fmt.Errorf("domain not found or no changes made")
	}

	return int(rowsAffected), nil
}

// UpdateDomainSite24x7ID updates only the Site24x7 monitor ID for a domain
func (s *DomainService) UpdateDomainSite24x7ID(domainID int, site24x7ID string) (int, error) {
	var site24x7Param interface{}

	if site24x7ID == "" {
		site24x7Param = nil
	} else {
		site24x7Param = site24x7ID
	}

	result, err := s.db.Exec(`
        UPDATE domains 
        SET site24x7_monitor_id = $1, updated_at = NOW() 
        WHERE id = $2
    `, site24x7Param, domainID)

	if err != nil {
		return 0, fmt.Errorf("failed to update domain Site24x7 monitor ID: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return 0, fmt.Errorf("domain not found or no changes made")
	}

	return int(rowsAffected), nil
}

// GetDomainsWithoutSite24x7Monitor gets all active domains that don't have a Site24x7 monitor
func (s *DomainService) GetDomainsWithoutSite24x7Monitor() ([]model.Domain, error) {
	var domains []model.Domain

	query := `
        SELECT id, user_id, name, active, interval, monitor_guid, site24x7_monitor_id, 
               last_status, error_code, total_time, error_description, last_check, 
               created_at, updated_at, region
        FROM domains 
        WHERE active = true
        AND (site24x7_monitor_id IS NULL OR site24x7_monitor_id = '')
    `

	err := s.db.Select(&domains, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching domains without Site24x7 monitors: %w", err)
	}

	return domains, nil
}

// DeleteBatchDomains deletes multiple domains by their IDs
func (s *DomainService) DeleteBatchDomains(userID int, domainIDs []int) (*model.DomainBatchDeleteResponse, error) {
	if len(domainIDs) == 0 {
		return nil, errors.New("no domain IDs provided")
	}

	response := &model.DomainBatchDeleteResponse{
		Success:      []model.DomainDeleteResult{},
		Failed:       []model.DomainDeleteResult{},
		DeletedCount: 0,
		TotalCount:   len(domainIDs),
	}

	// Get all domains that belong to the user and exist in the provided IDs
	placeholders := make([]string, len(domainIDs))
	args := []interface{}{userID}
	for i, id := range domainIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf(`
        SELECT id, name, monitor_guid, site24x7_monitor_id
        FROM domains 
        WHERE user_id = $1 AND id IN (%s)
    `, strings.Join(placeholders, ","))

	var domains []struct {
		ID                int     `db:"id"`
		Name              string  `db:"name"`
		MonitorGuid       *string `db:"monitor_guid"`
		Site24x7MonitorID *string `db:"site24x7_monitor_id"`
	}

	err := s.db.Select(&domains, query, args...)
	if err != nil {
		log.Printf("Failed to fetch domains for batch delete: %v", err)
		return nil, fmt.Errorf("failed to fetch domains: %w", err)
	}

	// Create a map of found domains for quick lookup
	foundDomains := make(map[int]struct {
		Name              string
		MonitorGuid       *string
		Site24x7MonitorID *string
	})

	for _, domain := range domains {
		foundDomains[domain.ID] = struct {
			Name              string
			MonitorGuid       *string
			Site24x7MonitorID *string
		}{
			Name:              domain.Name,
			MonitorGuid:       domain.MonitorGuid,
			Site24x7MonitorID: domain.Site24x7MonitorID,
		}
	}

	// Process each requested domain ID
	for _, domainID := range domainIDs {
		domain, exists := foundDomains[domainID]
		if !exists {
			// Domain not found or doesn't belong to user
			response.Failed = append(response.Failed, model.DomainDeleteResult{
				ID:     domainID,
				Name:   "",
				Reason: "Domain not found or access denied",
			})
			continue
		}

		// Delete from external monitoring services first
		var deleteErrors []string

		// Delete from Uptrends if monitor GUID exists
		if domain.MonitorGuid != nil && *domain.MonitorGuid != "" {
			if err := s.uptrendsClient.DeleteMonitor(*domain.MonitorGuid); err != nil {
				deleteErrors = append(deleteErrors, fmt.Sprintf("Uptrends: %v", err))
				log.Printf("Warning: Failed to delete Uptrends monitor %s for domain %s: %v",
					*domain.MonitorGuid, domain.Name, err)
			}
		}

		// Delete from Site24x7 if monitor ID exists
		if domain.Site24x7MonitorID != nil && *domain.Site24x7MonitorID != "" {
			if err := s.site24x7Client.DeleteMonitor(*domain.Site24x7MonitorID); err != nil {
				deleteErrors = append(deleteErrors, fmt.Sprintf("Site24x7: %v", err))
				log.Printf("Warning: Failed to delete Site24x7 monitor %s for domain %s: %v",
					*domain.Site24x7MonitorID, domain.Name, err)
			}
		}

		// Delete from database (continue even if external deletions failed)
		err := s.deleteDomainFromDB(domainID, userID)
		if err != nil {
			reason := fmt.Sprintf("Database deletion failed: %v", err)
			if len(deleteErrors) > 0 {
				reason = fmt.Sprintf("%s (External services: %s)", reason, strings.Join(deleteErrors, ", "))
			}

			response.Failed = append(response.Failed, model.DomainDeleteResult{
				ID:     domainID,
				Name:   domain.Name,
				Reason: reason,
			})
			continue
		}

		// Success
		result := model.DomainDeleteResult{
			ID:   domainID,
			Name: domain.Name,
		}

		// Add warning about external service failures if any
		if len(deleteErrors) > 0 {
			result.Reason = fmt.Sprintf("Warning: %s", strings.Join(deleteErrors, ", "))
		}

		response.Success = append(response.Success, result)
		response.DeletedCount++
	}

	log.Printf("Batch delete completed for user %d: %d/%d domains deleted successfully",
		userID, response.DeletedCount, response.TotalCount)

	return response, nil
}

// deleteDomainFromDB is a helper method to delete a domain from the database
func (s *DomainService) deleteDomainFromDB(domainID, userID int) error {
	// Start transaction for cleanup
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete notification history first
	_, err = tx.Exec(`
        DELETE FROM notification_history 
        WHERE domain_id = $1
    `, domainID)
	if err != nil {
		return fmt.Errorf("failed to delete notification history: %w", err)
	}

	// Delete the domain
	result, err := tx.Exec(`
        DELETE FROM domains 
        WHERE id = $1 AND user_id = $2
    `, domainID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}

	// Check if domain was actually deleted
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("domain not found or access denied")
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
