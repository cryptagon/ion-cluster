package cluster

import (
	sfu "github.com/pion/ion-sfu/pkg"
)

// RootConfig is the root config read in from config.toml
type RootConfig struct {
	Signal      SignalConfig
	SFU         sfu.Config
	Coordinator Coordinator
}

// SignalConfig params for http listener / grpc / websocket server
type SignalConfig struct {
	Key      string
	Cert     string
	HTTPAddr string
	GRPCAddr string
}

type CoordinatorConfig struct {
	Mem *struct {
		Enabled bool
	}
	Etcd *struct {
		Enabled bool
		Host    string
	}
	Redis *struct {
		Enabled bool
		Host    string
	}
}
