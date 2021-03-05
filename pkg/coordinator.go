package cluster

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pborman/uuid"
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/buffer"
	"github.com/pion/ion-sfu/pkg/middlewares/datachannel"
	"github.com/pion/ion-sfu/pkg/sfu"
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
	getOrCreateSession(sessionID string) (*sessionMeta, error)
	sfu.SessionProvider
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

	mu            sync.Mutex
	w             sfu.WebRTCTransportConfig
	bufferFactory *buffer.Factory
	sessions      map[string]*sfu.Session
	datachannels  []*sfu.Datachannel
}

func newCoordinatorLocal(conf RootConfig) (coordinator, error) {
	if conf.SFU.BufferFactory == nil {
		conf.SFU.BufferFactory = buffer.NewBufferFactory(conf.SFU.Router.MaxPacketTrack)
	}
	w := sfu.NewWebRTCTransportConfig(conf.SFU)
	dc := &sfu.Datachannel{Label: sfu.APIChannelLabel}
	dc.Use(datachannel.SubscriberAPI)

	return &localCoordinator{
		nodeID:        uuid.New(),
		nodeEndpoint:  conf.Endpoint(),
		datachannels:  []*sfu.Datachannel{dc},
		sessions:      make(map[string]*sfu.Session),
		w:             w,
		bufferFactory: conf.SFU.BufferFactory,
	}, nil
}

func (c *localCoordinator) ensureSession(sessionID string) *sfu.Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s, ok := c.sessions[sessionID]; ok {
		return s
	}

	s := sfu.NewSession(sessionID, c.bufferFactory, c.datachannels, c.w)
	s.OnClose(func() {
		c.onSessionClosed(sessionID)
	})
	prometheusGaugeSessions.Inc()

	c.sessions[sessionID] = s
	return s
}

func (c *localCoordinator) GetSession(sid string) (*sfu.Session, sfu.WebRTCTransportConfig) {
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
	log.Debugf("session %v closed", sessionID)
	delete(c.sessions, sessionID)
	prometheusGaugeSessions.Dec()
}
