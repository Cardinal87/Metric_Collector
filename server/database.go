package main

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func configureDatabase(connectionString string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(connectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to database: %v", err)
	}

	db.AutoMigrate(&MetricUnit{})

	query := "SELECT create_hypertable('metrics', 'time', if_not_exists => TRUE)"
	err = db.Exec(query).Error
	if err != nil {
		log.Fatalf("FATAL: Failed to configure metrics hyperdtable: %v", err)
	}
	return db
}

func sendMetrics(metricUnit *MetricUnit, db *gorm.DB) {
	err := db.Create(metricUnit).Error
	if err != nil {
		log.Printf("ERROR: Failed to save metric to database: %v", err)
	}
}
