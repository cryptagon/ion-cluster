package rtc

import (
	"net"
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/ion-cluster/pkg/sfu/buffer"
	"github.com/pion/ion-sfu/pkg/stats"
	"github.com/pion/webrtc/v3"
)

// ICEServerConfig defines parameters for ice servers
type ICEServerConfig struct {
	URLs       []string `mapstructure:"urls"`
	Username   string   `mapstructure:"username"`
	Credential string   `mapstructure:"credential"`
}

type Candidates struct {
	IceLite    bool     `mapstructure:"icelite"`
	NAT1To1IPs []string `mapstructure:"nat1to1"`
}

// WebRTCTransportConfig represents Configuration options
type WebRTCTransportConfig struct {
	Configuration webrtc.Configuration
	Setting       webrtc.SettingEngine
	Router        RouterConfig
	BufferFactory *buffer.Factory
}

type WebRTCTimeoutsConfig struct {
	ICEDisconnectedTimeout int `mapstructure:"disconnected"`
	ICEFailedTimeout       int `mapstructure:"failed"`
	ICEKeepaliveInterval   int `mapstructure:"keepalive"`
}

// WebRTCConfig defines parameters for ice
type WebRTCConfig struct {
	ICESinglePort int                  `mapstructure:"singleport"`
	ICEPortRange  []uint16             `mapstructure:"portrange"`
	ICEServers    []ICEServerConfig    `mapstructure:"iceserver"`
	Candidates    Candidates           `mapstructure:"candidates"`
	SDPSemantics  string               `mapstructure:"sdpsemantics"`
	MDNS          bool                 `mapstructure:"mdns"`
	Timeouts      WebRTCTimeoutsConfig `mapstructure:"timeouts"`
}

type RouterConfig struct {
	WithStats           bool            `mapstructure:"withstats"`
	MaxBandwidth        uint64          `mapstructure:"maxbandwidth"`
	MaxPacketTrack      int             `mapstructure:"maxpackettrack"`
	AudioLevelInterval  int             `mapstructure:"audiolevelinterval"`
	AudioLevelThreshold uint8           `mapstructure:"audiolevelthreshold"`
	AudioLevelFilter    int             `mapstructure:"audiolevelfilter"`
	Simulcast           SimulcastConfig `mapstructure:"simulcast"`
}

type SimulcastConfig struct {
	BestQualityFirst    bool `mapstructure:"bestqualityfirst"`
	EnableTemporalLayer bool `mapstructure:"enabletemporallayer"`
}

// Config for base SFU
type SFUConfig struct {
	SFU struct {
		Ballast   int64 `mapstructure:"ballast"`
		WithStats bool  `mapstructure:"withstats"`
	} `mapstructure:"sfu"`
	WebRTC        WebRTCConfig `mapstructure:"webrtc"`
	Router        RouterConfig `mapstructure:"Router"`
	BufferFactory *buffer.Factory
	TurnAuth      func(username string, realm string, srcAddr net.Addr) ([]byte, bool)
}

type ReceiverConfig struct {
	PacketBufferSize int
	maxBitrate       uint64
}

type RTPHeaderExtensionConfig struct {
	Audio []string
	Video []string
}

type RTCPFeedbackConfig struct {
	Audio []webrtc.RTCPFeedback
	Video []webrtc.RTCPFeedback
}

type DirectionConfig struct {
	RTPHeaderExtension RTPHeaderExtensionConfig
	RTCPFeedback       RTCPFeedbackConfig
}

// NewWebRTCTransportConfig parses our settings and returns a usable WebRTCTransportConfig for creating PeerConnections
func NewWebRTCTransportConfig(c SFUConfig) WebRTCTransportConfig {
	se := webrtc.SettingEngine{}
	se.DisableMediaEngineCopy(true)

	if c.WebRTC.ICESinglePort != 0 {
		logger.Infow("Listen on ", "single-port", c.WebRTC.ICESinglePort)
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
		logger.Infow("No webrtc timeouts found in config, using default ones")
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
