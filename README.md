# virtual-gpu-device-plugin

This is a fork of <https://github.com/awslabs/aws-virtual-gpu-device-plugin>.

AWS's original plugin does not support memory allocation via plugin, but by defining language specific arguments.

This fork is in active development, with following goals/challanges:

- Support memory allocation via plugin
- Support GPU allocation by model name
- Produce telemetry

End goal is something like:

```yaml
# On a server with 1 T4 and 2 v100 GPUs with 10 vGPU per device
    resources:
      limits:
        k8s.kuartis.com/nvidia-t4: 10
        k8s.kuartis.com/nvidia-t4: 16384
        k8s.kuartis.com/nvidia-v100: 20
        k8s.kuartis.com/nvidia-v100: 32768
```

## Install and Test

```bash

# Install daemonset + service + service monitor (prometheus)
kubectl create -f https://raw.githubusercontent.com/youscan/virtual-gpu-device-plugin/master/manifests/device-plugin.yml

# Notes about daemon set:
# - Uses nvml to find which processes use GPU resources
# - Mounts /proc folder to find container information from process id
# - Uses dockershim socket to read detailed container information

# You can set these variables:
# - --vgpu=<number_of_virtual_gpu_one_pyhsical_gpu_can_have> # Default is 10, Max 48
# - --allowmultigpu=<true|false> # Default is false. Prevents vGPU resources that one container can have to fall on different physical gpus.
```

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nvidia-device-query
spec:
  hostIPC: true # Required for MPS
  containers:
    - name: nvidia-device-query
      image: ghcr.io/kuartis/nvidia-device-query:1.0.0
      command: ["/bin/sh", "-ec", "while :; do echo '.'; sleep 5 ; done"]
      env:
        - name: CUDA_MPS_PINNED_DEVICE_MEM_LIMIT # Memory limit for GPU
          value: 0=2G # Read this: https://developer.nvidia.com/blog/revealing-new-features-in-the-cuda-11-5-toolkit/
      resources:
        limits:
          # Partition your GPUs inside daemon set with --vgpu=<number> argument
          # Request virtual gpu here
          nvidia.com/gpu: '1'
      volumeMounts:
        - name: nvidia-mps
          mountPath: /tmp/nvidia-mps
  volumes:
    - name: nvidia-mps
      hostPath:
        path: /tmp/nvidia-mps
```

## License

This project is licensed under the Apache-2.0 License.
