module github.com/pion/ion-cluster

go 1.15

//replace github.com/pion/ion-sfu => github.com/billylindeman/ion-sfu  master-tandem
replace github.com/pion/ion-sfu => github.com/billylindeman/ion-sfu v1.0.4-0.20201109212923-521fa5fe66e5

//replace github.com/pion/ion-sfu => /Users/billy/Development/go/src/github.com/pion/ion-sfu

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/coreos/etcd v3.3.25+incompatible
	github.com/envoyproxy/go-control-plane v0.9.4 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/koding/websocketproxy v0.0.0-20181220232114-7ed82d81a28c
	github.com/lucsky/cuid v1.0.2 // indirect
	github.com/pborman/uuid v1.2.1
	github.com/pion/ion-log v1.0.0
	github.com/pion/ion-sfu v1.0.28
	github.com/pion/sdp/v2 v2.4.0 // indirect
	github.com/pion/webrtc/v3 v3.0.0-beta.11
	github.com/prometheus/client_golang v1.8.0
	github.com/sourcegraph/jsonrpc2 v0.0.0-20200429184054-15c2290dcb37
	github.com/spf13/cobra v1.1.1 // indirect
	github.com/spf13/viper v1.7.1
	go.etcd.io/etcd v3.3.25+incompatible // indirect
	google.golang.org/grpc v1.33.1
	google.golang.org/grpc/examples v0.0.0-20201022203757-eb7fc22e4562 // indirect
	gopkg.in/ini.v1 v1.51.1 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)
