package types

import (
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ParticipantID string
type TrackID string

type StreamType int32

const (
	StreamType_UPSTREAM   StreamType = 0
	StreamType_DOWNSTREAM StreamType = 1
)

type AnalyticsStat struct {
	AnalyticsKey    string                 `protobuf:"bytes,1,opt,name=analytics_key,json=analyticsKey,proto3" json:"analytics_key,omitempty"`
	Kind            StreamType             `protobuf:"varint,2,opt,name=kind,proto3,enum=types.StreamType" json:"kind,omitempty"`
	TimeStamp       *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=time_stamp,json=timeStamp,proto3" json:"time_stamp,omitempty"`
	Node            string                 `protobuf:"bytes,4,opt,name=node,proto3" json:"node,omitempty"`
	RoomId          string                 `protobuf:"bytes,5,opt,name=room_id,json=roomId,proto3" json:"room_id,omitempty"`
	ParticipantId   string                 `protobuf:"bytes,6,opt,name=participant_id,json=participantId,proto3" json:"participant_id,omitempty"`
	Jitter          float64                `protobuf:"fixed64,7,opt,name=jitter,proto3" json:"jitter,omitempty"`
	TotalPackets    uint64                 `protobuf:"varint,8,opt,name=total_packets,json=totalPackets,proto3" json:"total_packets,omitempty"`
	PacketLost      uint64                 `protobuf:"varint,9,opt,name=packet_lost,json=packetLost,proto3" json:"packet_lost,omitempty"`
	Delay           uint64                 `protobuf:"varint,10,opt,name=delay,proto3" json:"delay,omitempty"`
	TotalBytes      uint64                 `protobuf:"varint,11,opt,name=total_bytes,json=totalBytes,proto3" json:"total_bytes,omitempty"`
	NackCount       int32                  `protobuf:"varint,12,opt,name=nack_count,json=nackCount,proto3" json:"nack_count,omitempty"`
	PliCount        int32                  `protobuf:"varint,13,opt,name=pli_count,json=pliCount,proto3" json:"pli_count,omitempty"`
	FirCount        int32                  `protobuf:"varint,14,opt,name=fir_count,json=firCount,proto3" json:"fir_count,omitempty"`
	RoomName        string                 `protobuf:"bytes,15,opt,name=room_name,json=roomName,proto3" json:"room_name,omitempty"`
	ConnectionScore float32                `protobuf:"fixed32,16,opt,name=connection_score,json=connectionScore,proto3" json:"connection_score,omitempty"`
	TrackId         string                 `protobuf:"bytes,17,opt,name=track_id,json=trackId,proto3" json:"track_id,omitempty"`
	Rtt             uint32                 `protobuf:"varint,18,opt,name=rtt,proto3" json:"rtt,omitempty"`
}
type ConnectionQuality int32

const (
	ConnectionQuality_POOR      ConnectionQuality = 0
	ConnectionQuality_GOOD      ConnectionQuality = 1
	ConnectionQuality_EXCELLENT ConnectionQuality = 2
)

type TrackSource int32

const (
	TrackSource_UNKNOWN            TrackSource = 0
	TrackSource_CAMERA             TrackSource = 1
	TrackSource_MICROPHONE         TrackSource = 2
	TrackSource_SCREEN_SHARE       TrackSource = 3
	TrackSource_SCREEN_SHARE_AUDIO TrackSource = 4
)
