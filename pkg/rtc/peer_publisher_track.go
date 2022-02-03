package rtc

import (
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/webrtc/v3"
)

type PublishedTrack struct {
	peer     *Peer
	track    *webrtc.TrackRemote
	receiver sfu.TrackReceiver

	subscriptions []*SubscribedTrack
}

func NewPublishedTrack(peer *Peer, track *webrtc.TrackRemote) *PublishedTrack {
	return &PublishedTrack{
		peer:          peer,
		track:         track,
		subscriptions: make([]*SubscribedTrack, 0),
	}
}
