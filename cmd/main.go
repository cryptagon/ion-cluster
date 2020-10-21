// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	cluster "github.com/pion/ion-cluster/pkg"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/ion-sfu/pkg/log"
)

var (
	conf = cluster.RootConfig{}
	file string
	cert string
	key  string
	addr string
)

const (
	portRangeLimit = 100
)

func showHelp() {
	fmt.Printf("Usage:%s {params}\n", os.Args[0])
	fmt.Println("      -c {config file}")
	fmt.Println("      -cert {cert file}")
	fmt.Println("      -key {key file}")
	fmt.Println("      -a {listen addr}")
	fmt.Println("      -h (show help info)")
}

func load() bool {
	_, err := os.Stat(file)
	if err != nil {
		return false
	}

	viper.SetConfigFile(file)
	viper.SetConfigType("toml")

	err = viper.ReadInConfig()
	if err != nil {
		fmt.Printf("config file %s read failed. %v\n", file, err)
		return false
	}
	err = viper.GetViper().Unmarshal(&conf)
	if err != nil {
		fmt.Printf("sfu config file %s loaded failed. %v\n", file, err)
		return false
	}

	if len(conf.SFU.WebRTC.ICEPortRange) > 2 {
		fmt.Printf("config file %s loaded failed. range port must be [min,max]\n", file)
		return false
	}

	if len(conf.SFU.WebRTC.ICEPortRange) != 0 && conf.SFU.WebRTC.ICEPortRange[1]-conf.SFU.WebRTC.ICEPortRange[0] < portRangeLimit {
		fmt.Printf("config file %s loaded failed. range port must be [min, max] and max - min >= %d\n", file, portRangeLimit)
		return false
	}

	fmt.Printf("config %s load ok!\n", file)
	return true
}

func parse() bool {
	flag.StringVar(&file, "c", "config.toml", "config file")
	flag.StringVar(&cert, "cert", "", "cert file")
	flag.StringVar(&key, "key", "", "key file")
	flag.StringVar(&addr, "a", ":7000", "address to use")
	help := flag.Bool("h", false, "help info")
	flag.Parse()
	if !load() {
		return false
	}

	if *help {
		showHelp()
		return false
	}
	return true
}

func main() {
	if !parse() {
		showHelp()
		os.Exit(-1)
	}
	log.Init(conf.SFU.Log.Level, conf.SFU.Log.Fix)

	log.Infof("--- Starting SFU Node ---")
	s := sfu.NewSFU(conf.SFU)

	// Spin up raft node
	raftLogger := zerolog.New(os.Stdout)
	r, err := cluster.NewRaft(&conf.Cluster.Raft, &raftLogger)
	if err != nil {
		log.Errorf("Error initializing raft node: %v", err)
		return
	}

	// Start cluster node
	n, nErr := cluster.NewNode(conf.Cluster)
	go n.Run()

	// Spin up websocket
	wsServer, wsError := cluster.NewWebsocketServer(s, conf.Signal)
	go wsServer.Run()

	// Listen for signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Select on error channels from different modules
	for {
		select {
		case err := <-wsError:
			log.Errorf("Error in wsServer: %v", err)
			return
		case err := <-nErr:
			log.Errorf("Error in cluster.Node{} %v", err)
			return
		case leader := <-r.RaftNode.LeaderCh():
			log.Debugf("Leadership Changed, isLeader %v", leader)
		case nodeEvent := <-n.NodeEventCh:
			log.Debugf("Node Event: %v", nodeEvent)
			if f := r.RaftNode.VerifyLeader(); f.Error() == nil {
				// we are leader
				switch nodeEvent.Event {
				case memberlist.NodeJoin:
					peerRaftPort := (nodeEvent.Node.Port + 100)
					peer := fmt.Sprintf("%v:%v", nodeEvent.Node.Addr.String(), peerRaftPort)
					f := r.RaftNode.AddVoter(raft.ServerID(peer), raft.ServerAddress(peer), 0, 0)
					if f.Error() != nil {
						log.Errorf("error adding voter: %s", err)
						break
					}
				case memberlist.NodeLeave:
					peerRaftPort := (nodeEvent.Node.Port + 100)
					peer := fmt.Sprintf("%v:%v", nodeEvent.Node.Addr.String(), peerRaftPort)
					f := r.RaftNode.RemoveServer(raft.ServerID(peer), 0, 0)
					if f.Error() != nil {
						log.Errorf("error adding voter: %s", err)
						break
					}
				}
			}
		case sig := <-sigs:
			log.Debugf("got signal %v", sig)
			n.Shutdown()
			r.RaftNode.Shutdown().Error()
			return
		}
	}
}
