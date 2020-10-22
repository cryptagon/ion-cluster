package cluster

import (
	"sync"

	"github.com/pborman/uuid"
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
	getOrCreateSession(sessionID string) (sessionMeta, error)
}

// NewCoordinator configures coordinator for this node
func NewCoordinator(conf RootConfig) coordinator {
	return &memCoordinator{
		nodeID:   uuid.New(),
		endpoint: conf.Endpoint(),
	}
}

type memCoordinator struct {
	mu       sync.Mutex
	nodeID   string
	endpoint string
}

func (m *memCoordinator) getOrCreateSession(sessionID string) (sessionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return sessionMeta{
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

// type etcdCoordinator struct {
// 	client *clientv3.Client
// }

// func newCoordinatorEtcd() (*etcdCoordinator, error) {
// 	ctx, err := context.WithTimeout(context.Background(), requestTimeout)
// 	if err != nil {
// 		return nil, err
// 	}

// 	cli, err := clientv3.New(clientv3.Config{
// 		DialTimeout: dialTimeout,
// 		Endpoints:   []string{"127.0.0.1:2379"},
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	return &etcdCoordinator{
// 		client: cli,
// 	}, nil
// }

// func (m *etcdCoordinator) getClientForSession(sessionID string) (*pb.SFUClient, error) {
// 	//todo iml
// 	return nil, nil
// }
