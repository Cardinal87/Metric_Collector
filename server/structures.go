package main

import "time"

type Node struct {
	Ip        string `json:"ip"`
	Frequency int    `json:"frequency"` //polling frequency in seconds
}

type Config struct {
	Nodes            []Node         `json:"nodes"`
	Logger           Logger         `json:"logger"`
	MethodConfig     []MethodConfig `json:"methodConfig"`
	ConnectionString string         `json:"connectionString"`
}

type Logger struct {
	MaxAge     int  `json:"maxAge"`
	MaxSize    int  `json:"maxSize"`
	MaxBackups int  `json:"maxBackups"`
	Compress   bool `json:"compress"`
}

type MethodConfig struct {
	Name        []MethodName `json:"name"`
	RetryPolicy RetryPolicy  `json:"retryPolicy"`
}

type MethodName struct {
	Service string `json:"service"`
	Method  string `json:"method"`
}

type RetryPolicy struct {
	MaxAttempts          int      `json:"maxAttempts"`
	InitialBackoff       string   `json:"initialBackoff"`
	MaxBackoff           string   `json:"maxBackoff"`
	BackoffMultiplier    float64  `json:"backoffMultiplier"`
	RetryableStatusCodes []string `json:"retryableStatusCodes"`
}

type MetricUnit struct {
	Time       time.Time `gorm:"primaryKey;not null"`
	Hostname   string    `gorm:"index;not null"`
	CpuPercent float32
	MemPercent float32
	RTT        float64
}

func (m MetricUnit) TableName() string {
	return "metrics"
}
