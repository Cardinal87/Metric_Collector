package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	metricsv1 "github.com/Cardinal87/Metric_Collector/gRPC/gen/go/metrics/v1"
	"github.com/xeipuuv/gojsonschema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/natefinch/lumberjack.v2"
)

//go:embed config_schema.json
var config_schema_bytes []byte

type Node struct {
	Ip        string `json:"ip"`
	Frequency int    `json:"frequency"` //polling frequency in seconds
}

type Logger struct {
	MaxAge     int  `json:"maxAge"`
	MaxSize    int  `json:"maxSize"`
	MaxBackups int  `json:"maxBackups"`
	Compress   bool `json:"compress"`
}

type Config struct {
	Nodes  []Node `json:"nodes"`
	Logger Logger `json:"logger"`
}

func parseConfig(config_path string) (config *Config, err error) {
	schemaLoader := gojsonschema.NewBytesLoader(config_schema_bytes)

	abs_config_path, err := filepath.Abs(config_path)
	if err != nil {
		return nil, err
	}
	configLoader := gojsonschema.NewReferenceLoader("file://" + abs_config_path)

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

	configFile, err := os.Open(abs_config_path)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	bytes, err := io.ReadAll(configFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func getMetrics(client metricsv1.MetricServiceClient) error {
	req := &metricsv1.GetMetricsRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.GetMetrics(ctx, req)
	if err != nil {
		return err
	}
	fmt.Printf("Name: %s\nCpu Percent: %v\nMemory Percent: %v\n", resp.Name, resp.CpuPercent, resp.MemPercent)
	return nil
}

func handleNode(node Node, stopChan chan struct{}) {
	socket := node.Ip + ":11111"
	duration := time.Duration(node.Frequency) * time.Second
	ticker := time.NewTicker(duration)

	conn, err := grpc.NewClient(socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("ERROR: Failed to connect to node %s: %v", node.Ip, err)
		return
	}
	client := metricsv1.NewMetricServiceClient(conn)
	log.Printf("INFO: Started %s monitoring", node.Ip)
	go func() {
		defer conn.Close()
		defer ticker.Stop()

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
	}()
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
		log.Fatal("FATAL: Unable to read config file", err)
	}

	configureLogger(config.Logger.MaxSize,
		config.Logger.MaxAge,
		config.Logger.MaxBackups,
		config.Logger.Compress)

	log.Printf("INFO: Application started")
	stopChan := make(chan struct{})
	defer func() {
		close(stopChan)
		time.Sleep(250 * time.Millisecond)
	}()

	for _, node := range config.Nodes {
		go handleNode(node, stopChan)
	}

	fmt.Println("Press Enter to stop")
	fmt.Println()
	fmt.Scanln()
	log.Printf("INFO: Stopping host")

}
