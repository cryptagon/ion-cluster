package cluster

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/pion/ice/v2"
	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/ion-cluster/pkg/rtc"
	"github.com/pion/ion-sfu/pkg/stats"
	"github.com/pion/webrtc/v3"
)

var log = logger.GetLogger().WithName("cluster")

func init() {
	// logr.SetGlobalOptions(logr.GlobalConfig{V: 1})
	// sfu.Logger = log.WithName("sfu")
}

// RootConfig is the root config read in from config.toml
type RootConfig struct {
	Signal      SignalConfig
	SFU         rtc.SFUConfig
	Coordinator CoordinatorConfig
}

type CongestionControlConfig struct {
	Enabled    bool `yaml:"enabled"`
	AllowPause bool `yaml:"allow_pause"`
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

//CoordinatorConfig params for which coordinator to use
type CoordinatorConfig struct {
	Local *struct {
		Enabled bool
	}
	Etcd *struct {
		Enabled bool
		Hosts   []string
	}
}

// NewWebRTCTransportConfig parses our settings and returns a usable WebRTCTransportConfig for creating PeerConnections
func NewWebRTCTransportConfig(c rtc.SFUConfig) rtc.WebRTCTransportConfig {
	se := webrtc.SettingEngine{}
	se.DisableMediaEngineCopy(true)

	if c.WebRTC.ICESinglePort != 0 {
		log.Info("Listen on ", "single-port", c.WebRTC.ICESinglePort)
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: c.WebRTC.ICESinglePort,
		})
		if err != nil {
			panic(err)
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))
	} else {
		var icePortStart, icePortEnd uint16

		if len(c.WebRTC.ICEPortRange) == 2 {
			icePortStart = c.WebRTC.ICEPortRange[0]
			icePortEnd = c.WebRTC.ICEPortRange[1]
		}
		if icePortStart != 0 || icePortEnd != 0 {
			if err := se.SetEphemeralUDPPortRange(icePortStart, icePortEnd); err != nil {
				panic(err)
			}
		}
	}

	var iceServers []webrtc.ICEServer
	if c.WebRTC.Candidates.IceLite {
		se.SetLite(c.WebRTC.Candidates.IceLite)
	} else {
		for _, iceServer := range c.WebRTC.ICEServers {
			s := webrtc.ICEServer{
				URLs:       iceServer.URLs,
				Username:   iceServer.Username,
				Credential: iceServer.Credential,
			}
			iceServers = append(iceServers, s)
		}
	}

	se.BufferFactory = c.BufferFactory.GetOrNew

	sdpSemantics := webrtc.SDPSemanticsUnifiedPlan
	switch c.WebRTC.SDPSemantics {
	case "unified-plan-with-fallback":
		sdpSemantics = webrtc.SDPSemanticsUnifiedPlanWithFallback
	case "plan-b":
		sdpSemantics = webrtc.SDPSemanticsPlanB
	}

	if c.WebRTC.Timeouts.ICEDisconnectedTimeout == 0 &&
		c.WebRTC.Timeouts.ICEFailedTimeout == 0 &&
		c.WebRTC.Timeouts.ICEKeepaliveInterval == 0 {
		log.Info("No webrtc timeouts found in config, using default ones")
	} else {
		se.SetICETimeouts(
			time.Duration(c.WebRTC.Timeouts.ICEDisconnectedTimeout)*time.Second,
			time.Duration(c.WebRTC.Timeouts.ICEFailedTimeout)*time.Second,
			time.Duration(c.WebRTC.Timeouts.ICEKeepaliveInterval)*time.Second,
		)
	}

	w := WebRTCTransportConfig{
		Configuration: webrtc.Configuration{
			ICEServers:   iceServers,
			SDPSemantics: sdpSemantics,
		},
		Setting:       se,
		Router:        c.Router,
		BufferFactory: c.BufferFactory,
	}

	if len(c.WebRTC.Candidates.NAT1To1IPs) > 0 {
		w.Setting.SetNAT1To1IPs(c.WebRTC.Candidates.NAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	if !c.WebRTC.MDNS {
		w.Setting.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	}

	if c.SFU.WithStats {
		w.Router.WithStats = true
		stats.InitStats()
	}

	return w
}
