package main

import (
	"context"
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
	name, _ := os.Hostname()
	cpu, _ := cpu.Percent(time.Second, false)
	mem, _ := mem.VirtualMemory()

	return &metricsv1.GetMetricsResponse{Name: name, CpuPercent: float32(cpu[0]), MemPercent: float32(mem.UsedPercent)}, nil
}

func main() {
	listener, err := net.Listen("tcp", ":11111")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	server := grpc.NewServer()

	metricsv1.RegisterMetricServiceServer(server, &metricAgent{})
	err = server.Serve(listener)

	if err != nil {
		log.Fatal(err)
	}
}
