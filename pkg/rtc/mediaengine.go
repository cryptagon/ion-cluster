package rtc

import (
	"strings"

	"github.com/pion/ion-cluster/pkg/types"
	"github.com/pion/webrtc/v3"
)

func registerCodecs(me *webrtc.MediaEngine, codecs []*types.Codec, rtcpFeedback RTCPFeedbackConfig) error {
	opusCodec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1", RTCPFeedback: rtcpFeedback.Audio}
	if isCodecEnabled(codecs, opusCodec) {
		if err := me.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: opusCodec,
			PayloadType:        111,
		}, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000, RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000, SDPFmtpLine: "profile-id=0", RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        98,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000, SDPFmtpLine: "profile-id=1", RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        100,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f", RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        125,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f", RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        108,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032", RTCPFeedback: rtcpFeedback.Video},
			PayloadType:        123,
		},
	} {
		if isCodecEnabled(codecs, codec.RTPCodecCapability) {
			if err := me.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
				return err
			}
		}
	}
	return nil
}

func registerHeaderExtensions(me *webrtc.MediaEngine, rtpHeaderExtension RTPHeaderExtensionConfig) error {
	for _, extension := range rtpHeaderExtension.Video {
		if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: extension}, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	for _, extension := range rtpHeaderExtension.Audio {
		if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: extension}, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	return nil
}

func createMediaEngine(codecs []*types.Codec, config DirectionConfig) (*webrtc.MediaEngine, error) {
	me := &webrtc.MediaEngine{}
	if err := registerCodecs(me, codecs, config.RTCPFeedback); err != nil {
		return nil, err
	}

	if err := registerHeaderExtensions(me, config.RTPHeaderExtension); err != nil {
		return nil, err
	}

	return me, nil
}

func isCodecEnabled(codecs []*types.Codec, cap webrtc.RTPCodecCapability) bool {
	for _, codec := range codecs {
		if !strings.EqualFold(codec.Mime, cap.MimeType) {
			continue
		}
		if codec.FmtpLine == "" || strings.EqualFold(codec.FmtpLine, cap.SDPFmtpLine) {
			return true
		}
	}
	return false
}
