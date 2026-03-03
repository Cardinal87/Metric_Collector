package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"

	metricsv1 "github.com/Cardinal87/Metric_Collector/gRPC/gen/go/metrics/v1"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type metricAgent struct {
	metricsv1.UnimplementedMetricServiceServer
}

func (this *metricAgent) GetMetrics(ctx context.Context, request *metricsv1.GetMetricsRequest) (*metricsv1.GetMetricsResponse, error) {
	hostname := "Undefined"
	if name, err := os.Hostname(); err != nil {
		log.Printf("WARNING: Unable to retrieve hostname: %v", err)
	} else {
		hostname = name
	}

	cpuPercent := float32(-1)
	if cpu, err := cpu.Percent(time.Second, false); err != nil {
		log.Printf("WARNING: Unable to retrieve cpu load: %v", err)
	} else {
		cpuPercent = float32(cpu[0])
	}

	memPercent := float32(-1)
	if mem, err := mem.VirtualMemory(); err != nil {
		log.Printf("WARNING: Unable to retrieve virtual memory load: %v", err)
	} else {
		memPercent = float32(mem.UsedPercent)
	}

	return &metricsv1.GetMetricsResponse{Name: hostname, CpuPercent: cpuPercent, MemPercent: memPercent}, nil
}

func main() {
	listener, err := net.Listen("tcp", ":11111")
	if err != nil {
		log.Fatalf("FATAL: Unable to listen port 11111: %v", err)
	}
	defer listener.Close()

	server := grpc.NewServer()
	metricsv1.RegisterMetricServiceServer(server, &metricAgent{})
	go func() {
		err = server.Serve(listener)
		if err != nil {
			log.Fatalf("FATAL: Agent terminated unexpectly: %v", err)
		}
	}()
	log.Printf("INFO: Agent successfully started")
	fmt.Println("Press Enter to stop")
	fmt.Scanln()
	log.Printf("INFO: Stopping agent")
	server.GracefulStop()
	log.Printf("INFO: Agent stopped")

}
