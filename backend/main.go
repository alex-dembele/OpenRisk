package main

import (
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
)

var db *gorm.DB

func main() {
	dsn := "host=postgres user=openrisk password=secret dbname=openrisk port=5432 sslmode=disable"
	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	r := gin.Default()
	r.Use(JWTMiddleware()) 

	r.GET("/risks", GetRisks)
	r.Run(":8000")
}

func GetRisks(c *gin.Context) {
	var risks []Risk 
	if err := db.Find(&risks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, risks)
}


func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		
		c.Next()
	}
}