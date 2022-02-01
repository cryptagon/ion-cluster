package rtc

import (
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/ion-cluster/pkg/types"
)

// wrapper around WebRTC receiver, overriding its ID

type WrappedReceiver struct {
	sfu.TrackReceiver
	trackID  types.TrackID
	streamId string
}

func NewWrappedReceiver(receiver sfu.TrackReceiver, trackID types.TrackID, streamId string) WrappedReceiver {
	return WrappedReceiver{
		TrackReceiver: receiver,
		trackID:       trackID,
		streamId:      streamId,
	}
}

func (r WrappedReceiver) TrackID() types.TrackID {
	return r.trackID
}

func (r WrappedReceiver) StreamID() string {
	return r.streamId
}
