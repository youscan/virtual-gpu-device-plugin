// Copyright 2020 Amazon.com, Inc. or its affiliates
// Copyright 2022 Kuartis.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nvidia

import (
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName           = "k8s.kuartis.com/vgpu"
	serverSock             = pluginapi.DevicePluginPath + "nvidia-kuartis-vgpu.sock"
	envDisableHealthChecks = "DP_DISABLE_HEALTHCHECKS"
	allHealthChecks        = "xids"
)

// NvidiaDevicePlugin implements the Kubernetes device plugin API
type NvidiaDevicePlugin struct {
	devs         []*pluginapi.Device
	physicalDevs []string

	allowMultiGpu bool

	socket string

	stop   chan interface{}
	health chan *pluginapi.Device

	server *grpc.Server
}

// NewNvidiaDevicePlugin returns an initialized NvidiaDevicePlugin
func NewNvidiaDevicePlugin(vGPUCount int, allowMultiGpu bool) *NvidiaDevicePlugin {
	physicalDevs := getPhysicalGPUDevices()
	vGPUDevs := getVGPUDevices(vGPUCount)

	return &NvidiaDevicePlugin{
		devs:          vGPUDevs,
		physicalDevs:  physicalDevs,
		allowMultiGpu: allowMultiGpu,
		socket:        serverSock,
		stop:          make(chan interface{}),
		health:        make(chan *pluginapi.Device),
	}
}

// dial establishes the gRPC communication with the registered device plugin.
func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

// Start starts the gRPC server of the device plugin
func (m *NvidiaDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go func() {
		lastCrashTime := time.Now()
		restartCount := 0
		for {
			log.Println("Starting GRPC server")
			err := m.server.Serve(sock)
			if err != nil {
				log.Printf("GRPC server crashed with error: %v", err)
			}
			// restart if it has not been too often
			// i.e. if server has crashed more than 5 times and it didn't last more than one hour each time
			if restartCount > 5 {
				// quit
				log.Fatal("GRPC server has repeatedly crashed recently. Quitting")
			}
			timeSinceLastCrash := time.Since(lastCrashTime).Seconds()
			lastCrashTime = time.Now()
			if timeSinceLastCrash > 3600 {
				// it has been one hour since the last crash.. reset the count
				// to reflect on the frequency
				restartCount = 1
			} else {
				restartCount += 1
			}
		}
	}()

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	//go m.healthcheck()

	return nil
}

// Stop stops the gRPC server
func (m *NvidiaDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *NvidiaDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		log.Printf("endpoint %s, Dial conn error: %s", kubeletEndpoint, err)
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		log.Printf("client register: %s", err)
		return err
	}
	return nil
}

// ListAndWatch lists devices and update that list according to the health status
func (m *NvidiaDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			log.Printf("device marked unhealthy: %s", d.ID)
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

func (m *NvidiaDevicePlugin) unhealthy(dev *pluginapi.Device) {
	m.health <- dev
}

// GetDevicePluginOptions returns the values of the optional settings for this plugin
func (m *NvidiaDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{GetPreferredAllocationAvailable: !m.allowMultiGpu}, nil
}

// GetPreferredAllocation returns the preferred allocation from the set of devices specified in the request
func (m *NvidiaDevicePlugin) GetPreferredAllocation(ctx context.Context, r *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	log.Printf("GetPreferredAllocation request: %v", r)
	response := &pluginapi.PreferredAllocationResponse{}
	for _, req := range r.ContainerRequests {
		// available vGPUs per physical GPU
		availableGpuMap := make(map[string][]string)
		for _, id := range req.AvailableDeviceIDs {
			if !deviceExists(m.devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown available device: %s", id)
			}
			availableGpuMap[getPhysicalDeviceID(id)] = append(availableGpuMap[getPhysicalDeviceID(id)], id)
		}
		// preferred vGPUs per physical GPU
		mustIncludeGpuMap := make(map[string][]string)
		for _, id := range req.MustIncludeDeviceIDs {
			if !deviceExists(m.devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown must include device: %s", id)
			}
			mustIncludeGpuMap[getPhysicalDeviceID(id)] = append(mustIncludeGpuMap[getPhysicalDeviceID(id)], id)
		}

		response.ContainerResponses = append(response.ContainerResponses, &pluginapi.ContainerPreferredAllocationResponse{
			DeviceIDs: findAvailableDevicesOnSamePhysicalGPU(availableGpuMap, mustIncludeGpuMap, int(req.AllocationSize)),
		})
	}
	log.Printf("GetPreferredAllocation response: %v", response)
	return response, nil
}

func findAvailableDevicesOnSamePhysicalGPU(availableGpuMap map[string][]string, mustIncludeGpuMap map[string][]string, size int) []string {
	if size <= 0 {
		return []string{}
	}
	if len(mustIncludeGpuMap) == 1 {
		for _, ids := range mustIncludeGpuMap {
			if len(ids) >= size {
				return ids[:size]
			}
		}
	}
	for _, ids := range availableGpuMap {
		if len(ids) >= size {
			return ids[:size]
		}
	}
	return []string{}
}

// Allocate which return list of devices.
func (m *NvidiaDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	devs := m.devs
	response := new(pluginapi.AllocateResponse)
	physicalDevsMap := make(map[string]bool)
	for _, req := range reqs.ContainerRequests {
		for _, id := range req.DevicesIDs {
			if !deviceExists(devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
			}

			// Convert virtual GPUDeviceId to physical GPUDeviceID
			physicalDevId := getPhysicalDeviceID(id)
			if !physicalDevsMap[physicalDevId] {
				physicalDevsMap[physicalDevId] = true
			}

			dev := getDeviceById(devs, id)
			if dev == nil {
				return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
			}

			if dev.Health != pluginapi.Healthy {
				return nil, fmt.Errorf("invalid allocation request with unhealthy device %s", id)
			}
		}

		if !m.allowMultiGpu && len(physicalDevsMap) > 1 {
			return nil, fmt.Errorf("invalid allocation request: multiple pyhsical GPUs are not allowed. Requested virtual device ids: %+v", req.DevicesIDs)
		}

		// Set physical GPU devices as container visible devices
		visibleDevs := make([]string, 0, len(physicalDevsMap))
		for visibleDev := range physicalDevsMap {
			visibleDevs = append(visibleDevs, visibleDev)
		}

		cresp := new(pluginapi.ContainerAllocateResponse)

		cresp.Envs = map[string]string{}
		cresp.Envs["NVIDIA_VISIBLE_DEVICES"] = strings.Join(visibleDevs, ",")
		cresp.Envs["CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"] = fmt.Sprintf("%d", 100*len(req.DevicesIDs)/len(m.devs))

		cresp.Annotations = map[string]string{}
		cresp.Annotations["k8s.kuartis.com/gpu-ids"] = strings.Join(visibleDevs, ",")
		cresp.Annotations["k8s.kuartis.com/vgpu-ids"] = strings.Join(req.DevicesIDs, ",")

		log.Printf("Allocated physical devices: %s", strings.Join(visibleDevs, ","))
		log.Printf("Allocated virtual devices: %s", strings.Join(req.DevicesIDs, ","))
		log.Printf("Allocated MPS ACTIVE THREAD PERCENTAGE: %s", fmt.Sprintf("%d", 100*len(req.DevicesIDs)/len(m.devs)))

		response.ContainerResponses = append(response.ContainerResponses, cresp)
	}

	return response, nil
}

func (m *NvidiaDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *NvidiaDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Need to make sure all health check check against real device but not the virtual device

func (m *NvidiaDevicePlugin) healthcheck() {
	disableHealthChecks := strings.ToLower(os.Getenv(envDisableHealthChecks))
	if disableHealthChecks == "all" {
		disableHealthChecks = allHealthChecks
	}

	ctx, cancel := context.WithCancel(context.Background())

	var xids chan *pluginapi.Device
	if !strings.Contains(disableHealthChecks, "xids") {
		xids = make(chan *pluginapi.Device)
		go watchXIDs(ctx, m.devs, xids)
	}

	for {
		select {
		case <-m.stop:
			cancel()
			return
		case dev := <-xids:
			m.unhealthy(dev)
		}
	}
}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *NvidiaDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket, resourceName)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		m.Stop()
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}

func getDeviceById(devices []*pluginapi.Device, deviceId string) *pluginapi.Device {
	for _, d := range devices {
		if d.ID == deviceId {
			return d
		}
	}

	return nil
}
