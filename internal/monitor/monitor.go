package monitor

import (
	"log"
	"time"

	"domain-detection-go/internal/domain"
	"domain-detection-go/pkg/model"
)

// MonitorService manages domain monitoring operations
type MonitorService struct {
	uptrendsClient *UptrendsClient
	domainService  *domain.DomainService
	regions        []string
	// mu             sync.Mutex
}

// NewMonitorService creates a new monitor service
func NewMonitorService(uptrendsClient *UptrendsClient, domainService *domain.DomainService) *MonitorService {
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
		uptrendsClient: uptrendsClient,
		domainService:  domainService,
		regions:        regions,
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

		// Use try-catch pattern with defer/recover to prevent panics
		func(d model.Domain) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic while checking domain %s: %v", d.Name, r)
				}
			}()

			result, err := s.uptrendsClient.GetLatestMonitorCheck(d.MonitorGuid)
			if err != nil {
				log.Printf("Error checking domain %s: %v", d.Name, err)
				return
			}

			// Only proceed if result is not nil
			if result == nil {
				log.Printf("No result returned for domain %s", d.Name)
				return
			}

			// Fill in domain name
			result.Domain = d.Name

			// Update domain status in database
			err = s.domainService.UpdateDomainStatus(d.ID, result.StatusCode, result.ErrorCode, result.TotalTime, result.ErrorDescription)
			if err != nil {
				log.Printf("Error updating status for domain %s: %v", d.Name, err)
			}
		}(domain)
	}
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

// RunScheduledChecks performs periodic checks on all active domains
func (s *MonitorService) RunScheduledChecks() {
	log.Printf("RunScheduledChecks")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAllActiveDomains()
		}
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

// Close cleans up resources
func (s *MonitorService) Close() {
	s.uptrendsClient.Close()
}
