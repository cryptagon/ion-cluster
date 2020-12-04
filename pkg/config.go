package cluster

import (
	"fmt"
	"strings"

	"github.com/dgrijalva/jwt-go"
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
	Auth     AuthConfig
}

//AuthConfig params for JWT token authentication
type AuthConfig struct {
	Enabled bool
	Key     string
	KeyType string
}

func (a AuthConfig) keyFunc(t *jwt.Token) (interface{}, error) {
	switch a.KeyType {
	//TODO: add more support for keytypes here
	default:
		return []byte(a.Key), nil
	}
}

type CoordinatorConfig struct {
	Local *struct {
		Enabled bool
	}
	Etcd *struct {
		Enabled bool
		Hosts   []string
	}
}
