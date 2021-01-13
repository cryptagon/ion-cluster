package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	cluster "github.com/pion/ion-cluster/pkg"
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start an ion-cluster server node",
	RunE:  serverMain,
}

func init() {
	serverCmd.PersistentFlags().StringVarP(&conf.Signal.HTTPAddr, "addr", "a", ":7000", "http listen address")
	serverCmd.PersistentFlags().StringVar(&conf.Signal.Cert, "cert", "", "tls certificate")
	serverCmd.PersistentFlags().StringVar(&conf.Signal.Key, "key", "", "tls priv key")

	rootCmd.AddCommand(serverCmd)

}

func serverMain(cmd *cobra.Command, args []string) error {

	log.Infof("--- Starting SFU Node ---")
	s := sfu.NewSFU(conf.SFU)

	coordinator, err := cluster.NewCoordinator(conf)
	if err != nil {
		log.Errorf("error creating coordinator: %v", err)
		return err
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
			return err
		case sig := <-sigs:
			log.Debugf("Got Signal %v, beginning shutdown", sig)
			sServer.NodeState(cluster.NodeStateTerminating)
			ticker := time.NewTicker(500 * time.Millisecond)
			for {
				active := cluster.MetricsGetActiveClientsCount()
				if active == 0 {
					log.Debugf("server idle, shutting down")
					return nil
				}
				log.Debugf("shutdown waiting on %v clients", active)
				select {
				case <-ticker.C:
					continue
				case sig = <-sigs:
					log.Debugf("Got second signal: forcing shutdown")
					return nil
				}
			}
		}
	}
}
