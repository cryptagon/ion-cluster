package cluster

import pb "github.com/pion/ion-sfu/cmd/signal/grpc/proto"

type nodeStatus string

const (
	nodeStatusUp       = "node_up"
	nodeStatusDraining = "node_draining"
	nodeStatusDown     = "node_down"
)

type nodeInfo struct {
	id          string
	status      nodeStatus
	endpoint    string
	streamCount int
}

// Coordinator is responsible for managing sessions
// and providing rpc connections to other nodes
type coordinator interface {
	updateNodeInfo(info nodeInfo) error
	getClientForNode(nodeID string) (*pb.SFUClient, error)
	createSession(nodeID string, sessionID string) error
}
