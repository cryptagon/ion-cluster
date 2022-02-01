package rtc

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/bep/debounce"
	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const APIChannelLabel = "ion-sfu"

type Subscriber struct {
	sync.RWMutex

	id string
	pc *webrtc.PeerConnection
	me *webrtc.MediaEngine

	tracks     map[string][]*sfu.DownTrack
	channels   map[string]*webrtc.DataChannel
	candidates []webrtc.ICECandidateInit

	negotiate func()
	closeOnce sync.Once

	noAutoSubscribe bool
}

// NewSubscriber creates a new Subscriber
func NewSubscriber(id string, cfg WebRTCTransportConfig) (*Subscriber, error) {
	me, err := getSubscriberMediaEngine()
	if err != nil {
		logger.Errorw("NewPeer error", err)
		return nil, errPeerConnectionInitFailed
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(cfg.Setting))
	pc, err := api.NewPeerConnection(cfg.Configuration)

	if err != nil {
		logger.Errorw("NewPeer error", err)
		return nil, errPeerConnectionInitFailed
	}

	s := &Subscriber{
		id:              id,
		me:              me,
		pc:              pc,
		tracks:          make(map[string][]*sfu.DownTrack),
		channels:        make(map[string]*webrtc.DataChannel),
		noAutoSubscribe: false,
	}

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		logger.Infow("ice connection status", "state", connectionState)
		switch connectionState {
		case webrtc.ICEConnectionStateFailed:
			fallthrough
		case webrtc.ICEConnectionStateClosed:
			s.closeOnce.Do(func() {
				logger.Infow("webrtc ice closed", "peer_id", s.id)
				if err := s.Close(); err != nil {
					logger.Errorw("webrtc transport close err", err)
				}
			})
		}
	})

	go s.downTracksReports()

	return s, nil
}

func (s *Subscriber) AddDatachannel(peer Peer, dc *Datachannel) error {
	ndc, err := s.pc.CreateDataChannel(dc.Label, &webrtc.DataChannelInit{})
	if err != nil {
		return err
	}

	mws := newDCChain(dc.middlewares)
	p := mws.Process(ProcessFunc(func(ctx context.Context, args ProcessArgs) {
		if dc.onMessage != nil {
			dc.onMessage(ctx, args)
		}
	}))
	ndc.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.Process(context.Background(), ProcessArgs{
			Peer:        peer,
			Message:     msg,
			DataChannel: ndc,
		})
	})

	s.channels[dc.Label] = ndc

	return nil
}

// DataChannel returns the channel for a label
func (s *Subscriber) DataChannel(label string) *webrtc.DataChannel {
	s.RLock()
	defer s.RUnlock()
	return s.channels[label]
}

func (s *Subscriber) OnNegotiationNeeded(f func()) {
	debounced := debounce.New(250 * time.Millisecond)
	s.negotiate = func() {
		debounced(f)
	}
}

func (s *Subscriber) CreateOffer() (webrtc.SessionDescription, error) {
	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	err = s.pc.SetLocalDescription(offer)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	return offer, nil
}

// OnICECandidate handler
func (s *Subscriber) OnICECandidate(f func(c *webrtc.ICECandidate)) {
	s.pc.OnICECandidate(f)
}

// AddICECandidate to peer connection
func (s *Subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if s.pc.RemoteDescription() != nil {
		return s.pc.AddICECandidate(candidate)
	}
	s.candidates = append(s.candidates, candidate)
	return nil
}

func (s *Subscriber) AddDownTrack(streamID string, downTrack *sfu.DownTrack) {
	s.Lock()
	defer s.Unlock()
	if dt, ok := s.tracks[streamID]; ok {
		dt = append(dt, downTrack)
		s.tracks[streamID] = dt
	} else {
		s.tracks[streamID] = []*sfu.DownTrack{downTrack}
	}
}

func (s *Subscriber) RemoveDownTrack(streamID string, downTrack *sfu.DownTrack) {
	s.Lock()
	defer s.Unlock()
	if dts, ok := s.tracks[streamID]; ok {
		idx := -1
		for i, dt := range dts {
			if dt == downTrack {
				idx = i
				break
			}
		}
		if idx >= 0 {
			dts[idx] = dts[len(dts)-1]
			dts[len(dts)-1] = nil
			dts = dts[:len(dts)-1]
			s.tracks[streamID] = dts
		}
	}
}

func (s *Subscriber) AddDataChannel(label string) (*webrtc.DataChannel, error) {
	s.Lock()
	defer s.Unlock()

	if s.channels[label] != nil {
		return s.channels[label], nil
	}

	dc, err := s.pc.CreateDataChannel(label, &webrtc.DataChannelInit{})
	if err != nil {
		logger.Errorw("dc creation error", err)
		return nil, errCreatingDataChannel
	}

	s.channels[label] = dc

	return dc, nil
}

// SetRemoteDescription sets the SessionDescription of the remote peer
func (s *Subscriber) SetRemoteDescription(desc webrtc.SessionDescription) error {
	if err := s.pc.SetRemoteDescription(desc); err != nil {
		logger.Errorw("SetRemoteDescription error", err)
		return err
	}

	for _, c := range s.candidates {
		if err := s.pc.AddICECandidate(c); err != nil {
			logger.Errorw("Add subscriber ice candidate to peer err", err, "peer_id", s.id)
		}
	}
	s.candidates = nil

	return nil
}

func (s *Subscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {
	s.Lock()
	s.channels[label] = dc
	s.Unlock()
}

func (s *Subscriber) GetDatachannel(label string) *webrtc.DataChannel {
	return s.DataChannel(label)
}

func (s *Subscriber) DownTracks() []*sfu.DownTrack {
	s.RLock()
	defer s.RUnlock()
	var downTracks []*sfu.DownTrack
	for _, tracks := range s.tracks {
		downTracks = append(downTracks, tracks...)
	}
	return downTracks
}

func (s *Subscriber) GetDownTracks(streamID string) []*sfu.DownTrack {
	s.RLock()
	defer s.RUnlock()
	return s.tracks[streamID]
}

// Negotiate fires a debounced negotiation request
func (s *Subscriber) Negotiate() {
	s.negotiate()
}

// Close peer
func (s *Subscriber) Close() error {
	return s.pc.Close()
}

func (s *Subscriber) downTracksReports() {
	for {
		time.Sleep(5 * time.Second)

		if s.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}

		var r []rtcp.Packet
		var sd []rtcp.SourceDescriptionChunk
		s.RLock()
		for _, dts := range s.tracks {
			for _, dt := range dts {
				// if !dt.bound.get() {
				// 	continue
				// }
				if sr := dt.CreateSenderReport(); sr != nil {
					r = append(r, sr)
				}
				sd = append(sd, dt.CreateSourceDescriptionChunks()...)
			}
		}
		s.RUnlock()
		i := 0
		j := 0
		for i < len(sd) {
			i = (j + 1) * 15
			if i >= len(sd) {
				i = len(sd)
			}
			nsd := sd[j*15 : i]
			r = append(r, &rtcp.SourceDescription{Chunks: nsd})
			j++
			if err := s.pc.WriteRTCP(r); err != nil {
				if err == io.EOF || err == io.ErrClosedPipe {
					return
				}
				logger.Errorw("Sending downtrack reports err", err)
			}
			r = r[:0]
		}
	}
}

func (s *Subscriber) sendStreamDownTracksReports(streamID string) {
	var r []rtcp.Packet
	var sd []rtcp.SourceDescriptionChunk

	s.RLock()
	dts := s.tracks[streamID]
	for _, dt := range dts {
		//if !dt.bound.get() {
		//	continue
		//}
		if sdc := dt.CreateSourceDescriptionChunks(); sdc != nil {
			sd = append(sd, sdc...)
		}
	}
	s.RUnlock()
	if len(sd) == 0 {
		return
	}
	r = append(r, &rtcp.SourceDescription{Chunks: sd})
	go func() {
		r := r
		i := 0
		for {
			if err := s.pc.WriteRTCP(r); err != nil {
				logger.Errorw("Sending track binding reports err", err)
			}
			if i > 5 {
				return
			}
			i++
			time.Sleep(20 * time.Millisecond)
		}
	}()
}
