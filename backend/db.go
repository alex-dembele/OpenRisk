package main

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"github.com/rs/zerolog/log"
)

var DB *gorm.DB

func InitDB() error {
	dsn := os.Getenv("DB_DSN") // "host=postgres user=openrisk password=secret dbname=openrisk"
	if dsn == "" {
		dsn = "host=localhost user=openrisk password=secret dbname=openrisk port=5432 sslmode=disable"
	}
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	
	err = DB.AutoMigrate(&Risk{}, &MitigationPlan{}, &History{}, &User{})
	if err != nil {
		return err
	}
	log.Info().Msg("DB migrated")
	return nil
}