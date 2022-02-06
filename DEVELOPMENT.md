## Build with Docker

```shell
$ git clone https://github.com/kuartis/kuartis-virtual-gpu-device-plugin.git && cd kuartis-virtual-gpu-device-plugin
$ docker build -t kuartis/kuartis-virtual-gpu-device-plugin:v0.2.0 .
```

#### Run locally

```shell
$ docker run --security-opt=no-new-privileges --cap-drop=ALL --ipc=host --network=none -it -v /var/lib/kubelet/device-plugins:/var/lib/kubelet/device-plugins kuartis/kuartis-virtual-gpu-device-plugin:v0.1.0
```

#### Deploy as Daemon Set:

```shell
$ kubectl create -f vgpu-device-plugin.yml
```

## Build without Docker

### Build

```shell
$ export CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files'
$ go build -ldflags="-s -w" -o plugin
```

### Run locally

```shell
$ ./plugin -vgpu 10
```
