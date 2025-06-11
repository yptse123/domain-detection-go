package service

import (
	"domain-detection-go/pkg/model"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type TelegramPromptService struct {
	db *sqlx.DB
}

func NewTelegramPromptService(db *sqlx.DB) *TelegramPromptService {
	return &TelegramPromptService{db: db}
}

// GetPrompts with pagination
func (s *TelegramPromptService) GetPrompts(page, perPage int, search, sortBy, sortOrder string) (*model.TelegramPromptResponse, error) {
	offset := (page - 1) * perPage

	// Build where clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if search != "" {
		whereClause += fmt.Sprintf(" AND (key ILIKE $%d OR message ILIKE $%d OR description ILIKE $%d)", argIndex, argIndex, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	// Validate sort fields
	validSorts := map[string]bool{"key": true, "language": true, "updated_at": true, "created_at": true}
	if !validSorts[sortBy] {
		sortBy = "updated_at"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM telegram_prompts %s", whereClause)
	var total int
	err := s.db.Get(&total, countQuery, args...)
	if err != nil {
		return nil, err
	}

	// Get prompts
	query := fmt.Sprintf(`
        SELECT id, key, language, message, description, created_at, updated_at
        FROM telegram_prompts %s
        ORDER BY %s %s
        LIMIT $%d OFFSET $%d
    `, whereClause, sortBy, sortOrder, argIndex, argIndex+1)

	args = append(args, perPage, offset)

	var prompts []model.TelegramPrompt
	err = s.db.Select(&prompts, query, args...)
	if err != nil {
		return nil, err
	}

	totalPages := (total + perPage - 1) / perPage

	return &model.TelegramPromptResponse{
		Prompts:    prompts,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// GetTranslation gets a specific message template
func (s *TelegramPromptService) GetTranslation(key, language string) (string, error) {
	var message string
	err := s.db.Get(&message, `
        SELECT message FROM telegram_prompts 
        WHERE key = $1 AND language = $2
    `, key, language)
	if err != nil {
		// Fallback to English if specific language not found
		err = s.db.Get(&message, `
            SELECT message FROM telegram_prompts 
            WHERE key = $1 AND language = 'en'
        `, key)
	}
	return message, err
}

// CreatePrompt creates a new prompt
func (s *TelegramPromptService) CreatePrompt(req model.TelegramPromptRequest) (*model.TelegramPrompt, error) {
	var prompt model.TelegramPrompt
	err := s.db.QueryRow(`
        INSERT INTO telegram_prompts (key, language, message, description, created_at, updated_at)
        VALUES ($1, $2, $3, $4, NOW(), NOW())
        RETURNING id, key, language, message, description, created_at, updated_at
    `, req.Key, req.Language, req.Message, req.Description).Scan(
		&prompt.ID, &prompt.Key, &prompt.Language, &prompt.Message,
		&prompt.Description, &prompt.CreatedAt, &prompt.UpdatedAt)
	return &prompt, err
}

// UpdatePrompt updates an existing prompt
func (s *TelegramPromptService) UpdatePrompt(id int, req model.TelegramPromptRequest) (*model.TelegramPrompt, error) {
	var prompt model.TelegramPrompt
	err := s.db.QueryRow(`
        UPDATE telegram_prompts 
        SET message = $1, description = $2, updated_at = NOW()
        WHERE id = $3
        RETURNING id, key, language, message, description, created_at, updated_at
    `, req.Message, req.Description, id).Scan(
		&prompt.ID, &prompt.Key, &prompt.Language, &prompt.Message,
		&prompt.Description, &prompt.CreatedAt, &prompt.UpdatedAt)
	return &prompt, err
}

// DeletePrompt deletes a prompt
func (s *TelegramPromptService) DeletePrompt(id int) error {
	_, err := s.db.Exec("DELETE FROM telegram_prompts WHERE id = $1", id)
	return err
}

// GetPromptByID gets a prompt by ID
func (s *TelegramPromptService) GetPromptByID(id int) (*model.TelegramPrompt, error) {
	var prompt model.TelegramPrompt
	err := s.db.Get(&prompt, `
        SELECT id, key, language, message, description, created_at, updated_at
        FROM telegram_prompts WHERE id = $1
    `, id)
	return &prompt, err
}

// GetAllPromptsByLanguage gets all prompts for a specific language
func (s *TelegramPromptService) GetAllPromptsByLanguage(language string) ([]model.TelegramPrompt, error) {
	var prompts []model.TelegramPrompt
	err := s.db.Select(&prompts, `
        SELECT id, key, language, message, description, created_at, updated_at
        FROM telegram_prompts 
        WHERE language = $1
        ORDER BY key
    `, language)
	return prompts, err
}
