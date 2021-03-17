package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/pion/ion-cluster/pkg/client"
	"github.com/pion/ion-cluster/pkg/client/gst"
	log "github.com/pion/ion-log"
	"github.com/spf13/cobra"
)

var relayCmd = &cobra.Command{
	Use:   "rtmp-relay",
	Short: "Connect to an ion-cluster server as a client",
	RunE:  relayMain,
}

func init() {
	relayCmd.PersistentFlags().StringVarP(&clientURL, "url", "u", "ws://localhost:7000", "sfu host to connect to")
	relayCmd.PersistentFlags().StringVarP(&clientSID, "sid", "s", "test-session", "session id to join")
	relayCmd.PersistentFlags().StringVarP(&clientToken, "token", "t", "", "jwt access token")

	rootCmd.AddCommand(relayCmd)
}

func relayMain(cmd *cobra.Command, args []string) error {
	runtime.LockOSThread()
	go relayThread(cmd, args)
	gst.MainLoop()
	return nil
}

func relayThread(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	w := webrtc.Configuration{}

	signal := client.NewJSONRPCSignalClient(ctx)
	c, err := client.NewClient(signal, &w, []interceptor.Interceptor{})
	if err != nil {
		log.Debugf("error initializing client %v", err)
	}

	fmt.Printf("client connecting to %v", endpoint())

	signalClosedCh, err := signal.Open(endpoint())
	if err != nil {
		return err
	}

	compositor := gst.NewCompositor()
	compositor.Play()

	c.OnTrack = func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		log.Debugf("Client got track: %#v", t)
		if t.Kind() == webrtc.RTPCodecTypeVideo {
			compositor.AddInputTrack(t)
		}
	}

	if err := c.Join(clientSID); err != nil {
		return err
	}

	log.Debugf("starting producer")

	// var producer *client.GSTProducer
	// if len(args) > 0 {
	// 	switch args[0] {
	// 	case "test":
	// 		producer = client.NewGSTProducer(c, "video", "")
	// 	default:
	// 		producer = client.NewGSTProducer(c, "screen", args[0])
	// 	}
	// }

	// if producer != nil {
	// 	log.Debugf("publishing tracks")
	// 	if err := c.Publish(producer); err != nil {
	// 		log.Errorf("error publishing tracks: %v", err)
	// 		return err
	// 	}

	// 	log.Debugf("tracks published")
	// }

	t := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-t.C:
			if err := signal.Ping(); err != nil {
				log.Debugf("signal ping err: %v", err)
			}
			log.Debugf("signal ping got pong")
		case sig := <-sigs:
			log.Debugf("got signal %v", sig)
			signal.Close()
		case <-signalClosedCh:
			log.Debugf("signal closed")
			os.Exit(1)
			return nil
		}
	}

}
