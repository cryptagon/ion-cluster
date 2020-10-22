package cluster

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"

	pb "github.com/pion/ion-sfu/cmd/signal/grpc/proto"
)

type memCoordinator struct {
	mu   sync.Mutex
	info nodeInfo

	sessions []string
}

func (m *memCoordinator) updateNodeInfo(info nodeInfo) error {
	m.info = info
	return nil
}
func (m *memCoordinator) getClientForNode(nodeID string) (*pb.SFUClient, error) {
	if nodeID != m.info.id {
		return nil, fmt.Errorf("memCoordinator cannot get client for external node")
	}

	// it would be better if we returned a pb.SFUClient mock here that acted like a local peer
	conn, err := grpc.Dial(m.info.endpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("memCoordinator couldn't create loopback grpc connection")
	}

	client := pb.NewSFUClient(conn)
	return &client, nil
}

func (m *memCoordinator) createSession(nodeID string, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if nodeID != m.info.id {
		return fmt.Errorf("memCoordinator cannot create session on external node")
	}

	m.sessions = append(m.sessions, sessionID)
}
