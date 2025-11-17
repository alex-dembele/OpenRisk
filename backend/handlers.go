package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"github.com/google/uuid"
	"time"
	"encoding/json"
	"gopdf" 
	"bytes"
	"encoding/csv"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate" 
)

func GetRisks(c *gin.Context) {
	var risks []Risk
	if err := DB.Find(&risks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, risks)
}

func CreateRisk(c *gin.Context) {
	var risk Risk
	if err := c.BindJSON(&risk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := DB.Create(&risk).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	LogHistory(risk.ID, "Create", risk)
	// Webhook to OpenDefender 
	SendWebhook("risk_created", risk)
	c.JSON(http.StatusCreated, risk)
}

func UpdateRisk(c *gin.Context) {
	id := c.Param("id")
	var risk Risk
	if err := DB.First(&risk, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Risk not found"})
		return
	}
	oldRisk := risk // For diff
	if err := c.BindJSON(&risk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := DB.Save(&risk).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	diff := ComputeDiff(oldRisk, risk)
	LogHistory(risk.ID, "Update", diff)
	
	if risk.Status == "Mitigated" {
		AwardUserLevel(risk.OwnerID)
	}
	SendWebhook("risk_updated", risk)
	c.JSON(http.StatusOK, risk)
}



func ExportPDF(c *gin.Context) {
	
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "OpenRisk Report")
	
	var risks []Risk
	DB.Find(&risks)
	for _, r := range risks {
		pdf.Ln(10)
		pdf.Cell(40, 10, r.Name)
		
	}
	buf := new(bytes.Buffer)
	pdf.Output(buf)
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}



func IntegrateOpenAsset(c *gin.Context) {
	var payload map[string]interface{} 
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	risk := Risk{AssetID: payload["asset_id"].(uuid.UUID), Name: "Asset Risk", /* auto-score */ }
	DB.Create(&risk)
	log.Info().Msg("Integrated from OpenAsset")
	c.JSON(http.StatusOK, gin.H{"status": "integrated"})
}



func LogHistory(riskID uuid.UUID, changeType string, data interface{}) {
	diffJSON, _ := json.Marshal(data)
	history := History{RiskID: riskID, ChangeType: changeType, Diff: string(diffJSON)}
	DB.Create(&history)
}

func ComputeDiff(old, new interface{}) string {
	
	oldJSON, _ := json.Marshal(old)
	newJSON, _ := json.Marshal(new)
	return string(oldJSON) + " -> " + string(newJSON) 
}

func AwardUserLevel(userID uuid.UUID) {
	var user User
	DB.First(&user, "id = ?", userID)
	user.Level += 1 
	DB.Save(&user)
	
}

func SendWebhook(eventType string, payload interface{}) {
	// Simple chan or HTTP post to OpenDefender services
	// Ex. go http.Post("http://openasset:port/webhook", payload)
	log.Info().Msgf("Webhook sent: %s", eventType)
}