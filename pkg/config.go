package cluster

import (
	"fmt"
	"strings"

	sfu "github.com/pion/ion-sfu/pkg"
)

// RootConfig is the root config read in from config.toml
type RootConfig struct {
	Signal      SignalConfig
	SFU         sfu.Config
	Coordinator CoordinatorConfig
}

// Endpoint public endpoint to hit
func (c *RootConfig) Endpoint() string {
	port := strings.Split(c.Signal.HTTPAddr, ":")[1]

	if c.Signal.Key != "" && c.Signal.Cert != "" {
		return fmt.Sprintf("wss://%v:%v/ws", c.Signal.FQDN, port)
	}
	return fmt.Sprintf("ws://%v:%v/ws", c.Signal.FQDN, port)
}

// SignalConfig params for http listener / grpc / websocket server
type SignalConfig struct {
	FQDN     string
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
}
