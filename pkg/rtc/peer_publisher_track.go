package rtc

import (
	"sync"

	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/webrtc/v3"
)

type PublishedTrack struct {
	peer     *Peer
	track    *webrtc.TrackRemote
	receiver sfu.TrackReceiver

	subscriptionsLock sync.RWMutex
	subscriptions     map[PeerID]*SubscribedTrack
}

func NewPublishedTrack(peer *Peer, track *webrtc.TrackRemote) *PublishedTrack {
	return &PublishedTrack{
		peer:          peer,
		track:         track,
		subscriptions: make(map[PeerID]*SubscribedTrack, 0),
	}
}

func (t *PublishedTrack) AddSubscriber(peer PeerID) *SubscribedTrack {
	t.subscriptionsLock.Lock()
	defer t.subscriptionsLock.Unlock()

	st := NewSubscribedTrack(peer *Peer, trackID trackID, )

}
