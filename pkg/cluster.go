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

//type delegate struct {
//	broadcasts *memberlist.TransmitLimitedQueue
//}
//
//func (d *delegate) NodeMeta(limit int) []byte {
//	return []byte{}
//}
//
//func (d *delegate) NotifyMsg(b []byte) {
//	if len(b) == 0 {
//		return
//	}
//	log.Debugf("got notify message: %v", string(b))
//}
//
//func (d *delegate) GetBroadcasts(overhead, limit int) [][]byte {
//	return d.broadcasts.GetBroadcasts(overhead, limit)
//}
//
//func (d *delegate) LocalState(join bool) []byte {
//	return []byte{}
//}
//
//func (d *delegate) MergeRemoteState(buf []byte, join bool) {
//	return
//}

type eventDelegate struct{}

func (ed *eventDelegate) NotifyJoin(node *memberlist.Node) {
	log.Debugf("A node has joined: " + node.String())
}

func (ed *eventDelegate) NotifyLeave(node *memberlist.Node) {
	log.Debugf("A node has left: " + node.String())
}

func (ed *eventDelegate) NotifyUpdate(node *memberlist.Node) {
	log.Debugf("A node was updated: " + node.String())
}
