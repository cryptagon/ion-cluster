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
	"github.com/spf13/cobra"
)

var compositeCmd = &cobra.Command{
	Use:   "composite",
	Short: "Connect to an ion-cluster server as a client",
	RunE:  compositeMain,
}

var compositeStreamURL string
var compositeSavePath string

func init() {
	compositeCmd.PersistentFlags().StringVarP(&clientURL, "url", "u", "ws://localhost:7000", "sfu host to connect to")
	compositeCmd.PersistentFlags().StringVarP(&clientSID, "sid", "s", "test-session", "session id to join")
	compositeCmd.PersistentFlags().StringVarP(&clientToken, "token", "t", "", "jwt access token")

	compositeCmd.PersistentFlags().StringVarP(&compositeSavePath, "save", "", "", "filepath to save video")
	compositeCmd.PersistentFlags().StringVarP(&compositeStreamURL, "stream", "", "", "rtmp url for streaming")

	rootCmd.AddCommand(compositeCmd)
}

func compositeMain(cmd *cobra.Command, args []string) error {
	runtime.LockOSThread()
	go compositeThread(cmd, args)
	gst.MainLoop()
	return nil
}

func compositeThread(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	w := webrtc.Configuration{}

	signal := client.NewJSONRPCSignalClient(ctx)
	c, err := client.NewClient(signal, &w, []interceptor.Interceptor{})
	if err != nil {
		log.Error(err, "error initializing client")
	}

	fmt.Printf("client connecting to %v", endpoint())

	signalClosedCh, err := signal.Open(endpoint())
	if err != nil {
		return err
	}

	encodePipeline := ""
	if compositeSavePath != "" || compositeStreamURL != "" {
		encodePipeline = fmt.Sprintf(`
				tee name=aenctee 
				tee name=venctee
				vtee. ! queue ! vtenc_h264 ! video/x-h264,chroma-site=mpeg2 ! venctee.
				atee. ! queue ! faac ! aenctee.
		`)

		log.Info("encoding composited stream")

		if compositeSavePath != "" {
			encodePipeline += fmt.Sprintf(`
				qtmux name=savemux ! queue ! filesink location=%s async=false sync=false
				venctee. ! queue ! savemux.
				aenctee. ! queue ! savemux. 
			`, compositeSavePath)
			log.Info("saving encoded stream", "path", compositeSavePath)
		}

		if compositeStreamURL != "" {
			encodePipeline += fmt.Sprintf(`
				flvmux name=streammux ! queue ! rtmpsink location=%s async=false sync=false
				venctee. ! queue ! streammux.
				aenctee. ! queue ! streammux. 
			`, compositeStreamURL)
			log.Info("streaming rtmp", "url", compositeStreamURL)
		}
	} else {
		log.Info("local compositing only")
	}

	compositor := gst.NewCompositorPipeline(encodePipeline)
	compositor.Play()

	c.OnTrack = func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver, pc *webrtc.PeerConnection) {
		log.Info("Client got track: %#v", t)
		compositor.AddInputTrack(t, pc)

	}

	if err := c.Join(clientSID); err != nil {
		return err
	}

	log.Info("starting producer")

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
				log.Error(err, "signal ping err")
			}
			log.Info("signal ping got pong")
		case sig := <-sigs:
			log.Info("got signal", "signal", sig)
			signal.Close()
		case <-signalClosedCh:
			log.Info("signal closed")
			compositor.Stop()
			return nil
		}
	}

}
