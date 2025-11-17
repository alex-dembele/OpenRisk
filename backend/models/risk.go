package models

import "gorm.io/gorm"

type Risk struct {
	gorm.Model
	Name        string `json:"name"`
	Probability int    `json:"probability"`
	Impact      int    `json:"impact"`
	Criticality int    `json:"criticality"`
	AssetID     uint   `json:"asset_id"`
	OwnerID     uint   `json:"owner_id"`
	Status      string `json:"status"`
	
}