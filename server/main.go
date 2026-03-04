package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	metricsv1 "github.com/Cardinal87/Metric_Collector/gRPC/gen/go/metrics/v1"
	"github.com/xeipuuv/gojsonschema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm"
)

//go:embed config_schema.json
var config_schema_bytes []byte
var database *gorm.DB

func parseConfig(config_path string) (config *Config, err error) {
	configBytes, err := os.ReadFile(config_path)
	if err != nil {
		return nil, err
	}

	schemaLoader := gojsonschema.NewBytesLoader(config_schema_bytes)
	configLoader := gojsonschema.NewBytesLoader(configBytes)
	result, err := gojsonschema.Validate(schemaLoader, configLoader)
	if err != nil {
		return nil, err
	}
	if !result.Valid() {
		for _, err := range result.Errors() {
			log.Printf("ERROR: Validation error: %v", err)
		}
		return nil, fmt.Errorf("Incorrect config file format")
	}
	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func getMetrics(client metricsv1.MetricServiceClient) error {
	req := &metricsv1.GetMetricsRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.GetMetrics(ctx, req, grpc.WaitForReady(true))
	rtt := time.Since(start)

	if err != nil {
		return err
	}

	if len(resp.Name) == 0 {
		pr, _ := peer.FromContext(ctx)
		log.Printf("WARNING: received metric from %s contains an empty hostname and will be discarded", pr.Addr)
	}

	metricUnit := &MetricUnit{
		Time:       time.Now(),
		Hostname:   resp.Name,
		CpuPercent: resp.CpuPercent,
		MemPercent: resp.MemPercent,
		RTT:        float64(rtt),
	}
	sendMetrics(metricUnit, database)
	return nil
}

func handleNode(node Node, stopChan chan struct{}, wg *sync.WaitGroup, methodConfig string) {
	defer wg.Done()

	socket := node.Ip + ":11111"
	duration := time.Duration(node.Frequency) * time.Second
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	conn, err := grpc.NewClient(socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(methodConfig))
	if err != nil {
		log.Printf("ERROR: Failed to connect to node %s: %v", node.Ip, err)
		return
	}
	defer conn.Close()

	client := metricsv1.NewMetricServiceClient(conn)

	log.Printf("INFO: Started %s monitoring", node.Ip)

	for {
		select {
		case _ = <-stopChan:
			log.Printf("INFO: Stopped %s monitoring", node.Ip)
			return

		case _ = <-ticker.C:
			err := getMetrics(client)
			if err != nil {
				log.Printf("ERROR: Unable to retrieve metrics from %s: %v", node.Ip, err)
				return
			}

		}
	}
}

func configureLogger(maxSize int, maxAge int, maxBackups int, compress bool) {
	os.MkdirAll("logs", 0755)
	writer := lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    maxSize,
		MaxAge:     maxAge,
		MaxBackups: maxBackups,
		Compress:   compress,
	}

	multi := io.MultiWriter(os.Stderr, &writer)
	log.SetOutput(multi)
}

func main() {
	config, err := parseConfig("config.json")
	if err != nil {
		log.Fatal("FATAL: Unable to read config file: ", err)
	}

	configureLogger(config.Logger.MaxSize,
		config.Logger.MaxAge,
		config.Logger.MaxBackups,
		config.Logger.Compress)

	db := configureDatabase(config.ConnectionString)
	database = db

	log.Printf("INFO: Application started")

	methodConfigStructure := struct {
		MethodConfig []MethodConfig `json:"methodConfig"`
	}{MethodConfig: config.MethodConfig}
	methodConfigBytes, _ := json.Marshal(methodConfigStructure)
	methodConfig := string(methodConfigBytes)

	stopChan := make(chan struct{})

	var wg sync.WaitGroup
	for _, node := range config.Nodes {
		wg.Add(1)
		go handleNode(node, stopChan, &wg, methodConfig)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	close(stopChan)

	log.Printf("INFO: Stopping host")
	wg.Wait()
}
