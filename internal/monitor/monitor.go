package monitor

import (
	"log"
	"time"

	"domain-detection-go/internal/domain"
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

// checkAllActiveDomains checks all active domains and updates their status
func (s *MonitorService) checkAllActiveDomains() {
	// Get all active domains with monitor GUIDs from the domain service
	domains, err := s.domainService.GetAllDomainsWithMonitors()
	if err != nil {
		log.Printf("Error getting active domains: %v", err)
		return
	}

	for _, domain := range domains {
		// Skip domains without monitor GUID
		if domain.MonitorGuid == "" {
			continue
		}

		// Use try-catch pattern with defer/recover to prevent panics
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic while checking domain %s: %v", domain.Name, r)
				}
			}()

			result, err := s.uptrendsClient.GetLatestMonitorCheck(domain.MonitorGuid)
			if err != nil {
				log.Printf("Error checking domain %s: %v", domain.Name, err)
				return
			}

			// Only proceed if result is not nil
			if result == nil {
				log.Printf("No result returned for domain %s", domain.Name)
				return
			}

			// Fill in domain name
			result.Domain = domain.Name

			// Update domain status in database
			err = s.domainService.UpdateDomainStatus(domain.ID, result.StatusCode, result.ErrorCode, result.TotalTime, result.ErrorDescription)
			if err != nil {
				log.Printf("Error updating status for domain %s: %v", domain.Name, err)
			}
		}()
	}
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
