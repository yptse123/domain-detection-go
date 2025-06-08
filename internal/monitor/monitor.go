package monitor

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"domain-detection-go/internal/domain"
	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/model"
)

// MonitorService manages domain monitoring operations
type MonitorService struct {
	uptrendsClient  *UptrendsClient
	site24x7Client  *Site24x7Client // Add this field
	domainService   *domain.DomainService
	telegramService *notification.TelegramService
	regions         []string
}

// NewMonitorService creates a new monitor service
func NewMonitorService(uptrendsClient *UptrendsClient, site24x7Client *Site24x7Client, domainService *domain.DomainService, telegramService *notification.TelegramService) *MonitorService {
	// Default regions to check
	regions := []string{
		"CN", // China
		"JP", // Japan
		"KR", // Korea
		"TH", // Thailand
		"IN", // India
		"ID", // Indonesia
		"VN", // Vietnam
	}

	return &MonitorService{
		uptrendsClient:  uptrendsClient,
		site24x7Client:  site24x7Client,
		domainService:   domainService,
		telegramService: telegramService,
		regions:         regions,
	}
}

// ensureUptrendsMonitor creates an Uptrends monitor if the domain doesn't have one
func (s *MonitorService) ensureUptrendsMonitor(domain model.Domain) string {
	// If domain already has an Uptrends monitor GUID, return it
	if domain.GetMonitorGuid() != "" {
		return domain.GetMonitorGuid()
	}

	// If Uptrends client is not available, return empty
	if s.uptrendsClient == nil {
		log.Printf("Uptrends client not available for domain %s", domain.Name)
		return ""
	}

	log.Printf("Creating missing Uptrends monitor for domain %s in region %s", domain.Name, domain.Region)

	// Extract domain name for the monitor name
	parsedURL, err := url.Parse(domain.Name)
	if err != nil {
		log.Printf("Failed to parse URL for monitor creation: %v", err)
		return ""
	}

	displayName := parsedURL.Hostname()
	if displayName == "" {
		displayName = domain.Name
	}
	monitorName := fmt.Sprintf("Domain Check - %s", displayName)

	// Create monitor with the domain's region
	regions := []string{domain.Region}

	// Add fallback regions based on primary region
	switch domain.Region {
	case "TH", "ID", "KR":
		regions = append(regions, "VN") // Add Vietnam
	case "VN":
		regions = append(regions, "TH") // Add Thailand
	}

	uptrendsGuid, err := s.uptrendsClient.CreateMonitor(domain.Name, monitorName, regions)
	if err != nil {
		log.Printf("Failed to create Uptrends monitor for domain %s: %v", domain.Name, err)
		return ""
	}

	// Update the domain in the database with the new monitor GUID
	_, dbErr := s.domainService.UpdateDomainUptrendsGUID(domain.ID, uptrendsGuid)
	if dbErr != nil {
		log.Printf("Failed to update domain %d with Uptrends monitor GUID %s: %v", domain.ID, uptrendsGuid, dbErr)

		// Clean up created monitor if database update failed
		if delErr := s.uptrendsClient.DeleteMonitor(uptrendsGuid); delErr != nil {
			log.Printf("Failed to delete orphaned Uptrends monitor %s: %v", uptrendsGuid, delErr)
		}
		return ""
	}

	log.Printf("Successfully created and linked Uptrends monitor %s for domain %s", uptrendsGuid, domain.Name)
	return uptrendsGuid
}

// ensureSite24x7Monitor creates a Site24x7 monitor if the domain doesn't have one
func (s *MonitorService) ensureSite24x7Monitor(domain model.Domain) string {
	// If domain already has a Site24x7 monitor ID, return it
	if domain.GetSite24x7MonitorID() != "" {
		return domain.GetSite24x7MonitorID()
	}

	// If Site24x7 client is not available, return empty
	if s.site24x7Client == nil {
		log.Printf("Site24x7 client not available for domain %s", domain.Name)
		return ""
	}

	log.Printf("Creating missing Site24x7 monitor for domain %s in region %s", domain.Name, domain.Region)

	// Extract domain name for the monitor name
	parsedURL, err := url.Parse(domain.Name)
	if err != nil {
		log.Printf("Failed to parse URL for monitor creation: %v", err)
		return ""
	}

	displayName := parsedURL.Hostname()
	if displayName == "" {
		displayName = domain.Name
	}
	monitorName := fmt.Sprintf("Domain Check - %s", displayName)

	// Create monitor with the domain's region
	regions := []string{domain.Region}
	site24x7ID, err := s.site24x7Client.CreateMonitor(domain.Name, monitorName, regions)
	if err != nil {
		log.Printf("Failed to create Site24x7 monitor for domain %s: %v", domain.Name, err)
		return ""
	}

	// Update the domain in the database with the new monitor ID
	_, dbErr := s.domainService.UpdateDomainSite24x7ID(domain.ID, site24x7ID)
	if dbErr != nil {
		log.Printf("Failed to update domain %d with Site24x7 monitor ID %s: %v", domain.ID, site24x7ID, dbErr)

		// Clean up created monitor if database update failed
		if delErr := s.site24x7Client.DeleteMonitor(site24x7ID); delErr != nil {
			log.Printf("Failed to delete orphaned Site24x7 monitor %s: %v", site24x7ID, delErr)
		}
		return ""
	}

	log.Printf("Successfully created and linked Site24x7 monitor %s for domain %s", site24x7ID, domain.Name)
	return site24x7ID
}

// checkAllActiveDomains checks domains that are due for checking based on their interval
func (s *MonitorService) checkAllActiveDomains() {
	// Get all active domains with monitor GUIDs
	domains, err := s.domainService.GetAllActiveDomainsWithMonitors()
	if err != nil {
		log.Printf("Error getting active domains: %v", err)
		return
	}

	now := time.Now()

	for _, domain := range domains {
		// Use helper methods to get string values
		uptrendsGuid := domain.GetMonitorGuid()
		site24x7ID := domain.GetSite24x7MonitorID()

		// Skip domains without any monitor IDs
		if uptrendsGuid == "" && site24x7ID == "" {
			continue
		}

		// Check if this domain is due for checking based on its interval
		if !isDomainDueForCheck(domain, now) {
			continue
		}

		log.Printf("Checking domain %s (interval: %d minutes)", domain.Name, domain.Interval)

		func(d model.Domain) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic while checking domain %s: %v", d.Name, r)
				}
			}()

			var uptrendsResult, site24x7Result *model.DomainCheckResult
			var uptrendsErr, site24x7Err error

			// Ensure Uptrends monitor exists and get its GUID
			currentUptrendsGuid := d.GetMonitorGuid()
			if currentUptrendsGuid == "" {
				// Create Uptrends monitor if it doesn't exist
				currentUptrendsGuid = s.ensureUptrendsMonitor(d)
			}

			// Check with Uptrends API if available
			if currentUptrendsGuid != "" {
				uptrendsResult, uptrendsErr = s.uptrendsClient.GetLatestMonitorCheck(currentUptrendsGuid, d.Region)
				if uptrendsErr != nil {
					log.Printf("Error checking domain %s with Uptrends: %v", d.Name, uptrendsErr)
				}
			}

			// Ensure Site24x7 monitor exists and get its ID
			currentSite24x7ID := d.GetSite24x7MonitorID()
			if currentSite24x7ID == "" {
				// Create Site24x7 monitor if it doesn't exist
				currentSite24x7ID = s.ensureSite24x7Monitor(d)
			}

			// Check with Site24x7 API if available
			if currentSite24x7ID != "" {
				site24x7Result, site24x7Err = s.site24x7Client.GetLatestMonitorCheck(currentSite24x7ID, d.Region)
				if site24x7Err != nil {
					log.Printf("Error checking domain %s with Site24x7: %v", d.Name, site24x7Err)
				}
			}

			// Skip if both providers failed
			if uptrendsErr != nil && site24x7Err != nil {
				log.Printf("Both monitoring providers failed for domain %s, skipping notification", d.Name)
				return
			}

			// Determine final result and availability
			var finalResult *model.DomainCheckResult
			var isAvailable bool

			if uptrendsResult != nil && site24x7Result != nil {
				// Both providers available - domain is available only if BOTH report it as available
				isAvailable = uptrendsResult.Available && site24x7Result.Available

				// Use Uptrends result as primary, but adjust availability
				finalResult = uptrendsResult
				finalResult.Available = isAvailable

				log.Printf("Domain %s check results - Uptrends: available=%v, status=%d | Site24x7: available=%v, status=%d | Final: available=%v",
					d.Name, uptrendsResult.Available, uptrendsResult.StatusCode,
					site24x7Result.Available, site24x7Result.StatusCode, isAvailable)
			} else if uptrendsResult != nil {
				// Only Uptrends available
				finalResult = uptrendsResult
				isAvailable = uptrendsResult.Available
				log.Printf("Domain %s check result (Uptrends only): available=%v, status=%d",
					d.Name, isAvailable, uptrendsResult.StatusCode)
			} else {
				// Only Site24x7 available
				finalResult = site24x7Result
				isAvailable = site24x7Result.Available
				log.Printf("Domain %s check result (Site24x7 only): available=%v, status=%d",
					d.Name, isAvailable, site24x7Result.StatusCode)
			}

			finalResult.Domain = d.Name
			finalResult.Available = isAvailable

			// Get previous status to detect changes
			prevAvailable := d.Available()

			// Update domain status in database
			err := s.domainService.UpdateDomainStatus(d.ID, finalResult.StatusCode,
				finalResult.ErrorCode, finalResult.TotalTime,
				finalResult.ErrorDescription)
			if err != nil {
				log.Printf("Error updating status for domain %s: %v", d.Name, err)
			}

			// Get updated domain with new status
			updatedDomain, _ := s.domainService.GetDomain(d.ID, d.UserID)
			if updatedDomain != nil {
				// Get current availability status
				currentAvailable := updatedDomain.Available()

				// Check if status changed (available → unavailable or vice versa)
				statusChanged := prevAvailable != currentAvailable

				// Only log status changes when they actually occur
				if statusChanged {
					log.Printf("Domain %s status changed: %v -> %v", d.Name, prevAvailable, currentAvailable)
				} else {
					log.Printf("Domain %s status unchanged: %v", d.Name, currentAvailable)
				}

				// Send notification if domain is down or status changed
				if !currentAvailable || statusChanged {
					if statusChanged {
						log.Printf("Domain %s status changed. Sending notification.", d.Name)
					} else if !currentAvailable {
						log.Printf("Domain %s is still down. Sending notification.", d.Name)
					}

					if s.telegramService != nil {
						if err := s.telegramService.SendDomainStatusNotification(*updatedDomain, statusChanged); err != nil {
							log.Printf("Failed to send Telegram notification for domain %s: %v", d.Name, err)
						}
					}
				}
			}
		}(domain)
	}
}

// RunScheduledChecks performs periodic checks on all active domains
func (s *MonitorService) RunScheduledChecks() {
	log.Printf("RunScheduledChecks")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAllActiveDomains()
	}
}

// SyncMonitorStatus ensures that monitor statuses in Uptrends match the database
func (s *MonitorService) SyncMonitorStatus() {
	log.Printf("Starting monitor status sync")

	// Get all domains with monitor GUIDs
	domains, err := s.domainService.GetAllDomainsWithMonitors()
	if err != nil {
		log.Printf("Error fetching domains with monitors: %v", err)
		return
	}

	for _, domain := range domains {
		// Update Uptrends monitor status if available
		if domain.GetMonitorGuid() != "" && s.uptrendsClient != nil {
			err := s.uptrendsClient.UpdateMonitorStatus(domain.GetMonitorGuid(), domain.Active)
			if err != nil {
				log.Printf("Error syncing Uptrends monitor status for domain %d: %v", domain.ID, err)
			}
		}

		// Update Site24x7 monitor status if available
		if domain.GetSite24x7MonitorID() != "" && s.site24x7Client != nil {
			err := s.site24x7Client.UpdateMonitorStatus(domain.GetSite24x7MonitorID(), domain.Active)
			if err != nil {
				log.Printf("Error syncing Site24x7 monitor status for domain %d: %v", domain.ID, err)
			}
		}
	}

	log.Printf("Completed monitor status sync")
}

// checkDomainDirect performs a direct HTTP check from the application
// func (s *MonitorService) checkDomainDirect(fullURL string) (*model.DomainCheckResult, error) {
// 	start := time.Now()

// 	// Parse the URL
// 	parsedURL, err := url.Parse(fullURL)
// 	if err != nil {
// 		return nil, fmt.Errorf("invalid URL format: %w", err)
// 	}

// 	// If no scheme provided, default to HTTPS
// 	if parsedURL.Scheme == "" {
// 		fullURL = fmt.Sprintf("https://%s", fullURL)
// 	}

// 	// Create HTTP client with timeout
// 	client := &http.Client{
// 		Timeout: 10 * time.Second,
// 		CheckRedirect: func(req *http.Request, via []*http.Request) error {
// 			// Allow up to 10 redirects
// 			if len(via) >= 10 {
// 				return errors.New("too many redirects")
// 			}
// 			return nil
// 		},
// 	}

// 	// Create request
// 	req, err := http.NewRequest("GET", fullURL, nil)
// 	if err != nil {
// 		return nil, fmt.Errorf("error creating request: %w", err)
// 	}

// 	// Add user agent
// 	req.Header.Set("User-Agent", "DomainMonitor/1.0")

// 	// Perform request
// 	resp, err := client.Do(req)

// 	// Calculate response time regardless of error
// 	responseTime := int(time.Since(start).Milliseconds())

// 	// Log any errors from the HTTP request
// 	if err != nil {
// 		log.Printf("Direct check error for domain %s: %v", fullURL, err)
// 	}

// 	// Check for connection errors
// 	if err != nil {
// 		// Return result with error info
// 		return &model.DomainCheckResult{
// 			Domain:           fullURL,
// 			StatusCode:       0,
// 			ResponseTime:     responseTime,
// 			Available:        false,
// 			TotalTime:        responseTime,
// 			ErrorCode:        -1, // Custom error code for connection issues
// 			ErrorDescription: fmt.Sprintf("Connection error: %v", err),
// 			CheckedAt:        time.Now(),
// 		}, nil
// 	}
// 	defer resp.Body.Close()

// 	// Read a small portion of the body to ensure connection is working
// 	// but don't download everything
// 	buffer := make([]byte, 1024)
// 	_, err = resp.Body.Read(buffer)

// 	// Log response details
// 	log.Printf("Direct check response for %s: status=%d (%s), time=%dms",
// 		fullURL, resp.StatusCode, resp.Status, responseTime)

// 	return &model.DomainCheckResult{
// 		Domain:           fullURL,
// 		StatusCode:       resp.StatusCode,
// 		ResponseTime:     responseTime,
// 		Available:        resp.StatusCode >= 200 && resp.StatusCode < 400,
// 		TotalTime:        responseTime,
// 		ErrorCode:        0,
// 		ErrorDescription: resp.Status,
// 		CheckedAt:        time.Now(),
// 	}, nil
// }

// Close cleans up resources
func (s *MonitorService) Close() {
	s.uptrendsClient.Close()
	s.site24x7Client.Close() // Add this
}

// isDomainDueForCheck determines if a domain is due for a check based on its interval
func isDomainDueForCheck(domain model.Domain, now time.Time) bool {
	// If domain has never been checked, it's due for a check
	if domain.LastCheck.IsZero() {
		return true
	}

	// Calculate next check time based on interval (in minutes)
	nextCheckTime := domain.LastCheck.Add(time.Duration(domain.Interval) * time.Minute)

	// If the next check time has passed, the domain is due for a check
	return now.After(nextCheckTime) || now.Equal(nextCheckTime)
}
