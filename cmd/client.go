package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/pion/ion-cluster/pkg/client"
	"github.com/pion/ion-cluster/pkg/client/gst"
	log "github.com/pion/ion-log"
	"github.com/spf13/cobra"
)

var (
	clientURL   string
	clientSID   string
	clientToken string
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to an ion-cluster server as a client",
	RunE:  clientMain,
}

func init() {
	clientCmd.PersistentFlags().StringVarP(&clientURL, "url", "u", "ws://localhost:7000", "sfu host to connect to")
	clientCmd.PersistentFlags().StringVarP(&clientSID, "sid", "s", "test-session", "session id to join")
	clientCmd.PersistentFlags().StringVarP(&clientToken, "token", "t", "", "jwt access token")

	rootCmd.AddCommand(clientCmd)
}

func endpoint() string {
	url := fmt.Sprintf("%s/session/%s", clientURL, clientSID)
	if clientToken != "" {
		url += fmt.Sprintf("?access_token=%s", clientToken)
	}

	return url
}

func clientMain(cmd *cobra.Command, args []string) error {
	go clientThread(cmd, args)
	gst.MainLoop()
	return nil
}

func clientThread(cmd *cobra.Command, args []string) error {
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

	c.OnTrack = func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver, pc *webrtc.PeerConnection) {
		log.Debugf("Client got track: %#v", t)

		go func() {
			var videoTrack, audioTrack *webrtc.TrackRemote
			switch t.Kind() {
			case webrtc.RTPCodecTypeVideo:
				videoTrack = t
			case webrtc.RTPCodecTypeAudio:
				audioTrack = t
			}

			log.Debugf("client pipeline starting: ", t)
			pipeline := gst.CreateClientPipeline(audioTrack, videoTrack)
			pipeline.Start()
			buf := make([]byte, 1400)
			for {
				i, _, readErr := t.Read(buf)
				if readErr != nil {
					panic(err)
				}
				pipeline.Push(buf[:i], t.ID())
			}
		}()
	}

	if err := c.Join(clientSID); err != nil {
		return err
	}

	log.Debugf("starting producer")

	var producer *client.GSTProducer
	if len(args) > 0 {
		switch args[0] {
		case "test":
			producer = client.NewGSTProducer(c, "video", "")
		default:
			producer = client.NewGSTProducer(c, "screen", args[0])
		}
	}

	if producer != nil {
		log.Debugf("publishing tracks")
		if err := c.Publish(producer); err != nil {
			log.Errorf("error publishing tracks: %v", err)
			return err
		}

		log.Debugf("tracks published")
	}

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
