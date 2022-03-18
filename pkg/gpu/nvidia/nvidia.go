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
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"golang.org/x/net/context"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

func check(ret nvml.Return) bool {
	if ret != nvml.SUCCESS {
		log.Printf("Error: %s", nvml.ErrorString(ret))
		return false
	}
	return true
}

func checkAndPanic(ret nvml.Return) {
	if ret != nvml.SUCCESS {
		log.Panicf("Fatal: %s", nvml.ErrorString(ret))
	}
}

// Instead of returning physical GPU devices, device plugin returns vGPU devices here.
// Total number of vGPU depends on the vGPU count user specify.
func getVGPUDevices(vGPUCount int) []*pluginapi.Device {
	var devs []*pluginapi.Device
	for i := 0; i < getDeviceCount(); i++ {
		d, ret := nvml.DeviceGetHandleByIndex(i)
		checkAndPanic(ret)

		log.Printf("Device Memory: %d, vGPU Count: %d", getDeviceMemory(d), vGPUCount)

		for j := 0; j < vGPUCount; j++ {
			vGPUDeviceID := getVGPUID(getDeviceUUID(d), j)
			dev := pluginapi.Device{
				ID:     vGPUDeviceID,
				Health: pluginapi.Healthy,
			}

			// TODO: Enable Affinity for kubernetes > 1.16.x
			//if d.CPUAffinity != nil {
			//	dev.Topology = &pluginapi.TopologyInfo{
			//		Nodes: []*pluginapi.NUMANode{
			//			&pluginapi.NUMANode{
			//				ID: int64(*(d.CPUAffinity)),
			//			},
			//		},
			//	}
			//}

			devs = append(devs, &dev)
		}
	}

	return devs
}

func getDeviceCount() int {
	n, ret := nvml.DeviceGetCount()
	checkAndPanic(ret)
	return n
}

func getDeviceUUID(device nvml.Device) string {
	uuid, ret := device.GetUUID()
	checkAndPanic(ret)
	return uuid
}

func getDeviceMemory(device nvml.Device) uint64 {
	mem, ret := device.GetMemoryInfo()
	checkAndPanic(ret)
	return mem.Total
}

func getPhysicalGPUDevices() []string {
	var devs []string
	for i := 0; i < getDeviceCount(); i++ {
		d, ret := nvml.DeviceGetHandleByIndex(i)
		checkAndPanic(ret)
		devs = append(devs, getDeviceUUID(d))
	}
	return devs
}

func getVGPUID(deviceID string, vGPUIndex int) string {
	return fmt.Sprintf("%s-%d", deviceID, vGPUIndex)
}

func getPhysicalDeviceID(vGPUDeviceID string) string {
	lastDashIndex := strings.LastIndex(vGPUDeviceID, "-")
	return vGPUDeviceID[0:lastDashIndex]
}

func deviceExists(devs []*pluginapi.Device, id string) bool {
	for _, d := range devs {
		if d.ID == id {
			return true
		}
	}
	return false
}

func physicialDeviceExists(devs []string, id string) bool {
	for _, d := range devs {
		if d == id {
			return true
		}
	}
	return false
}

func watchXIDs(ctx context.Context, devs []*pluginapi.Device, xids chan<- *pluginapi.Device) {
	eventSet, ret := nvml.EventSetCreate()
	checkAndPanic(ret)
	defer nvml.EventSetFree(eventSet)
	var physicalDeviceIDs []string

	// We don't have to loop all virtual GPUs here. Only need to check physical GPUs.
	for _, d := range devs {
		physicalDeviceID := getPhysicalDeviceID(d.ID)
		if physicialDeviceExists(physicalDeviceIDs, physicalDeviceID) {
			continue
		}
		physicalDeviceIDs = append(physicalDeviceIDs, physicalDeviceID)

		log.Printf("virtual id %s physical id %s", d.ID, physicalDeviceID)

		device, ret := nvml.DeviceGetHandleByUUID(physicalDeviceID)
		checkAndPanic(ret)
		ret = nvml.DeviceRegisterEvents(device, nvml.EventTypeXidCriticalError, eventSet)
		if ret == nvml.ERROR_NOT_SUPPORTED {
			log.Printf("Warning: %s is too old to support healthchecking: %s. Marking it unhealthy.", physicalDeviceID, nvml.ErrorString(ret))
			xids <- d
			continue
		}
		checkAndPanic(ret)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e, ret := nvml.EventSetWait(eventSet, 5000)
		checkAndPanic(ret)
		if e.EventType != nvml.EventTypeXidCriticalError {
			continue
		}

		// FIXME: formalize the full list and document it.
		// http://docs.nvidia.com/deploy/xid-errors/index.html#topic_4
		// Application errors: the GPU should still be healthy
		if e.EventData == 31 || e.EventData == 43 || e.EventData == 45 {
			continue
		}

		uuid, ret := e.Device.GetUUID()
		checkAndPanic(ret)
		if len(uuid) == 0 {
			// All devices are unhealthy
			for _, d := range devs {
				log.Printf("XidCriticalError: Xid=%d, All devices will go unhealthy.", e.EventData)
				xids <- d
			}
			continue
		}

		for _, d := range devs {
			if d.ID == uuid {
				log.Printf("XidCriticalError: Xid=%d on GPU=%s, the device will go unhealthy.", e.EventData, d.ID)
				xids <- d
			}
		}
	}
}
