package main

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
	"bytes"
	"encoding/json"
)

func TestGetRisks(t *testing.T) {
	r := gin.Default()
	r.GET("/risks", GetRisks)
	req := httptest.NewRequest("GET", "/risks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateRisk(t *testing.T) {
	r := gin.Default()
	r.POST("/risks", CreateRisk)
	risk := Risk{Name: "Test", Probability: 3, Impact: 4, Criticality: 5}
	jsonData, _ := json.Marshal(risk)
	req := httptest.NewRequest("POST", "/risks", bytes.NewBuffer(jsonData))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
}

