package client

import (
	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
)

const (
	rolePublish   int = 0
	roleSubscribe int = 1
)

type transport struct {
	role       int
	api        *webrtc.DataChannel
	pc         *webrtc.PeerConnection
	signal     Signal
	candidates []*webrtc.ICECandidateInit
}

func newTransport(role int, signal Signal, cfg *webrtc.Configuration) (*transport, error) {
	me, _ := getProducerMediaEngine()
	se := webrtc.SettingEngine{}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(*cfg)
	if err != nil {
		return nil, err
	}

	t := &transport{
		role:       role,
		signal:     signal,
		candidates: []*webrtc.ICECandidateInit{},
		pc:         pc,
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			trickle := candidate.ToJSON()
			signal.Trickle(role, &trickle)
		}
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		log.Debugf("transport got datachannel: %v", channel.Label())
		//todo handle api / remoteStream
		t.api = channel
	})

	if role == rolePublish {
		t.api, _ = pc.CreateDataChannel("ion-sfu", nil)
	}

	return t, nil
}

// Client for ion-cluster
type Client struct {
	signal Signal

	pub *transport
	sub *transport

	OnTrack func(*webrtc.TrackRemote, *webrtc.RTPReceiver)
}

//NewClient returns a new jsonrpc2 client that manages a pub and sub peerConnection
func NewClient(signal Signal, cfg *webrtc.Configuration) (*Client, error) {
	pub, err := newTransport(rolePublish, signal, cfg)
	if err != nil {
		return nil, err
	}
	sub, err := newTransport(roleSubscribe, signal, cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		signal: signal,
		pub:    pub,
		sub:    sub,
	}, nil
}

//Join a session
func (c *Client) Join(sid string) error {
	c.signal.OnNegotiate(c.signalOnNegotiate)
	c.signal.OnTrickle(c.signalOnTrickle)

	c.sub.pc.OnTrack(func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		log.Debugf("client sub got remote stream %v track %v", track.Msid(), track.ID())
		if c.OnTrack != nil {
			c.OnTrack(track, recv)
		}
	})

	// Setup Pub PC
	offer, err := c.pub.pc.CreateOffer(nil)
	if err != nil {
		log.Errorf("client join could not create pub offer: %v", err)
		return err
	}
	if err := c.pub.pc.SetLocalDescription(offer); err != nil {
		log.Errorf("client join pub couldn't SetLocalDescription %v", err)
		return err
	}

	answer, err := c.signal.Join(sid, &offer)
	if err != nil {
		log.Errorf("client join signal error: %v", err)
		return err
	}
	if err := c.pub.pc.SetRemoteDescription(*answer); err != nil {
		log.Errorf("client join pub couldn't SetRemoteDescription %v", err)
		return err
	}

	for _, candidate := range c.pub.candidates {
		c.pub.pc.AddICECandidate(*candidate)
	}
	c.pub.pc.OnNegotiationNeeded(c.pubNegotiationNeeded)

	return nil
}

// Publish takes a producer and publishes its data to the peer connection
func (c *Client) Publish(p Producer) error {
	videoSender, err := c.pub.pc.AddTrack(p.VideoTrack())
	if err != nil {
		return err
	}
	audioSender, err := c.pub.pc.AddTrack(p.AudioTrack())
	if err != nil {
		return err

	}
	defer c.pubNegotiationNeeded()

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := videoSender.Read(rtcpBuf); rtcpErr != nil {
				log.Errorf("videoSender rtcp error: %v", err)
				return
			}
		}
	}()

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := audioSender.Read(rtcpBuf); rtcpErr != nil {
				log.Errorf("audioSender rtcp error: %v", err)
				return
			}
		}
	}()

	go p.Start()
	return nil
}

// Pub PC re-negotiation
func (c *Client) pubNegotiationNeeded() {
	log.Debugf("client pubOnNegotiationNeeded")
	offer, err := c.pub.pc.CreateOffer(nil)
	if err != nil {
		log.Errorf("pub could not create pub offer: %v", err)
		return
	}
	if err := c.pub.pc.SetLocalDescription(offer); err != nil {
		log.Errorf("pub couldn't SetLocalDescription %v", err)
		return
	}

	answer, err := c.signal.Offer(&offer)
	if err != nil {
		log.Errorf("pub signal error: %v", err)
		return
	}
	if err := c.pub.pc.SetRemoteDescription(*answer); err != nil {
		log.Errorf("pub couldn't SetRemoteDescription %v", err)
		return
	}

	log.Debugf("client negotiated")
}

// signalOnNegotiate is triggered from server for the sub pc
func (c *Client) signalOnNegotiate(desc *webrtc.SessionDescription) {
	if err := c.sub.pc.SetRemoteDescription(*desc); err != nil {
		log.Errorf("sub couldn't SetRemoteDescription: %v", err)
		return
	}

	for _, candidate := range c.sub.candidates {
		c.sub.pc.AddICECandidate(*candidate)
	}
	c.sub.candidates = []*webrtc.ICECandidateInit{}

	answer, err := c.sub.pc.CreateAnswer(nil)
	if err != nil {
		log.Errorf("sub couldn't create answer %v", err)
		return
	}
	if err := c.sub.pc.SetLocalDescription(answer); err != nil {
		log.Errorf("sub couldn't setLocalDescription %v", err)
		return
	}

	c.signal.Answer(&answer)
}

// signalOnNegotiate is triggered from server for the sub pc
func (c *Client) signalOnTrickle(role int, candidate *webrtc.ICECandidateInit) {
	var target *transport
	switch role {
	case rolePublish:
		target = c.pub
	case roleSubscribe:
		target = c.sub
	}

	if target.pc.RemoteDescription() != nil {
		target.pc.AddICECandidate(*candidate)
	} else {
		target.candidates = append(target.candidates, candidate)
	}

}
