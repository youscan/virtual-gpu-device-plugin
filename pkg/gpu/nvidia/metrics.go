package nvidia

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	dockershimsocket = "/var/run/dockershim.sock"
	timeout          = 10 * time.Second
)

var node = os.Getenv("NODE_NAME")

var metricsFormat = `
# HELP gpu_memory_usage_per_container Shows the GPU memory usage per container.
# TYPE gpu_memory_usage_per_container gauge
{{- range $m := . }}
gpu_memory_usage_per_container{pid="{{ $m.Pid }}",gpuindex="{{ $m.GpuIndex }}",gpuuuid="{{ $m.GpuUUID }}",node="{{ $m.Node }}",namespace="{{ $m.Namespace }}",pod="{{ $m.Pod }}",poduid="{{ $m.PodUid }}",container="{{ $m.Container }}",containerid="{{ $m.ContainerId }}"} {{ $m.UsedGpuMemory }}
{{- end -}}`

type metric struct {
	Pid           uint32
	UsedGpuMemory uint64
	GpuIndex      int
	GpuUUID       string
	Node          string
	Namespace     string
	Pod           string
	PodUid        string
	Container     string
	ContainerId   string
}

type containerInfo struct {
	Node        string
	Namespace   string
	Pod         string
	PodUid      string
	Container   string
	ContainerId string
}

func MetricServer() {
	http.HandleFunc("/metrics", collectMetrics)
	http.ListenAndServe(":8080", nil)
}

func collectMetrics(w http.ResponseWriter, r *http.Request) {
	runtimeClient, runtimeConn, err := getRuntimeClient()
	if err != nil {
		log.Println("Error getting runtime client:", err)
		return
	}
	if runtimeConn != nil {
		defer runtimeConn.Close()
	}
	containers, err := runtimeClient.ListContainers(context.Background(), &pb.ListContainersRequest{})
	if err != nil {
		log.Println("Error getting containers:", err)
		return
	}
	log.Printf("Found %d containers", len(containers.Containers))
	containerMap := make(map[string]containerInfo)
	for _, container := range containers.GetContainers() {
		containerMap[container.Id] = containerInfo{
			Node:        node,
			Namespace:   container.Labels["io.kubernetes.pod.namespace"],
			Pod:         container.Labels["io.kubernetes.pod.name"],
			PodUid:      container.Labels["io.kubernetes.pod.uid"],
			Container:   container.Metadata.Name,
			ContainerId: container.Id,
		}
	}
	collected := []metric{}
	for i := 0; i < getDeviceCount(); i++ {
		d, ret := nvml.DeviceGetHandleByIndex(i)
		check(ret)
		processes, ret := nvml.DeviceGetMPSComputeRunningProcesses(d)
		check(ret)
		log.Printf("Found %d processes on GPU %d", len(processes), i)
		for _, process := range processes {
			containerId := getContainerId(process.Pid)
			container := containerMap[containerId]
			collected = append(collected, metric{
				Pid:           process.Pid,
				UsedGpuMemory: process.UsedGpuMemory,
				GpuIndex:      i,
				GpuUUID:       getDeviceUUID(d),
				Node:          container.Node,
				Namespace:     container.Namespace,
				Pod:           container.Pod,
				PodUid:        container.PodUid,
				Container:     container.Container,
				ContainerId:   container.ContainerId,
			})
		}
	}

	t := template.Must(template.New("metrics").Parse(metricsFormat))
	var res bytes.Buffer
	if err := t.Execute(&res, collected); err != nil {
		w.Write([]byte(fmt.Sprintf("Error generating metrics: %s", err)))
	} else {
		w.Write(res.Bytes())
	}
}

func getRuntimeClient() (pb.RuntimeServiceClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, dockershimsocket, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	return pb.NewRuntimeServiceClient(conn), conn, nil
}

func getContainerId(pid uint32) string {
	file := fmt.Sprintf("/host/proc/%d/cpuset", pid)
	data, err := os.ReadFile(file)
	if err != nil {
		log.Printf("Error reading proc file %s for process: %d, error: %s", file, pid, err)
	}
	proc := string(data)
	containerId := proc[strings.LastIndex(proc, "/")+1:]
	log.Printf("Found container id %s for process: %d", containerId, pid)
	return containerId
}
