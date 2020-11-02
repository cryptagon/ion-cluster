// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"

	cluster "github.com/pion/ion-cluster/pkg"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
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

	if host := os.Getenv("ION_CLUSTER_HOST"); host != "" {
		conf.Signal.FQDN = host
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
	fixByFile := []string{"asm_amd64.s", "proc.go", "icegatherer.go"}
	fixByFunc := []string{}
	log.Init(conf.SFU.Log.Level, fixByFile, fixByFunc)

	log.Infof("--- Starting SFU Node ---")
	s := sfu.NewSFU(conf.SFU)

	coordinator, err := cluster.NewCoordinator(conf)
	if err != nil {
		log.Errorf("error creating coordinator: %v", err)
		return
	}

	// Spin up websocket
	sServer, sError := cluster.NewSignal(s, coordinator, conf.Signal)
	if conf.Signal.HTTPAddr != "" {
		go sServer.ServeWebsocket()
	}
	// if conf.Signal.GRPCAddr != "" {
	// 	go sServer.ServeGRPC()
	// }

	// Listen for signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Select on error channels from different modules
	for {
		select {
		case err := <-sError:
			log.Errorf("Error in wsServer: %v", err)
			return
		case sig := <-sigs:
			log.Debugf("got signal %v", sig)
			//todo wait for all sessions to end
			return
		}
	}
}
