module github.com/pion/ion-cluster

go 1.15

// replace github.com/pion/webrtc/v3 => github.com/billylindeman/webrtc/v3 v3.1.0-tandem-2

//replace github.com/pion/ion-sfu => /Users/billy/Development/go/src/github.com/pion/ion-sfu
//replace github.com/pion/webrtc/v3 => /Users/billy/Development/go/src/github.com/pion/webrtc

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/bep/debounce v1.2.0
	github.com/coreos/etcd v3.3.25+incompatible // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/elliotchance/orderedmap v1.4.0
	github.com/gammazero/deque v0.1.0
	github.com/gammazero/workerpool v1.1.2
	github.com/getlantern/deepcopy v0.0.0-20160317154340-7f45deb8130a
	github.com/go-logr/logr v1.2.0
	github.com/go-logr/zerologr v1.2.1 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/koding/websocketproxy v0.0.0-20181220232114-7ed82d81a28c
	github.com/lucsky/cuid v1.2.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pborman/uuid v1.2.1
	github.com/pion/interceptor v0.1.7
	github.com/pion/ion-sfu v1.11.0
	github.com/pion/rtcp v1.2.9
	github.com/pion/rtp v1.7.4
	github.com/pion/sdp/v2 v2.4.0
	github.com/pion/sdp/v3 v3.0.4
	github.com/pion/transport v0.12.3
	github.com/pion/webrtc/v3 v3.1.7
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/rs/zerolog v1.26.1
	github.com/sourcegraph/jsonrpc2 v0.1.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.9.0
	github.com/stretchr/testify v1.7.0
	go.etcd.io/etcd v3.3.27+incompatible
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
	k8s.io/klog/v2 v2.20.0 // indirect
)
