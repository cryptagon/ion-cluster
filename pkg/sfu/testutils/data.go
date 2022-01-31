package testutils

import (
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"

	"github.com/pion/ion-cluster/pkg/sfu/buffer"
)

// -----------------------------------------------------------

type TestExtPacketParams struct {
	SetMarker      bool
	SetPadding     bool
	IsHead         bool
	IsKeyFrame     bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	PayloadSize    int
	PaddingSize    int
	ArrivalTime    int64
}

// -----------------------------------------------------------

func GetTestExtPacket(params *TestExtPacketParams) (*buffer.ExtPacket, error) {
	packet := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        params.SetPadding,
			Marker:         params.SetMarker,
			PayloadType:    params.PayloadType,
			SequenceNumber: params.SequenceNumber,
			Timestamp:      params.Timestamp,
			SSRC:           params.SSRC,
		},
		Payload: make([]byte, params.PayloadSize),
		// LK-TODO need a newer version of pion/rtp PaddingSize: params.PaddingSize,
	}

	raw, err := packet.Marshal()
	if err != nil {
		return nil, err
	}

	ep := &buffer.ExtPacket{
		Head:      params.IsHead,
		Arrival:   params.ArrivalTime,
		Packet:    packet,
		KeyFrame:  params.IsKeyFrame,
		RawPacket: raw,
	}

	return ep, nil
}

// --------------------------------------

func GetTestExtPacketVP8(params *TestExtPacketParams, vp8 *buffer.VP8) (*buffer.ExtPacket, error) {
	ep, err := GetTestExtPacket(params)
	if err != nil {
		return nil, err
	}

	ep.KeyFrame = vp8.IsKeyFrame
	ep.Payload = *vp8
	return ep, nil
}

// --------------------------------------

var TestVP8Codec = webrtc.RTPCodecCapability{
	MimeType:  "video/vp8",
	ClockRate: 90000,
}

var TestOpusCodec = webrtc.RTPCodecCapability{
	MimeType:  "audio/opus",
	ClockRate: 48000,
}

// --------------------------------------
