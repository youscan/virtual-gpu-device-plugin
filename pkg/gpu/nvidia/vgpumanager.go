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
	"syscall"

	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/fsnotify/fsnotify"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

type vGPUManager struct {
	vGPUCount int
}

// NewVirtualGPUManager create a instance of vGPUManager
func NewVirtualGPUManager(vGPUCount int) *vGPUManager {
	return &vGPUManager{
		vGPUCount: vGPUCount,
	}
}

func (vgm *vGPUManager) Run() error {
	log.Println("Loading NVML")
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		log.Printf("Failed to initialize NVML: %s.", nvml.ErrorString(ret))
		log.Printf("If this is a GPU node, did you set the docker default runtime to `nvidia`?")

		log.Printf("You can check the prerequisites at: https://github.com/kuartis/kuartis-virtual-gpu-device-plugin#prerequisites")
		log.Printf("You can learn how to set the runtime at: https://github.com/kuartis/kuartis-virtual-gpu-device-plugin#quick-start")

		select {}
	}
	defer func() { log.Println("Shutdown of NVML returned:", nvml.Shutdown()) }()

	log.Println("Fetching devices.")
	if getDeviceCount() == 0 {
		log.Println("No devices found. Waiting indefinitely.")
		select {}
	}

	log.Println("Starting FS watcher.")
	watcher, err := newFSWatcher(pluginapi.DevicePluginPath)
	if err != nil {
		log.Println("Failed to created FS watcher.")
		return err
	}
	defer watcher.Close()

	log.Println("Starting OS watcher.")
	sigs := newOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	restart := true
	var devicePlugin *NvidiaDevicePlugin

	log.Println("Starting metric server.")
	go MetricServer()

L:
	for {
		if restart {
			if devicePlugin != nil {
				devicePlugin.Stop()
			}

			devicePlugin = NewNvidiaDevicePlugin(vgm.vGPUCount)
			if err := devicePlugin.Serve(); err != nil {
				log.Printf("You can check the prerequisites at: https://github.com/kuartis/kuartis-virtual-gpu-device-plugin#prerequisites")
				log.Printf("You can learn how to set the runtime at: https://github.com/kuartis/kuartis-virtual-gpu-device-plugin#quick-start")
			} else {
				restart = false
			}
		}

		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				log.Printf("inotify: %s created, restarting.", pluginapi.KubeletSocket)
				restart = true
			}

		case err := <-watcher.Errors:
			log.Printf("inotify: %s", err)

		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				log.Println("Received SIGHUP, restarting.")
				restart = true
			default:
				log.Printf("Received signal \"%v\", shutting down.", s)
				devicePlugin.Stop()
				break L
			}
		}
	}

	return nil
}
