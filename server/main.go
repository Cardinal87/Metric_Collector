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
)

//go:embed config_schema.json
var config_schema_bytes []byte

type Node struct {
	Ip        string `json:"ip"`
	Frequency int    `json:"frequency"` //polling frequency in seconds
}

type Config struct {
	Nodes []Node `json:"nodes"`
}

func parseConfig(config_path string) (config *Config, err error) {
	schemaLoader := gojsonschema.NewBytesLoader(config_schema_bytes)

	abs_config_path, _ := filepath.Abs(config_path)
	configLoader := gojsonschema.NewReferenceLoader("file://" + abs_config_path)

	result, err := gojsonschema.Validate(schemaLoader, configLoader)
	if err != nil {
		return nil, err
	}
	if !result.Valid() {
		return nil, fmt.Errorf("%s", result.Errors()[0])
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

func getMetrics(client metricsv1.MetricServiceClient) {
	req := &metricsv1.GetMetricsRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.GetMetrics(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Name: %s\nCpu Percent: %v\nMemory Percent: %v\n", resp.Name, resp.CpuPercent, resp.MemPercent)
}

func handleNode(node Node, stopChan chan struct{}) {
	socket := node.Ip + ":11111"
	duration := time.Duration(node.Frequency) * time.Second
	ticker := time.NewTicker(duration)

	conn, err := grpc.NewClient(socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	client := metricsv1.NewMetricServiceClient(conn)

	go func() {
		defer conn.Close()
		defer ticker.Stop()

		for {
			select {
			case _ = <-stopChan:
				return

			case _ = <-ticker.C:
				getMetrics(client)

			}
		}
	}()
}

func main() {
	config, err := parseConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}
	stopChan := make(chan struct{})
	for _, node := range config.Nodes {
		go handleNode(node, stopChan)
	}

	fmt.Println("Для завершения нажмите Enter")
	fmt.Println()
	fmt.Scanln()

	close(stopChan)
	time.Sleep(time.Second)
}
