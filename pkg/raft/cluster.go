package cluster

import (
	"fmt"
	"os"

	"github.com/hashicorp/memberlist"
	"github.com/pborman/uuid"
	"github.com/pion/ion-sfu/pkg/log"
)

// Node manages cluster membership
type Node struct {
	config     NodeConfig
	memberlist *memberlist.Memberlist
	errChan    chan error

	NodeEventCh chan memberlist.NodeEvent
}

// NewNode initializes a cluster node with memberlist
func NewNode(config NodeConfig) (*Node, chan error) {
	return &Node{
		config:      config,
		errChan:     make(chan error),
		NodeEventCh: make(chan memberlist.NodeEvent),
	}, nil
}

// Run starts memberlist and the delegates
func (n *Node) Run() {
	hostname, _ := os.Hostname()
	c := memberlist.DefaultLocalConfig()
	c.Events = &memberlist.ChannelEventDelegate{
		Ch: n.NodeEventCh,
	}
	c.BindPort = n.config.MemberlistPort
	c.Name = hostname + "-" + uuid.NewUUID().String()

	log.Infof("Creating memberlist node: %v", c.Name)
	list, err := memberlist.Create(c)
	if err != nil {
		n.errChan <- fmt.Errorf("Failed to create memberlist: " + err.Error())
		return
	}
	n.memberlist = list

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(n.config.Join)
	if err != nil {
		n.errChan <- fmt.Errorf("Failed to join cluster: " + err.Error())
	}

	node := list.LocalNode()
	log.Debugf("Local member %s:%d\n", node.Addr, node.Port)
}

// Shutdown memberlist
func (n *Node) Shutdown() {
	n.memberlist.Shutdown()
}
