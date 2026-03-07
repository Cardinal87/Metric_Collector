package main

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func configureDatabase(connectionString string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(connectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	db.AutoMigrate(&MetricUnit{})

	query := "SELECT create_hypertable('metrics', 'time', if_not_exists => TRUE)"
	err = db.Exec(query).Error
	if err != nil {
		return nil, err
	}
	return db, nil
}

func sendMetrics(metricUnit *MetricUnit, db *gorm.DB) {
	err := db.Create(metricUnit).Error
	if err != nil {
		log.Printf("ERROR: Failed to save metric to database: %v", err)
	}
}
