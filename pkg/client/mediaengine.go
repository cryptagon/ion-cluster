package client

import (
	"github.com/pion/sdp/v2"
	"github.com/pion/webrtc/v3"
)

const (
	mimeTypeH264 = "video/h264"
	mimeTypeOpus = "audio/opus"
)

func getProducerMediaEngine() (*webrtc.MediaEngine, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: mimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: mimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f", RTCPFeedback: videoRTCPFeedback},
			PayloadType:        102,
		},
	} {
		if err := me.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
	}

	for _, extension := range []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		sdp.TransportCCURI,
	} {
		if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: extension}, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
		if extension == sdp.TransportCCURI {
			continue
		}
		if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: extension}, webrtc.RTPCodecTypeAudio); err != nil {
			return nil, err
		}
	}

	return me, nil
}
