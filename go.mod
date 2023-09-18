module github.com/youscan/virtual-gpu-device-plugin

go 1.17

require (
	github.com/NVIDIA/go-nvml v0.11.6-0
	github.com/fsnotify/fsnotify v1.5.1
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f
	google.golang.org/grpc v1.45.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubelet v0.0.0
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	golang.org/x/sys v0.0.0-20220318055525-2edf467146b5 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220317150908-0efb43f6373e // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.20.13
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.13
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.13
	k8s.io/apiserver => k8s.io/apiserver v0.20.13
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.13
	k8s.io/client-go => k8s.io/client-go v0.20.13
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.13
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.13
	k8s.io/code-generator => k8s.io/code-generator v0.20.13
	k8s.io/component-base => k8s.io/component-base v0.20.13
	k8s.io/cri-api => k8s.io/cri-api v0.20.13
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.13
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.13
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.13
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.13
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.13
	k8s.io/kubectl => k8s.io/kubectl v0.20.13
	k8s.io/kubelet => k8s.io/kubelet v0.20.13
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.13
	k8s.io/metrics => k8s.io/metrics v0.20.13
	k8s.io/node-api => k8s.io/node-api v0.20.13
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.13
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.20.13
	k8s.io/sample-controller => k8s.io/sample-controller v0.20.13
	k8s.io/utils => k8s.io/utils v0.0.0-20201110183641-67b214c5f920
)
