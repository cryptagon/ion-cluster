package cluster

import (
	"io"
	"net"
	"os"
	"time"

	"github.com/hashicorp/raft"
	"github.com/rs/zerolog"
)

type Raft struct {
	config   *resolvedRaftConfig
	RaftNode *raft.Raft
	fsm      *fsm
	log      *zerolog.Logger
}

func NewRaft(rawConfig *RawRaftConfig, log *zerolog.Logger) (*Raft, error) {
	config, err := resolveRaftConfig(rawConfig)
	if err != nil {
		return nil, err
	}

	fsm := &fsm{
		sessions: make(map[string]string),
	}

	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return nil, err
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.RaftAddress.String())
	transportLogger := log.With().Str("component", "raft-transport").Logger()
	transport, err := raftTransport(config.RaftAddress, transportLogger)
	if err != nil {
		return nil, err
	}

	//snapshotStoreLogger := log.With().Str("component", "raft-snapshots").Logger()
	//snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 1, snapshotStoreLogger)
	//if err != nil {
	//	return nil, err
	//}

	//	logStore, err := raftboltdb.NewBoltStore(filepath.Join(config.DataDir, "raft-log.bolt"))
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(config.DataDir, "raft-stable.bolt"))
	//	if err != nil {
	//		return nil, err
	//	}
	//

	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()
	snapshotStore := raft.NewInmemSnapshotStore()

	raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore,
		snapshotStore, transport)
	if err != nil {
		return nil, err
	}
	if config.Bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	}
	return &Raft{
		config:   config,
		RaftNode: raftNode,
		log:      log,
		fsm:      fsm,
	}, nil
}

func raftTransport(raftAddr net.Addr, log io.Writer) (*raft.NetworkTransport, error) {
	address, err := net.ResolveTCPAddr("tcp", raftAddr.String())
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(address.String(), address, 3, 10*time.Second, log)
	if err != nil {
		return nil, err
	}

	return transport, nil
}
