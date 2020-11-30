package cluster

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pborman/uuid"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
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
	sfu.TransportProvider
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

	mu       sync.Mutex
	w        sfu.WebRTCTransportConfig
	sessions map[string]*sfu.Session
}

func newCoordinatorLocal(conf RootConfig) (coordinator, error) {
	w := sfu.NewWebRTCTransportConfig(conf.SFU)
	return &localCoordinator{
		nodeID:       uuid.New(),
		nodeEndpoint: conf.Endpoint(),
		sessions:     make(map[string]*sfu.Session),
		w:            w,
	}, nil
}

func (c *localCoordinator) ensureSession(sessionID string) *sfu.Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s, ok := c.sessions[sessionID]; ok {
		return s
	}

	s := sfu.NewSession(sessionID)
	s.OnClose(func() {
		c.onSessionClosed(sessionID)
	})
	prometheusGaugeSessions.Inc()

	c.sessions[sessionID] = s
	return s
}
func (c *localCoordinator) NewTransport(sid, pid string, me sfu.MediaEngine) (*sfu.Session, *sfu.Publisher, *sfu.Subscriber, error) {
	session := c.ensureSession(sid)

	sub, err := sfu.NewSubscriber(session, pid, me, c.w)

	if err != nil {
		return nil, nil, nil, err
	}

	pub, err := sfu.NewPublisher(session, pid, me, c.w)
	if err != nil {
		return nil, nil, nil, err
	}

	return session, pub, sub, nil
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
