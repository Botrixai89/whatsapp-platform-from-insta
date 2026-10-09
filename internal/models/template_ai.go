package models

import "github.com/google/uuid"

// TemplateAIPrompt records a prompt used with the AI template generator so
// users can reuse previous prompts.
type TemplateAIPrompt struct {
	BaseModel
	OrganizationID uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	UserID         uuid.UUID `gorm:"type:uuid;index;not null" json:"user_id"`
	Prompt         string    `gorm:"type:text;not null" json:"prompt"`
	Category       string    `gorm:"size:30" json:"category"`
	Language       string    `gorm:"size:10" json:"language"`
	Style          string    `gorm:"size:20" json:"style"`
	OptimizeFor    string    `gorm:"size:20" json:"optimize_for"`
	HeaderType     string    `gorm:"size:20" json:"header_type"`
}

func (TemplateAIPrompt) TableName() string {
	return "template_ai_prompts"
}
