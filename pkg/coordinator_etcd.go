package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pborman/uuid"
	"github.com/pion/ion-sfu/pkg/buffer"
	"github.com/pion/ion-sfu/pkg/middlewares/datachannel"
	"github.com/pion/ion-sfu/pkg/sfu"
	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
)

type etcdCoordinator struct {
	mu           sync.Mutex
	nodeID       string
	nodeEndpoint string
	client       *clientv3.Client

	w             sfu.WebRTCTransportConfig
	datachannels  []*sfu.Datachannel
	localSessions map[string]*Session
	sessionLeases map[string]context.CancelFunc
}

func newCoordinatorEtcd(conf RootConfig) (*etcdCoordinator, error) {
	log.Info("creating etcd client")
	cli, err := clientv3.New(clientv3.Config{
		DialTimeout: time.Second * 3,
		DialOptions: []grpc.DialOption{grpc.WithBlock()},
		Endpoints:   conf.Coordinator.Etcd.Hosts,
	})
	if err != nil {
		return nil, err
	}

	if conf.SFU.BufferFactory == nil {
		conf.SFU.BufferFactory = buffer.NewBufferFactory(conf.SFU.Router.MaxPacketTrack, log.WithName("buffer"))
	}
	w := sfu.NewWebRTCTransportConfig(conf.SFU)
	dc := &sfu.Datachannel{Label: sfu.APIChannelLabel}
	dc.Use(datachannel.SubscriberAPI)

	log.Info("created etcdCoordinator")
	return &etcdCoordinator{
		client:        cli,
		nodeID:        uuid.New(),
		nodeEndpoint:  conf.Endpoint(),
		w:             w,
		datachannels:  []*sfu.Datachannel{dc},
		sessionLeases: make(map[string]context.CancelFunc),
		localSessions: make(map[string]*Session),
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
		log.Error(err, "could not acquire session lock", "sessionID", sessionID)
		return nil, err
	}
	defer mu.Unlock(ctx)

	// Check to see if sessionMeta exists for this key
	gr, err := e.client.Get(ctx, key)
	if err != nil {
		log.Error(err, "error looking up session", "sessionID", sessionID)
		return nil, err
	}

	// Session already exists somewhere in the cluster
	// return the existing meta to the user
	if gr.Count > 0 {
		log.Info("found session on node ", "session_id", sessionID)

		var meta sessionMeta
		if err := json.Unmarshal(gr.Kvs[0].Value, &meta); err != nil {
			log.Error(err, "error unmarshaling session meta", "sessionID", sessionID)
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
		log.Error(err, "error acquiring lease for session key", "key", key)
		return nil, err
	}

	ctx, leaseCancel := context.WithCancel(context.Background())
	leaseKeepAlive, err := e.client.KeepAlive(ctx, lease.ID)
	if err != nil {
		log.Error(err, "error activating keepAlive for lease", "leaseID", lease.ID)
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
		log.Error(err, "error storing session meta")
		return nil, err
	}

	return &meta, nil
}

func (e *etcdCoordinator) ensureSession(sessionID string) *Session {
	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.localSessions[sessionID]; ok {
		return s
	}

	s := NewSession(sessionID, e.datachannels, e.w)
	s.OnClose(func() {
		e.onSessionClosed(sessionID)
	})
	prometheusGaugeSessions.Inc()

	e.localSessions[sessionID] = &s
	return &s
}

func (e *etcdCoordinator) GetSession(sid string) (sfu.Session, sfu.WebRTCTransportConfig) {
	return e.ensureSession(sid), e.w
}

func (e *etcdCoordinator) onSessionClosed(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Acquire the lock for this sessionID
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	s, err := concurrency.NewSession(e.client, concurrency.WithContext(ctx))
	if err != nil {
		log.Error(err, "etcdCoordinator onSessionClosed couldn't acquire sesion lock", "sessionID", sessionID)
		return
	}
	key := fmt.Sprintf("/session/%v", sessionID)
	mu := concurrency.NewMutex(s, key)
	if err := mu.Lock(ctx); err != nil {
		log.Error(err, "etcdCoordinator onSessionClosed couldn't acquire sesion lock", "sessionID", sessionID)
		return
	}
	defer mu.Unlock(ctx)

	// Cancel our lease
	if leaseCancel, ok := e.sessionLeases[sessionID]; ok {
		delete(e.sessionLeases, sessionID)
		leaseCancel()
	} else {
		log.Error(nil, "Could not find session lease!", "sessionID", sessionID)
	}

	// Delete session meta
	_, err = e.client.Delete(ctx, key)
	if err != nil {
		log.Error(err, "etcdCoordinator error deleting sessionMeta", "sessionID", sessionID)
		return
	}

	// Delete localSession
	delete(e.localSessions, sessionID)
	prometheusGaugeSessions.Dec()

	log.Info("etcdCoordinator canceled session lease", "sessionID", sessionID)
}
