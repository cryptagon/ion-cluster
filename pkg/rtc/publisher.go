package rtc

import (
	"sync"
	"sync/atomic"

	sfu "github.com/pion/ion-cluster/pkg/sfu"

	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type Publisher struct {
	mu  sync.RWMutex
	id  string
	pc  *webrtc.PeerConnection
	cfg *WebRTCTransportConfig

	router     Router
	session    ISession
	tracks     []PublisherTrack
	candidates []webrtc.ICECandidateInit

	onICEConnectionStateChangeHandler atomic.Value // func(webrtc.ICEConnectionState)
	onPublisherTrack                  atomic.Value // func(PublisherTrack)

	closeOnce sync.Once
}

type PublisherTrack struct {
	Track    *webrtc.TrackRemote
	Receiver sfu.TrackReceiver
}

// NewPublisher creates a new Publisher
func NewPublisher(id string, session ISession, cfg *WebRTCTransportConfig) (*Publisher, error) {
	me, err := getPublisherMediaEngine()
	if err != nil {
		logger.Errorw("NewPeer error", err, "peer_id", id)
		return nil, errPeerConnectionInitFailed
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(cfg.Setting))
	pc, err := api.NewPeerConnection(cfg.Configuration)

	if err != nil {
		logger.Errorw("NewPeer error", err, "peer_id", id)
		return nil, errPeerConnectionInitFailed
	}

	p := &Publisher{
		id:      id,
		pc:      pc,
		cfg:     cfg,
		router:  newRouter(id, session, cfg),
		session: session,
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		logger.Infow("Peer got remote track id",
			"peer_id", p.id,
			"track_id", track.ID(),
			"mediaSSRC", track.SSRC(),
			"rid", track.RID(),
			"stream_id", track.StreamID(),
		)

		r, pub := p.router.AddReceiver(receiver, track, track.ID(), track.StreamID())
		if pub {
			p.session.Publish(p.router, r)
			p.mu.Lock()
			publisherTrack := PublisherTrack{track, r}
			p.tracks = append(p.tracks, publisherTrack)
			//for _, rp := range p.relayPeers {
			//	if err = p.createRelayTrack(track, r, rp.peer); err != nil {
			//		logger.V(1).Error(err, "Creating relay track.", "peer_id", p.id)
			//	}
			//}
			p.mu.Unlock()
			if handler, ok := p.onPublisherTrack.Load().(func(PublisherTrack)); ok && handler != nil {
				handler(publisherTrack)
			}
		} else {
			p.mu.Lock()
			p.tracks = append(p.tracks, PublisherTrack{track, r})
			p.mu.Unlock()
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == APIChannelLabel {
			// terminate api data channel
			return
		}
		p.session.AddDatachannel(id, dc)
	})

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		logger.Infow("ice connection status", "state", connectionState)
		switch connectionState {
		case webrtc.ICEConnectionStateFailed:
			fallthrough
		case webrtc.ICEConnectionStateClosed:
			logger.Infow("webrtc ice closed", "peer_id", p.id)
			p.Close()
		}

		if handler, ok := p.onICEConnectionStateChangeHandler.Load().(func(webrtc.ICEConnectionState)); ok && handler != nil {
			handler(connectionState)
		}
	})

	p.router.SetRTCPWriter(p.pc.WriteRTCP)

	return p, nil
}

func (p *Publisher) Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return webrtc.SessionDescription{}, err
	}

	for _, c := range p.candidates {
		if err := p.pc.AddICECandidate(c); err != nil {
			logger.Errorw("Add publisher ice candidate to peer err", err, "peer_id", p.id)
		}
	}
	p.candidates = nil

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, err
	}
	return answer, nil
}

// GetRouter returns Router with mediaSSRC
func (p *Publisher) GetRouter() Router {
	return p.router
}

// Close peer
func (p *Publisher) Close() {
	p.closeOnce.Do(func() {
		p.router.Stop()
		if err := p.pc.Close(); err != nil {
			logger.Errorw("webrtc transport close err", err)
		}
	})
}

func (p *Publisher) OnPublisherTrack(f func(track PublisherTrack)) {
	p.onPublisherTrack.Store(f)
}

// OnICECandidate handler
func (p *Publisher) OnICECandidate(f func(c *webrtc.ICECandidate)) {
	p.pc.OnICECandidate(f)
}

func (p *Publisher) OnICEConnectionStateChange(f func(connectionState webrtc.ICEConnectionState)) {
	p.onICEConnectionStateChangeHandler.Store(f)
}

func (p *Publisher) SignalingState() webrtc.SignalingState {
	return p.pc.SignalingState()
}

func (p *Publisher) PeerConnection() *webrtc.PeerConnection {
	return p.pc
}

func (p *Publisher) PublisherTracks() []PublisherTrack {
	p.mu.Lock()
	defer p.mu.Unlock()

	tracks := make([]PublisherTrack, len(p.tracks))
	for idx, track := range p.tracks {
		tracks[idx] = track
	}
	return tracks
}

func (p *Publisher) Tracks() []*webrtc.TrackRemote {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tracks := make([]*webrtc.TrackRemote, len(p.tracks))
	for idx, track := range p.tracks {
		tracks[idx] = track.Track
	}
	return tracks
}

// AddICECandidate to peer connection
func (p *Publisher) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if p.pc.RemoteDescription() != nil {
		return p.pc.AddICECandidate(candidate)
	}
	p.candidates = append(p.candidates, candidate)
	return nil
}