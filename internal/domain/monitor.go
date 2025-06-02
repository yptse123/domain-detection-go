package domain

import "domain-detection-go/pkg/model"

// MonitorClient defines the interface for domain monitoring operations
type MonitorClient interface {
	CreateMonitor(fullURL string, name string, regions []string) (string, error)
	UpdateMonitorStatus(monitorID string, isActive bool) error
	DeleteMonitor(monitorID string) error
	GetLatestMonitorCheck(monitorID string, region string) (*model.DomainCheckResult, error)
	Close()
}
