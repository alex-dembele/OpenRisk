package main

import (
	"time"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"encoding/json"
)

type Risk struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Name        string `gorm:"unique"`
	Probability int `gorm:"check:probability >=1 AND probability <=5"`
	Impact      int `gorm:"check:impact >=1 AND impact <=5"`
	Criticality int `gorm:"check:criticality >=1 AND criticality <=5"`
	AssetID     uuid.UUID `gorm:"index"`
	OwnerID     uuid.UUID `gorm:"index"`
	Status      string `gorm:"type:enum('Open','Mitigated','Closed');default:'Open'"`
	Tags        string `gorm:"type:jsonb"` // ["CIS","ISO"]
	CustomFields string `gorm:"type:jsonb"` // User-defined
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Risk) Score() int {
	return r.Probability * r.Impact * r.Criticality // Auto-score
}

type MitigationPlan struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	RiskID      uuid.UUID `gorm:"index"`
	Action      string
	AssigneeID  uuid.UUID
	Deadline    time.Time
	Progress    int `gorm:"default:0"`
	Badges      int `gorm:"default:0"` 
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (p *MitigationPlan) AwardBadge() {
	p.Badges += 1 
}

type History struct {
	ID         uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	RiskID     uuid.UUID `gorm:"index"`
	ChangeType string
	Diff       string `gorm:"type:jsonb"`
	CreatedAt  time.Time
}

type User struct {
	ID    uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Email string `gorm:"unique"`
	Role  string `gorm:"type:enum('Admin','Analyst','Viewer');default:'Viewer'"`
	Level int `gorm:"default:0"` 
}