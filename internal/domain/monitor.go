package domain

import "domain-detection-go/pkg/model"

// MonitorClient defines the interface for domain monitoring operations
type MonitorClient interface {
	// CreateMonitor creates a monitor for a domain and returns the monitor GUID
	CreateMonitor(fullURL string, name string, regions []string) (string, error)

	// UpdateMonitorStatus updates the active status of a monitor
	UpdateMonitorStatus(monitorGuid string, isActive bool) error

	// DeleteMonitor deletes a monitor in the monitoring service
	DeleteMonitor(monitorGuid string) error

	GetLatestMonitorCheck(monitorGUID string) (*model.DomainCheckResult, error)
}
