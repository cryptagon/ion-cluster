package config

import "time"

type RTCConfig struct {
	UDPPort           uint32 `yaml:"udp_port,omitempty"`
	TCPPort           uint32 `yaml:"tcp_port,omitempty"`
	ICEPortRangeStart uint32 `yaml:"port_range_start,omitempty"`
	ICEPortRangeEnd   uint32 `yaml:"port_range_end,omitempty"`
	NodeIP            string `yaml:"node_ip,omitempty"`
	// for testing, disable UDP
	ForceTCP      bool     `yaml:"force_tcp,omitempty"`
	StunServers   []string `yaml:"stun_servers,omitempty"`
	UseExternalIP bool     `yaml:"use_external_ip"`

	// Number of packets to buffer for NACK
	PacketBufferSize int `yaml:"packet_buffer_size,omitempty"`

	// Max bitrate for REMB
	MaxBitrate uint64 `yaml:"max_bitrate,omitempty"`

	// Throttle periods for pli/fir rtcp packets
	PLIThrottle PLIThrottleConfig `yaml:"pli_throttle,omitempty"`

	// Which side runs bandwidth estimation
	UseSendSideBWE bool `yaml:"send_side_bandwidth_estimation,omitempty"`

	CongestionControl CongestionControlConfig `yaml:"congestion_control,omitempty"`
}

type PLIThrottleConfig struct {
	LowQuality  time.Duration `yaml:"low_quality,omitempty"`
	MidQuality  time.Duration `yaml:"mid_quality,omitempty"`
	HighQuality time.Duration `yaml:"high_quality,omitempty"`
}

type CongestionControlConfig struct {
	Enabled    bool `yaml:"enabled"`
	AllowPause bool `yaml:"allow_pause"`
}

type AudioConfig struct {
	// minimum level to be considered active, 0-127, where 0 is loudest
	ActiveLevel uint8 `yaml:"active_level"`
	// percentile to measure, a participant is considered active if it has exceeded the ActiveLevel more than
	// MinPercentile% of the time
	MinPercentile uint8 `yaml:"min_percentile"`
	// interval to update clients, in ms
	UpdateInterval uint32 `yaml:"update_interval"`
	// smoothing for audioLevel values sent to the client.
	// audioLevel will be an average of `smooth_intervals`, 0 to disable
	SmoothIntervals uint32 `yaml:"smooth_intervals"`
}
