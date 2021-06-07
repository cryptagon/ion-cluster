module github.com/pion/ion-cluster

go 1.15

//replace github.com/pion/ion-sfu => github.com/cryptagon/ion-sfu master-tandem
//replace github.com/pion/ion-sfu => github.com/cryptagon/ion-sfu v1.10.4-0.20210518234538-0fe209599f01

// replace github.com/pion/ion-sfu => /Users/billy/Development/go/src/github.com/pion/ion-sfu

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	cloud.google.com/go v0.77.0 // indirect
	github.com/coreos/etcd v3.3.25+incompatible
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/envoyproxy/go-control-plane v0.9.4 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/koding/websocketproxy v0.0.0-20181220232114-7ed82d81a28c
	github.com/lucsky/cuid v1.0.2
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pborman/uuid v1.2.1
	github.com/pion/interceptor v0.0.12
	github.com/pion/ion-log v1.0.1
	github.com/pion/ion-sfu v1.10.4
	github.com/pion/quic v0.1.4 // indirect
	github.com/pion/rtcp v1.2.6
	github.com/pion/sdp/v2 v2.4.0
	github.com/pion/srtp v1.5.2 // indirect
	github.com/pion/webrtc/v3 v3.0.29
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.15.0 // indirect
	github.com/sourcegraph/jsonrpc2 v0.0.0-20210201082850-366fbb520750
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.1
	go.etcd.io/etcd v3.3.25+incompatible // indirect
	google.golang.org/grpc v1.35.0
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
)
