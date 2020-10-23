package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pborman/uuid"
	"github.com/pion/ion-sfu/pkg/log"
	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
)

type sessionMeta struct {
	SessionID string `json:"session_id"`
	NodeID    string `json:"node_id"`
	Endpoint  string `json:"endpoint"`
	Redirect  bool   `json:"redirect"`
}

// Coordinator is responsible for managing sessions
// and providing rpc connections to other nodes
type coordinator interface {
	getOrCreateSession(sessionID string) (*sessionMeta, error)
}

// NewCoordinator configures coordinator for this node
func NewCoordinator(conf RootConfig) (coordinator, error) {
	if conf.Coordinator.Etcd != nil {
		return newCoordinatorEtcd(conf)
	}

	return nil, fmt.Errorf("error no coodinator configured")
	// return &memCoordinator{
	// 	nodeID:   uuid.New(),
	// 	endpoint: conf.Endpoint(),
	// }, nil
}

type memCoordinator struct {
	mu       sync.Mutex
	nodeID   string
	endpoint string
}

func (m *memCoordinator) getOrCreateSession(sessionID string) (*sessionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &sessionMeta{
		SessionID: sessionID,
		NodeID:    m.nodeID,
		Endpoint:  m.endpoint,
		Redirect:  false,
	}, nil
}

// func (m *memCoordinator) getClientForNode(nodeID string) (*pb.SFUClient, error) {
// 	// it would be better if we returned a pb.SFUClient mock here that acted like a local peer
// 	conn, err := grpc.Dial(m.endpoint, grpc.WithInsecure(), grpc.WithBlock())
// 	if err != nil {
// 		return nil, fmt.Errorf("memCoordinator couldn't create loopback grpc connection")
// 	}

// 	client := pb.NewSFUClient(conn)
// 	return &client, nil
// }

type etcdCoordinator struct {
	nodeID   string
	endpoint string
	client   *clientv3.Client
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

	log.Debugf("created etcdCoordinator")
	return &etcdCoordinator{
		client:   cli,
		nodeID:   uuid.New(),
		endpoint: conf.Endpoint(),
	}, nil
}

func (e *etcdCoordinator) getOrCreateSession(sessionID string) (*sessionMeta, error) {
	// This operation is only alloted 5 seconds to complete
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	s, err := concurrency.NewSession(e.client, concurrency.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("/session/%v", sessionID)
	mu := concurrency.NewMutex(s, key)
	if err := mu.Lock(ctx); err != nil {
		log.Errorf("could not acquire session lock")
		return nil, err
	}
	defer mu.Unlock(ctx)

	gr, err := e.client.Get(ctx, key)
	if err != nil {
		log.Errorf("error looking up session")
		return nil, err
	}

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

	// session doesn't already exist, so lets take it
	// @todo load balance here / be smarter

	meta := sessionMeta{
		SessionID: sessionID,
		NodeID:    e.nodeID,
		Endpoint:  e.endpoint,
	}
	payload, _ := json.Marshal(&meta)
	_, err = e.client.Put(ctx, key, string(payload))
	if err != nil {
		log.Errorf("error storing session meta")
		return nil, err
	}

	return &meta, nil
}
