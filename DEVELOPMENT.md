## Build with Docker

```shell
git clone https://github.com/youscan/virtual-gpu-device-plugin.git && cd virtual-gpu-device-plugin
docker build -t youscan/virtual-gpu-device-plugin:v0.2.0 .
```

#### Run locally

```shell
docker run --security-opt=no-new-privileges --cap-drop=ALL --ipc=host --network=none -it -v /var/lib/kubelet/device-plugins:/var/lib/kubelet/device-plugins youscan/virtual-gpu-device-plugin:v0.1.0
```

#### Deploy as Daemon Set

```shell
kubectl create -f vgpu-device-plugin.yml
```

## Build without Docker

### Build

```shell
export CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files'
go build -ldflags="-s -w" -o plugin
```

### Run locally

```shell
./plugin -vgpu 10
```
