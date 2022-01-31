package cluster

import (
	"sync"

	"github.com/getlantern/deepcopy"
	"github.com/pion/ion-cluster/pkg/sfu"
)

type Broadcast struct {
	method string
	params interface{}
}

type Session struct {
	mu               sync.Mutex
	presence         map[string]interface{}
	presenceRevision uint64

	broadcastListeners map[string]chan<- Broadcast

	SessionLocal
}

func NewSession(id string, dcs []*Datachannel, cfg WebRTCTransportConfig) Session {
	return Session{
		sync.Mutex{},
		make(map[string]interface{}),
		0,
		make(map[string]chan<- Broadcast),
		*NewSessionLocal(id, dcs, cfg).(*sfu.SessionLocal),
	}
}

func (s *Session) UpdatePresenceMetaForPeer(peerID string, meta interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.presenceRevision += 1
	if meta != nil {
		s.presence[peerID] = meta
	} else {
		delete(s.presence, peerID)
	}

	currentPresence := make(map[string]interface{})
	deepcopy.Copy(&currentPresence, s.presence)

	msg := Broadcast{
		method: "presence",
		params: Presence{
			Revision: s.presenceRevision,
			Meta:     currentPresence,
		},
	}

	s.Broadcast(msg)
}

func (s *Session) BroadcastAddListener(peerID string, ch chan<- Broadcast) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.broadcastListeners[peerID] = ch
}

func (s *Session) BroadcastRemoveListener(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.broadcastListeners, peerID)
}

func (s *Session) Broadcast(msg Broadcast) {
	for id, ch := range s.broadcastListeners {
		select {
		case ch <- msg:
			log.V(4).Info("wrote broadcast", "msg", msg)
		default:
			log.Error(nil, "couldn't write broadcast to channel, removing", "id", id)
		}
	}
}
