package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"

	"github.com/pion/ion-cluster/pkg/client"
	log "github.com/pion/ion-log"
	"github.com/spf13/cobra"
)

var (
	clientUrl string
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to an ion-cluster server as a client",
	RunE:  clientMain,
}

func init() {
	clientCmd.PersistentFlags().StringVarP(&clientUrl, "url", "u", "ws://localhost:7000/session/test", "config file (default is $HOME/.cobra.yaml)")

	rootCmd.AddCommand(clientCmd)
}

func clientMain(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	w := sfu.NewWebRTCTransportConfig(conf.SFU)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {

	}()

	signal := client.NewJSONRPCSignalClient(ctx)
	c, err := client.NewClient(signal, w)
	if err != nil {
		log.Debugf("error initializing client %v", err)
	}

	fmt.Printf("client connecting to %v", clientUrl)

	signalClosedCh, err := signal.Open(clientUrl)
	if err != nil {
		return err
	}

	c.OnTrack = func(t *webrtc.Track, r *webrtc.RTPReceiver) {
		log.Debugf("Client got track!!!!")
	}
	c.Join("test")

	for {
		select {
		case sig := <-sigs:
			log.Debugf("got signal %v", sig)
			signal.Close()
		case <-signalClosedCh:
			log.Debugf("signal closed")
			return nil
		}
	}

}
