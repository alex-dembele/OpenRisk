package main

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog"
	"os"
	"github.com/gin-contrib/ratelimit"
	"time"
	"golang.org/x/time/rate"
	"github.com/gin-contrib/cors"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err := InitDB(); err != nil {
		log.Fatal().Err(err).Msg("Failed to init DB")
	}
	log.Info().Msg("DB connected and migrated")

	r := gin.Default()

	// Prod middlewares
	r.Use(gin.Recovery())
	r.Use(cors.Default()) // CORS for frontend
	r.Use(ratelimit.New(rate.Limit(10), rate.Every(time.Minute))) // Rate limit 10/min
	r.Use(LoggerMiddleware()) // Audit logs
	r.Use(JWTMiddleware()) // RBAC

	// Routes
	r.GET("/risks", GetRisks)
	r.POST("/risks", CreateRisk)
	r.GET("/risks/:id", GetRisk)
	r.PUT("/risks/:id", UpdateRisk)
	r.DELETE("/risks/:id", DeleteRisk)

	r.GET("/plans", GetPlans)
	r.POST("/plans", CreatePlan)
	r.GET("/plans/:id", GetPlan)
	r.PUT("/plans/:id", UpdatePlan)
	r.DELETE("/plans/:id", DeletePlan)

	r.GET("/history/:risk_id", GetHistory)

	r.POST("/exports/pdf", ExportPDF)
	r.POST("/exports/csv", ExportCSV)
	r.POST("/exports/json", ExportJSON)

	r.POST("/integrate/openasset", IntegrateOpenAsset) 
	

	r.Run(":8000") // Prod: Use env PORT, TLS gin.RunTLS
}