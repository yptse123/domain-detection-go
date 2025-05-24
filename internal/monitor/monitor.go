package monitor

import (
	"fmt"
	"log"
	"time"

	"domain-detection-go/internal/domain"
	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/model"
)

// MonitorService manages domain monitoring operations
type MonitorService struct {
	uptrendsClient  *UptrendsClient
	domainService   *domain.DomainService
	telegramService *notification.TelegramService
	regions         []string
	// mu             sync.Mutex
}

// NewMonitorService creates a new monitor service
func NewMonitorService(uptrendsClient *UptrendsClient, domainService *domain.DomainService, telegramService *notification.TelegramService) *MonitorService {
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
		domainService:   domainService,
		telegramService: telegramService,
		regions:         regions,
	}
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
		// Skip domains without monitor GUID
		if domain.MonitorGuid == "" {
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

			// Check with Uptrends API only
			uptrendsResult, uptrendsErr := s.uptrendsClient.GetLatestMonitorCheck(d.MonitorGuid, d.Region)
			if uptrendsErr != nil {
				log.Printf("Error checking domain %s with Uptrends: %v", d.Name, uptrendsErr)

				// Create error result if Uptrends check failed
				uptrendsResult = &model.DomainCheckResult{
					Domain:           d.Name,
					StatusCode:       503, // Service Unavailable
					Available:        false,
					ErrorCode:        -999,
					ErrorDescription: fmt.Sprintf("Uptrends monitoring failed: %v", uptrendsErr),
					CheckedAt:        time.Now(),
				}
			}

			// Use Uptrends result as final result
			finalResult := uptrendsResult
			finalResult.Domain = d.Name

			// Get previous status to detect changes
			prevAvailable := d.Available()

			log.Printf("Domain %s check result: available=%v, status=%d, time=%dms",
				d.Name, finalResult.Available, finalResult.StatusCode, finalResult.TotalTime)

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
		// Skip if no monitor GUID
		if domain.MonitorGuid == "" {
			continue
		}

		// Update monitor status to match database
		err := s.uptrendsClient.UpdateMonitorStatus(domain.MonitorGuid, domain.Active)
		if err != nil {
			log.Printf("Error syncing monitor status for domain %d: %v", domain.ID, err)
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
