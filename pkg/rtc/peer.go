package rtc

import (
	"errors"
	"fmt"
	"sync"

	"github.com/lucsky/cuid"
	"github.com/pion/ion-cluster/pkg/logger"
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/ion-sfu/pkg/twcc"
	"github.com/pion/rtcp"

	"github.com/pion/webrtc/v3"
)

type PeerID string
type TransportDirection int

const (
	TransportDirectionPublisher  = 0
	TransportDirectionSubscriber = 1
)

var (
	// ErrTransportExists join is called after a peerconnection is established
	ErrTransportExists = errors.New("rtc transport already exists for this connection")
	// ErrNoTransportEstablished cannot signal before join
	ErrNoTransportEstablished = errors.New("no rtc transport exists for this Peer")
	// ErrOfferIgnored if offer received in unstable state
	ErrOfferIgnored = errors.New("offered ignored")
)

type Peer interface {
	ID() PeerID
	// Session() ISession
	// Publisher() *PCTransport
	// Subscriber() *PCTransport
	Close() error
	SendDCMessage(label string, msg []byte) error
}

// JoinConfig allow adding more control to the peers joining a SessionLocal.
type JoinConfig struct {
	// If true the peer will not be allowed to publish tracks to SessionLocal.
	NoPublish bool
	// If true the peer will not be allowed to subscribe to other peers in SessionLocal.
	NoSubscribe bool
	// If true the peer will not automatically subscribe all tracks,
	// and then the peer can use peer.Subscriber().AddDownTrack/RemoveDownTrack
	// to customize the subscrbe stream combination as needed.
	// this parameter depends on NoSubscribe=false.
	NoAutoSubscribe bool
}

// SessionProvider provides the SessionLocal to the sfu.Peer
// This allows the sfu.SFU{} implementation to be customized / wrapped by another package
type SessionProvider interface {
	GetSession(sid SessionID) (ISession, WebRTCTransportConfig)
}

type ChannelAPIMessage struct {
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// PeerLocal represents a pair peer connection
type PeerLocal struct {
	sync.Mutex
	id       PeerID
	isClosed atomicBool

	session  ISession
	provider SessionProvider

	rtcpCh chan []rtcp.Packet

	// hold reference for MediaTrack
	twcc *twcc.Responder

	publisher  *Publisher
	subscriber *Subscriber

	publishedTracks  []PublishedTrack
	subscribedTracks []sfu.DownTrack

	apiDC *webrtc.DataChannel

	SignalOnOffer        func(*webrtc.SessionDescription)
	SignalOnIceCandidate func(*webrtc.ICECandidateInit, int)

	OnICEConnectionStateChange func(webrtc.ICEConnectionState)

	remoteAnswerPending bool
	negotiationPending  bool
}

// NewPeer creates a new PeerLocal for signaling with the given SFU
func NewPeer(provider SessionProvider) *PeerLocal {
	return &PeerLocal{
		provider: provider,
	}
}

// Join initializes this peer for a given sessionID
func (p *PeerLocal) Join(sid SessionID, uid PeerID, config ...JoinConfig) error {
	var conf JoinConfig
	if len(config) > 0 {
		conf = config[0]
	}

	if p.session != nil {
		logger.Infow("peer already exists", "session_id", sid, "peer_id", p.id, "publisher_id", p.publisher.id)
		return ErrTransportExists
	}

	if uid == "" {
		uid = PeerID(cuid.New())
	}
	p.id = uid
	var err error

	s, cfg := p.provider.GetSession(sid)
	p.session = s

	if !conf.NoSubscribe {
		p.subscriber, err = NewSubscriber(uid, cfg)
		if err != nil {
			return fmt.Errorf("error creating transport: %v", err)
		}

		p.subscriber.noAutoSubscribe = conf.NoAutoSubscribe

		p.subscriber.OnNegotiationNeeded(func() {
			p.Lock()
			defer p.Unlock()

			if p.remoteAnswerPending {
				p.negotiationPending = true
				return
			}

			logger.Infow("Negotiation needed", "peer_id", p.id)
			offer, err := p.subscriber.CreateOffer()
			if err != nil {
				logger.Errorw("CreateOffer error", err)
				return
			}

			p.remoteAnswerPending = true
			if p.SignalOnOffer != nil && !p.isClosed.get() {
				logger.Infow("Send offer", "peer_id", p.id)
				p.SignalOnOffer(&offer)
			}
		})

		p.subscriber.OnICECandidate(func(c *webrtc.ICECandidate) {
			logger.Infow("On subscriber ice candidate called for peer", "peer_id", p.id)
			if c == nil {
				return
			}

			if p.SignalOnIceCandidate != nil && !p.isClosed.get() {
				json := c.ToJSON()
				p.SignalOnIceCandidate(&json, TransportDirectionSubscriber)
			}
		})
	}

	if !conf.NoPublish {
		p.publisher, err = NewPublisher(uid, &cfg)
		if err != nil {
			return fmt.Errorf("error creating transport: %v", err)
		}

		if !conf.NoSubscribe {
			p.apiDC, err = p.subscriber.pc.CreateDataChannel(APIChannelLabel, &webrtc.DataChannelInit{})
			if err != nil {
				return err
			}

		}

		p.publisher.OnICECandidate(func(c *webrtc.ICECandidate) {
			logger.Infow("on publisher ice candidate called for peer", "peer_id", p.id)
			if c == nil {
				return
			}

			if p.SignalOnIceCandidate != nil && !p.isClosed.get() {
				json := c.ToJSON()
				p.SignalOnIceCandidate(&json, TransportDirectionPublisher)
			}
		})

		p.publisher.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
			if p.OnICEConnectionStateChange != nil && !p.isClosed.get() {
				p.OnICEConnectionStateChange(s)
			}
		})
	}

	p.session.AddPeer(p)

	// logger.Infow("PeerLocal join SessionLocal", "peer_id", p.id, "session_id", sid)

	// if !conf.NoSubscribe {
	// 	p.session.Subscribe(p)
	// }
	return nil
}

// Answer an offer from remote
func (p *PeerLocal) SignalAnswer(sdp webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if p.publisher == nil {
		return nil, ErrNoTransportEstablished
	}

	logger.Infow("PeerLocal got offer", "peer_id", p.id)

	if p.publisher.SignalingState() != webrtc.SignalingStateStable {
		return nil, ErrOfferIgnored
	}

	answer, err := p.publisher.Answer(sdp)
	if err != nil {
		return nil, fmt.Errorf("error creating answer: %v", err)
	}

	logger.Infow("PeerLocal send answer", "peer_id", p.id)

	return &answer, nil
}

// SetRemoteDescription when receiving an answer from remote
func (p *PeerLocal) SignalSetRemoteDescription(sdp webrtc.SessionDescription) error {
	if p.subscriber == nil {
		return ErrNoTransportEstablished
	}
	p.Lock()
	defer p.Unlock()

	logger.Infow("PeerLocal got answer", "peer_id", p.id)
	if err := p.subscriber.SetRemoteDescription(sdp); err != nil {
		return fmt.Errorf("setting remote description: %w", err)
	}

	p.remoteAnswerPending = false

	if p.negotiationPending {
		p.negotiationPending = false
		p.subscriber.negotiate()
	}

	return nil
}

// Trickle candidates available for this peer
func (p *PeerLocal) SignalTrickle(candidate webrtc.ICECandidateInit, target TransportDirection) error {
	if p.subscriber == nil || p.publisher == nil {
		return ErrNoTransportEstablished
	}
	logger.Infow("PeerLocal trickle", "peer_id", p.id)
	switch target {
	case TransportDirectionPublisher:
		if err := p.publisher.AddICECandidate(candidate); err != nil {
			return fmt.Errorf("setting ice candidate: %w", err)
		}
	case TransportDirectionSubscriber:
		if err := p.subscriber.AddICECandidate(candidate); err != nil {
			return fmt.Errorf("setting ice candidate: %w", err)
		}
	}
	return nil
}

func (p *PeerLocal) SendDCMessage(label string, msg []byte) error {
	if p.subscriber == nil {
		return fmt.Errorf("no subscriber for this peer")
	}
	if p.apiDC == nil {
		return fmt.Errorf("data channel %s doesn't exist", label)
	}

	if err := p.apiDC.SendText(string(msg)); err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	return nil
}

// Close shuts down the peer connection and sends true to the done channel
func (p *PeerLocal) Close() error {
	p.Lock()
	defer p.Unlock()

	if !p.isClosed.set(true) {
		return nil
	}

	if p.session != nil {
		p.session.RemovePeer(p)
	}
	if p.publisher != nil {
		p.publisher.Close()
	}
	if p.subscriber != nil {
		if err := p.subscriber.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (p *PeerLocal) Subscriber() *Subscriber {
	return p.subscriber
}

func (p *PeerLocal) Publisher() *Publisher {
	return p.publisher
}

func (p *PeerLocal) Session() ISession {
	return p.session
}

// ID return the peer id
func (p *PeerLocal) ID() PeerID {
	return p.id
}
