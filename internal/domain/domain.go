package domain

import (
	"database/sql"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"

	"domain-detection-go/pkg/model"

	"fmt"

	"github.com/jmoiron/sqlx"
)

// DomainService handles domain operations
type DomainService struct {
	db *sqlx.DB
}

// NewDomainService creates a new domain service
func NewDomainService(db *sqlx.DB) *DomainService {
	return &DomainService{
		db: db,
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

// ValidateDomainName checks if a domain name is valid
func (s *DomainService) ValidateDomainName(domain string) bool {
	// Basic validation
	if len(domain) < 3 || len(domain) > 253 {
		return false
	}

	// Remove trailing dot if present
	domain = strings.TrimSuffix(domain, ".")

	// Check domain pattern
	pattern := `^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	match, _ := regexp.MatchString(pattern, domain)
	if !match {
		return false
	}

	// Check if domain exists (optional - can be expensive)
	_, err := net.LookupHost(domain)
	return err == nil
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

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = DEFAULT_INTERVAL
	} else if interval != 10 && interval != 20 && interval != 30 {
		return 0, errors.New("interval must be 10, 20, or 30 minutes")
	}

	// Insert the domain
	var domainID int
	err = s.db.QueryRow(`
        INSERT INTO domains (user_id, name, interval, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $4)
        RETURNING id
    `, userID, req.Name, interval, time.Now()).Scan(&domainID)

	if err != nil {
		return 0, err
	}

	return domainID, nil
}

// GetDomains gets all domains for a user
func (s *DomainService) GetDomains(userID int) (model.DomainListResponse, error) {
	var domains []model.Domain
	err := s.db.Select(&domains, `
        SELECT id, user_id, name, active, interval, last_status, last_check, created_at, updated_at
        FROM domains
        WHERE user_id = $1
        ORDER BY created_at DESC
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
        SELECT id, user_id, name, active, interval, last_status, last_check, created_at, updated_at
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
	domain, err := s.GetDomain(domainID, userID)
	if err != nil {
		return err
	}

	// Build update query dynamically based on provided fields
	query := "UPDATE domains SET updated_at = NOW()"
	args := []interface{}{}
	argPosition := 1

	if req.Active != nil {
		query += ", active = $" + fmt.Sprint(argPosition+'0')
		args = append(args, *req.Active)
		argPosition++
	}

	if req.Interval != nil {
		// Validate interval
		if *req.Interval != 10 && *req.Interval != 20 && *req.Interval != 30 {
			return errors.New("interval must be 10, 20, or 30 minutes")
		}

		query += ", interval = $" + fmt.Sprint(argPosition+'0')
		args = append(args, *req.Interval)
		argPosition++
	}

	// Add WHERE clause
	query += " WHERE id = $" + fmt.Sprint(argPosition+'0') + " AND user_id = $" + fmt.Sprint(argPosition+1+'0')
	args = append(args, domain.ID, userID)

	// Execute update
	_, err = s.db.Exec(query, args...)
	return err
}

// DeleteDomain deletes a domain
func (s *DomainService) DeleteDomain(domainID, userID int) error {
	// Check if domain exists and belongs to user
	_, err := s.GetDomain(domainID, userID)
	if err != nil {
		return err
	}

	// Delete the domain
	_, err = s.db.Exec("DELETE FROM domains WHERE id = $1 AND user_id = $2", domainID, userID)
	return err
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

// UpdateDomainStatus updates the status of a domain
func (s *DomainService) UpdateDomainStatus(domainID int, statusCode int) error {
	_, err := s.db.Exec(`
        UPDATE domains
        SET last_status = $1, last_check = NOW(), updated_at = NOW()
        WHERE id = $2
    `, statusCode, domainID)

	return err
}
