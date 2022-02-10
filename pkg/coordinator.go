package cluster

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pborman/uuid"
	"github.com/pion/ion-cluster/pkg/rtc"
	"github.com/pion/ion-cluster/pkg/sfu/buffer"
)

var (
	errNonLocalSession = errors.New("session is not located on this node")
)

type sessionMeta struct {
	SessionID    string `json:"session_id"`
	NodeID       string `json:"node_id"`
	NodeEndpoint string `json:"node_endpoint"`
	Redirect     bool   `json:"redirect"`
}

// Coordinator is responsible for managing sessions
// and providing rpc connections to other nodes
type coordinator interface {
	getOrCreateSession(sessionID rtc.SessionID) (*sessionMeta, error)
	rtc.SessionProvider
}

// NewCoordinator configures coordinator for this node
func NewCoordinator(conf RootConfig) (coordinator, error) {
	if conf.Coordinator.Etcd != nil {
		return newCoordinatorEtcd(conf)
	}
	if conf.Coordinator.Local != nil {
		return newCoordinatorLocal(conf)
	}
	return nil, fmt.Errorf("error no coodinator configured")
}

type localCoordinator struct {
	nodeID       string
	nodeEndpoint string

	mu           sync.Mutex
	w            rtc.WebRTCTransportConfig
	sessions     map[string]*Session
	datachannels []*rtc.Datachannel
}

func newCoordinatorLocal(conf RootConfig) (coordinator, error) {
	if conf.SFU.BufferFactory == nil {
		conf.SFU.BufferFactory = buffer.NewBufferFactory(conf.SFU.Router.MaxPacketTrack, log.WithName("buffer"))
	}
	w := rtc.NewWebRTCTransportConfig(conf.SFU)
	dc := &rtc.Datachannel{Label: rtc.APIChannelLabel}
	dc.Use(rtc.SubscriberAPI)

	return &localCoordinator{
		nodeID:       uuid.New(),
		nodeEndpoint: conf.Endpoint(),
		datachannels: []*rtc.Datachannel{dc},
		sessions:     make(map[string]*Session),
		w:            w,
	}, nil
}

func (c *localCoordinator) ensureSession(sessionID string) *Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s, ok := c.sessions[sessionID]; ok {
		return s
	}

	s := NewSession(sessionID, c.datachannels, c.w)
	s.OnClose(func() {
		c.onSessionClosed(sessionID)
	})
	prometheusGaugeSessions.Inc()

	c.sessions[sessionID] = &s
	return &s
}

func (c *localCoordinator) GetSession(sid string) (rtc.ISession, rtc.WebRTCTransportConfig) {
	return c.ensureSession(sid), c.w
}

func (c *localCoordinator) getOrCreateSession(sessionID string) (*sessionMeta, error) {
	c.ensureSession(sessionID)

	return &sessionMeta{
		SessionID:    sessionID,
		NodeID:       c.nodeID,
		NodeEndpoint: c.nodeEndpoint,
		Redirect:     false,
	}, nil
}

func (c *localCoordinator) onSessionClosed(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Info("session closed", "sessionID", sessionID)
	delete(c.sessions, sessionID)
	prometheusGaugeSessions.Dec()
}
