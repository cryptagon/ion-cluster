package cluster

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/rs/zerolog"
)

// func main() {
// 	logger := zerolog.New(os.Stdout)

// 	rawConfig := readRawConfig()
// 	config, err := resolveConfig(rawConfig)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Configuration errors - %s\n", err)
// 		os.Exit(1)
// 	}

// 	nodeLogger := logger.With().Str("component", "node").Logger()
// 	node, err := NewNode(config, &nodeLogger)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Error configuring node: %s", err)
// 		os.Exit(1)
// 	}

// 	if config.JoinAddress != "" {
// 		go func() {
// 			retryJoin := func() error {
// 				url := url.URL{
// 					Scheme: "http",
// 					Host:   config.JoinAddress,
// 					Path:   "join",
// 				}

// 				req, err := http.NewRequest(http.MethodPost, url.String(), nil)
// 				if err != nil {
// 					return err
// 				}
// 				req.Header.Add("Peer-Address", config.RaftAddress.String())

// 				resp, err := http.DefaultClient.Do(req)
// 				if err != nil {
// 					return err
// 				}

// 				if resp.StatusCode != http.StatusOK {
// 					return fmt.Errorf("non 200 status code: %d", resp.StatusCode)
// 				}

// 				return nil
// 			}

// 			for {
// 				if err := retryJoin(); err != nil {
// 					logger.Error().Err(err).Str("component", "join").Msg("Error joining cluster")
// 					time.Sleep(1 * time.Second)
// 				} else {
// 					break
// 				}
// 			}
// 		}()
// 	}

// 	httpLogger := logger.With().Str("component", "http").Logger()
// 	httpServer := &httpServer{
// 		node:    node,
// 		address: config.HTTPAddress,
// 		logger:  &httpLogger,
// 	}

// 	httpServer.Start()

// }

type node struct {
	config   *resolvedRaftConfig
	raftNode *raft.Raft
	fsm      *fsm
	log      *zerolog.Logger
}

func NewNode(config *resolvedRaftConfig, log *zerolog.Logger) (*node, error) {
	fsm := &fsm{
		stateValue: 0,
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

	snapshotStoreLogger := log.With().Str("component", "raft-snapshots").Logger()
	snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 1, snapshotStoreLogger)
	if err != nil {
		return nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(config.DataDir, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(config.DataDir, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}
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
	return &node{
		config:   config,
		raftNode: raftNode,
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
