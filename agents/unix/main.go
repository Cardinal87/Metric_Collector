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
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	net_metric "github.com/shirou/gopsutil/v3/net"
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

	partitions, err := disk.Partitions(false)
	var diskInfo []*metricsv1.DiskMetric
	if err != nil {
		log.Printf("WARNING: Unable to retrieve disk partitions: %v", err)
	} else {

		for _, p := range partitions {
			stat, err := disk.Usage(p.Mountpoint)
			if err != nil {
				log.Printf("WARNING: Unable to retrieve info about %s mount point: %v", p.Mountpoint, err)
				continue
			}
			total := float64(stat.Total / 1024.0 / 1024.0)
			usedPercent := float64(stat.UsedPercent)
			free := float64(stat.Free / 1024.0 / 1024.0)

			diskInfo = append(diskInfo, &metricsv1.DiskMetric{
				Device:       p.Device,
				Total:        total,
				UsagePercent: usedPercent,
				Free:         free,
			})
		}
	}

	var netInfo []*metricsv1.NetMetric
	c1, err := net_metric.IOCounters(true)
	if err != nil {
		log.Printf("WARNING: Unable to retrieve network info: %v", err)
	} else {
		time.Sleep(1 * time.Second)

		mp := make(map[string]net_metric.IOCountersStat)
		for _, stat := range c1 {
			mp[stat.Name] = stat
		}
		c2, _ := net_metric.IOCounters(true)

		for _, stat2 := range c2 {
			if stat1, ok := mp[stat2.Name]; ok {
				name := stat2.Name
				sendSpeed := float64((stat2.BytesSent - stat1.BytesSent))
				recvSpeed := float64((stat2.BytesRecv - stat1.BytesRecv))
				netInfo = append(netInfo, &metricsv1.NetMetric{
					Interface: name,
					RecvSpeed: recvSpeed,
					SendSpeed: sendSpeed,
				})

			}
		}
	}

	return &metricsv1.GetMetricsResponse{Name: hostname,
		CpuPercent:   cpuPercent,
		MemPercent:   memPercent,
		Disks:        diskInfo,
		Networks:     netInfo,
		AgentVerison: "unix-v1"}, nil
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
