package models

import (
	"time"

	"github.com/google/uuid"
)

// CSATResponse is one customer-satisfaction survey sent when a chat is closed.
type CSATResponse struct {
	BaseModel
	OrganizationID  uuid.UUID  `gorm:"type:uuid;index;not null" json:"organization_id"`
	ContactID       uuid.UUID  `gorm:"type:uuid;index;not null" json:"contact_id"`
	AgentID         *uuid.UUID `gorm:"type:uuid;index" json:"agent_id,omitempty"` // agent the chat was assigned to when closed
	WhatsAppAccount string     `gorm:"size:100" json:"whatsapp_account"`
	Rating          int        `gorm:"default:0" json:"rating"` // 1-5; 0 until the customer answers
	SentAt          time.Time  `gorm:"index;not null" json:"sent_at"`
	RespondedAt     *time.Time `json:"responded_at,omitempty"`
}

func (CSATResponse) TableName() string { return "csat_responses" }

// AdReferral records a customer arriving from a Click-to-WhatsApp ad or post.
type AdReferral struct {
	BaseModel
	OrganizationID  uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	ContactID       uuid.UUID `gorm:"type:uuid;index;not null" json:"contact_id"`
	WhatsAppAccount string    `gorm:"size:100" json:"whatsapp_account"`
	SourceID        string    `gorm:"size:100;index" json:"source_id"` // Meta ad or post ID
	SourceType      string    `gorm:"size:20" json:"source_type"`      // "ad" or "post"
	SourceURL       string    `gorm:"type:text" json:"source_url"`
	Headline        string    `gorm:"type:text" json:"headline"`
	Body            string    `gorm:"type:text" json:"body"`
	CtwaClid        string    `gorm:"size:255" json:"ctwa_clid"` // click ID for Meta conversion reporting
	MessageWAMID    string    `gorm:"size:255" json:"message_wamid"`
}

func (AdReferral) TableName() string { return "ad_referrals" }

// TrackedLink is a short link (/l/{code}) that counts clicks before redirecting.
// Campaign links get one per recipient so clicks can be tied to a contact.
type TrackedLink struct {
	BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;not null" json:"organization_id"`
	Code           string     `gorm:"size:16;uniqueIndex;not null" json:"code"`
	TargetURL      string     `gorm:"type:text;not null" json:"target_url"`
	Title          string     `gorm:"size:255" json:"title"`
	CampaignID     *uuid.UUID `gorm:"type:uuid;index" json:"campaign_id,omitempty"`
	RecipientID    *uuid.UUID `gorm:"type:uuid;index" json:"recipient_id,omitempty"`
	ContactPhone   string     `gorm:"size:50" json:"contact_phone,omitempty"`
	Clicks         int        `gorm:"default:0" json:"clicks"`
	FirstClickedAt *time.Time `json:"first_clicked_at,omitempty"`
	LastClickedAt  *time.Time `json:"last_clicked_at,omitempty"`
}

func (TrackedLink) TableName() string { return "tracked_links" }

// Sequence is a drip campaign: templates sent one after another with delays.
type Sequence struct {
	BaseModel
	OrganizationID  uuid.UUID      `gorm:"type:uuid;index;not null" json:"organization_id"`
	Name            string         `gorm:"size:255;not null" json:"name"`
	WhatsAppAccount string         `gorm:"size:100;not null" json:"whatsapp_account"`
	IsActive        bool           `gorm:"default:true" json:"is_active"`
	StopOnReply     bool           `gorm:"default:true" json:"stop_on_reply"`
	TriggerTag      string         `gorm:"size:100;index" json:"trigger_tag"` // enrol automatically when this tag is added
	Steps           []SequenceStep `gorm:"foreignKey:SequenceID" json:"steps,omitempty"`
}

func (Sequence) TableName() string { return "sequences" }

// SequenceStep sends one approved template, DelayMinutes after the previous step (or enrolment).
type SequenceStep struct {
	BaseModel
	SequenceID   uuid.UUID `gorm:"type:uuid;index;not null" json:"sequence_id"`
	Position     int       `gorm:"not null" json:"position"`
	DelayMinutes int       `gorm:"not null" json:"delay_minutes"`
	TemplateID   uuid.UUID `gorm:"type:uuid;not null" json:"template_id"`
	// Values for the template's variables; "{name}" and "{phone}" are filled per contact.
	TemplateParams JSONB     `gorm:"type:jsonb;default:'{}'" json:"template_params"`
	Template       *Template `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
}

func (SequenceStep) TableName() string { return "sequence_steps" }

// Sequence enrolment states
const (
	EnrollmentActive    = "active"
	EnrollmentCompleted = "completed"
	EnrollmentStopped   = "stopped"
)

// SequenceEnrollment tracks one contact's progress through a sequence.
type SequenceEnrollment struct {
	BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;not null" json:"organization_id"`
	SequenceID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"sequence_id"`
	ContactID      uuid.UUID  `gorm:"type:uuid;index;not null" json:"contact_id"`
	Status         string     `gorm:"size:20;index;default:'active'" json:"status"`
	NextStep       int        `gorm:"default:0" json:"next_step"` // index into the ordered steps
	NextRunAt      *time.Time `gorm:"index" json:"next_run_at,omitempty"`
	StepsSent      int        `gorm:"default:0" json:"steps_sent"`
	StoppedReason  string     `gorm:"size:255" json:"stopped_reason,omitempty"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`
	Contact        *Contact   `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
}

func (SequenceEnrollment) TableName() string { return "sequence_enrollments" }

// AIKnowledgeChunk is one retrievable piece of a knowledge context's text.
type AIKnowledgeChunk struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index;not null" json:"organization_id"`
	ContextID      uuid.UUID `gorm:"type:uuid;index;not null" json:"context_id"`
	Position       int       `gorm:"not null" json:"position"`
	Content        string    `gorm:"type:text;not null" json:"content"`
}

func (AIKnowledgeChunk) TableName() string { return "ai_knowledge_chunks" }
