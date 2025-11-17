package main

import "gorm.io/gorm"

type Risk struct {
	gorm.Model
	Name        string `gorm:"unique"`
	Probability int
	Impact      int
	Criticality int
	AssetID     uint `gorm:"index"`
	OwnerID     uint `gorm:"index"`
	Status      string
	Tags        string 
	CustomFields string
}

type MitigationPlan struct {
	gorm.Model
	RiskID     uint
	Action     string
	AssigneeID uint
	Deadline   time.Time
	Progress   int
	Badges     int 
}

type History struct {
	gorm.Model
	RiskID     uint
	ChangeType string
	Diff       string 
}