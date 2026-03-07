package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	metricsv1 "github.com/Cardinal87/Metric_Collector/gRPC/gen/go/metrics/v1"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xeipuuv/gojsonschema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

//go:embed config_schema.json
var config_schema_bytes []byte
var database *gorm.DB
var (
	failed_nodes []Node
	mu           sync.RWMutex
)

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
		return nil
	}
	m := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}

	var disksBytes []byte
	var diskParts []string
	for _, disk := range resp.Disks {
		bytes, _ := m.Marshal(disk)
		diskParts = append(diskParts, string(bytes))
	}
	disksBytes = []byte("[" + strings.Join(diskParts, ",") + "]")

	var networkBytes []byte
	var networkParts []string
	for _, network := range resp.Networks {
		bytes, _ := m.Marshal(network)
		networkParts = append(networkParts, string(bytes))
	}
	networkBytes = []byte("[" + strings.Join(networkParts, ",") + "]")

	metricUnit := &MetricUnit{
		Time:         time.Now(),
		Hostname:     resp.Name,
		CpuPercent:   resp.CpuPercent,
		MemPercent:   resp.MemPercent,
		RTT:          float64(rtt),
		Disks:        datatypes.JSON(disksBytes),
		Networks:     datatypes.JSON(networkBytes),
		AgentVersion: resp.AgentVerison,
	}
	log.Printf("%v", resp)
	sendMetrics(metricUnit, database)
	return nil
}

func handleNode(node Node, stopChan chan struct{}, wg *sync.WaitGroup, methodConfig string) {
	socket := node.Ip + ":11111"
	duration := time.Duration(node.Frequency) * time.Second
	ticker := time.NewTicker(duration)

	conn, err := grpc.NewClient(socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(methodConfig))

	if err != nil {
		mu.Lock()
		if !slices.Contains(failed_nodes, node) {
			failed_nodes = append(failed_nodes, node)
		}
		mu.Unlock()
		log.Printf("ERROR: Failed to connect to node %s: %v", node.Ip, err)
		ticker.Stop()
		return
	}

	client := metricsv1.NewMetricServiceClient(conn)

	log.Printf("INFO: Started %s monitoring", node.Ip)

	wg.Go(func() {
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
					mu.Lock()
					if !slices.Contains(failed_nodes, node) {
						failed_nodes = append(failed_nodes, node)
					}
					mu.Unlock()
					return
				}

			}

		}
	})
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

	log.SetOutput(&writer)
}

func main() {
	readyChan := make(chan Options)
	stopChan := make(chan struct{})
	errorCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	go func() {
		config, err := parseConfig("config.json")
		if err != nil {
			log.Printf("FATAL: Unable to read config file: %v", err)
			cancel()
		}

		configureLogger(config.Logger.MaxSize,
			config.Logger.MaxAge,
			config.Logger.MaxBackups,
			config.Logger.Compress)

		db, err := configureDatabase(config.ConnectionString)
		if err != nil {
			log.Printf("FATAL: Unable to configure database: %v", err)
			cancel()
		}
		database = db

		log.Printf("INFO: Application started")

		methodConfigStructure := struct {
			MethodConfig []MethodConfig `json:"methodConfig"`
		}{MethodConfig: config.MethodConfig}
		methodConfigBytes, _ := json.Marshal(methodConfigStructure)
		methodConfig := string(methodConfigBytes)

		var outerWg sync.WaitGroup
		for _, node := range config.Nodes {
			outerWg.Go(func() { handleNode(node, stopChan, &wg, methodConfig) })
		}

		opt := Options{
			wg:           &wg,
			stopChan:     stopChan,
			methodConfig: methodConfig,
		}
		readyChan <- opt
		close(readyChan)
	}()

	defer func() {
		close(stopChan)
		log.Printf("INFO: Stopping host")
		wg.Wait()
	}()

	model := initModel(readyChan)
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion())

	go func() {
		<-errorCtx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		log.Fatalf("FATAL: Unexpected error on UI: %v", err)
	}

}
