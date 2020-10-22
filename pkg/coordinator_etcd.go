package cluster

import (
	"github.com/coreos/etcd/clientv3"

	pb "github.com/pion/ion-sfu/cmd/signal/grpc/proto"
)

type etcdCoordinator struct {
	client *clientv3.Client
}

func newCoordinatorEtcd() (*etcdCoordinator, error) {
	ctx, err := context.WithTimeout(context.Background(), requestTimeout)
	if err != nil {
		return nil, err
	}

    cli, err := clientv3.New(clientv3.Config{
        DialTimeout: dialTimeout,
        Endpoints: []string{"127.0.0.1:2379"},
	})
	if err!= nil {
		return err
	}

	return &etcdCoordinator {
		client: cli,
	}, nil
}

func (e *etcdCoordinator) updateNodeInfo(info nodeInfo) error {
	//todo impl
	return nil
}
func (m *memCoordinator) getClientForNode(nodeID string) (*pb.SFUClient, error) {
	//todo iml
	return nil, nil
}

func (m *memCoordinator) createSession(nodeID string, sessionID string) error {

	// todo impl
}

func (e *etcdCoordinator) Close() {

	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
    cli, _ := clientv3.New(clientv3.Config{
        DialTimeout: dialTimeout,
        Endpoints: []string{"127.0.0.1:2379"},
    })return
}
