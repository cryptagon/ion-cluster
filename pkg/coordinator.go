package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pborman/uuid"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
)

var (
	errNonLocalSession = errors.New("session is not located on this node")
)

type sessionMeta struct {
	SessionID    string `json:"session_id"`
	NodeID       string `json:"node_id"`
	NodeEndpoint string `json:"node_url"`
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

	c.sessions[sessionID] = s
	return s
}

func (c *localCoordinator) NewWebRTCTransport(sid string, me sfu.MediaEngine) (*sfu.WebRTCTransport, error) {
	session := c.ensureSession(sid)
	t, err := sfu.NewWebRTCTransport(session, me, c.w)
	if err != nil {
		return nil, err
	}
	return t, nil
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
}

type etcdCoordinator struct {
	mu           sync.Mutex
	nodeID       string
	nodeEndpoint string
	client       *clientv3.Client

	w             sfu.WebRTCTransportConfig
	localSessions map[string]*sfu.Session
	sessionLeases map[string]context.CancelFunc
}

func newCoordinatorEtcd(conf RootConfig) (*etcdCoordinator, error) {
	log.Debugf("creating etcd client")
	cli, err := clientv3.New(clientv3.Config{
		DialTimeout: time.Second * 3,
		DialOptions: []grpc.DialOption{grpc.WithBlock()},
		Endpoints:   conf.Coordinator.Etcd.Hosts,
	})
	if err != nil {
		return nil, err
	}
	w := sfu.NewWebRTCTransportConfig(conf.SFU)

	log.Debugf("created etcdCoordinator")
	return &etcdCoordinator{
		client:        cli,
		nodeID:        uuid.New(),
		nodeEndpoint:  conf.Endpoint(),
		w:             w,
		sessionLeases: make(map[string]context.CancelFunc),
		localSessions: make(map[string]*sfu.Session),
	}, nil
}

func (e *etcdCoordinator) getOrCreateSession(sessionID string) (*sessionMeta, error) {
	// This operation is only alloted 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	s, err := concurrency.NewSession(e.client, concurrency.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	// Acquire the lock for this sessionID
	key := fmt.Sprintf("/session/%v", sessionID)
	mu := concurrency.NewMutex(s, key)
	if err := mu.Lock(ctx); err != nil {
		log.Errorf("could not acquire session lock")
		return nil, err
	}
	defer mu.Unlock(ctx)

	// Check to see if sessionMeta exists for this key
	gr, err := e.client.Get(ctx, key)
	if err != nil {
		log.Errorf("error looking up session")
		return nil, err
	}

	// Session already exists somewhere in the cluster
	// return the existing meta to the user
	if gr.Count > 0 {
		log.Debugf("found session")

		var meta sessionMeta
		if err := json.Unmarshal(gr.Kvs[0].Value, &meta); err != nil {
			log.Errorf("error unmarshaling session meta: %v", err)
			return nil, err
		}
		meta.Redirect = (meta.NodeID != e.nodeID)

		// return meta for session
		return &meta, nil
	}

	// Session does not already exist, so lets take it
	// @todo load balance here / be smarter

	// First lets create a lease for the sessionKey
	lease, err := e.client.Grant(ctx, 1)
	if err != nil {
		log.Errorf("error acquiring lease for session key %v: %v", key, err)
		return nil, err
	}

	ctx, leaseCancel := context.WithCancel(context.Background())
	leaseKeepAlive, err := e.client.KeepAlive(ctx, lease.ID)
	if err != nil {
		log.Errorf("error activating keepAlive for lease %v: %v", lease.ID, err)
	}
	<-leaseKeepAlive

	e.mu.Lock()
	e.sessionLeases[sessionID] = leaseCancel
	defer e.mu.Unlock()

	meta := sessionMeta{
		SessionID:    sessionID,
		NodeID:       e.nodeID,
		NodeEndpoint: e.nodeEndpoint,
	}
	payload, _ := json.Marshal(&meta)
	_, err = e.client.Put(ctx, key, string(payload), clientv3.WithLease(lease.ID))
	if err != nil {
		log.Errorf("error storing session meta")
		return nil, err
	}

	return &meta, nil
}

func (e *etcdCoordinator) ensureSession(sessionID string) *sfu.Session {
	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.localSessions[sessionID]; ok {
		return s
	}

	s := sfu.NewSession(sessionID)
	s.OnClose(func() {
		e.onSessionClosed(sessionID)
	})

	e.localSessions[sessionID] = s
	return s
}

func (e *etcdCoordinator) NewWebRTCTransport(sid string, me sfu.MediaEngine) (*sfu.WebRTCTransport, error) {
	session := e.ensureSession(sid)
	t, err := sfu.NewWebRTCTransport(session, me, e.w)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (e *etcdCoordinator) onSessionClosed(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Acquire the lock for this sessionID
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	s, err := concurrency.NewSession(e.client, concurrency.WithContext(ctx))
	if err != nil {
		log.Errorf("etcdCoordinator onSessionClosed couldn't acquire sesion lock for %v", sessionID)
		return
	}
	key := fmt.Sprintf("/session/%v", sessionID)
	mu := concurrency.NewMutex(s, key)
	if err := mu.Lock(ctx); err != nil {
		log.Errorf("etcdCoordinator onSessionClosed couldn't acquire sesion lock for %v", sessionID)
		return
	}
	defer mu.Unlock(ctx)

	// Cancel our lease
	leaseCancel := e.sessionLeases[sessionID]
	delete(e.sessionLeases, sessionID)
	leaseCancel()

	// Delete session meta
	_, err = e.client.Delete(ctx, key)
	if err != nil {
		log.Errorf("etcdCoordinator error deleting sessionMeta for %v", sessionID)
		return
	}

	// Delete localSession
	delete(e.localSessions, sessionID)

	log.Debugf("etcdCoordinator canceled /session/%v lease", sessionID)
}
