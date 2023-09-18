// Copyright 2020 Amazon.com, Inc. or its affiliates
// Copyright 2022 Kuartis.com
// Copyright 2023 YouScan.io
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

package main

import (
	"flag"
	"log"

	"github.com/youscan/virtual-gpu-device-plugin/pkg/gpu/nvidia"
)

var (
	vGPU = flag.Int("vgpu", 10, "Number of virtual GPUs")
	allowMultiGpu = flag.Bool("allowmultigpu", false, "Allow multiple pyhsical GPU instance assingment to the pod")
)

const VOLTA_MAXIMUM_MPS_CLIENT = 48

func main() {
	flag.Parse()
	log.Println("Start virtual GPU device plugin")

	if *vGPU > VOLTA_MAXIMUM_MPS_CLIENT {
		log.Fatal("Number of virtual GPUs can not exceed maximum number of MPS clients")
	}

	vgm := nvidia.NewVirtualGPUManager(*vGPU, *allowMultiGpu)

	err := vgm.Run()
	if err != nil {
		log.Fatalf("Failed due to %v", err)
	}
}
