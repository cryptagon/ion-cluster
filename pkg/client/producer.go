package client

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/lucsky/cuid"
	"github.com/pion/ion-cluster/pkg/client/gst"
	"github.com/pion/webrtc/v3"
)

//producer interface
type producer interface {
	Start()
	Stop()

	AudioTrack() *webrtc.Track
	VideoTrack() *webrtc.Track
}

// GSTProducer will produce audio + video from a gstreamer pipeline and can be published to a client
type GSTProducer struct {
	name       string
	audioTrack *webrtc.Track
	videoTrack *webrtc.Track
	pipeline   *gst.Pipeline
	paused     bool
}

// NewGSTProducer will create a new producer for a given client and a videoFile
func NewGSTProducer(c *Client, path string) *GSTProducer {
	stream := fmt.Sprintf("gst-video-%v", cuid.New())
	videoTrack, err := c.pub.pc.NewTrack(webrtc.DefaultPayloadTypeH264, rand.Uint32(), cuid.New(), stream)
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err := c.pub.pc.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), cuid.New(), stream)
	if err != nil {
		log.Fatal(err)
	}

	pipeline := gst.CreatePipeline(path, audioTrack, videoTrack)

	return &GSTProducer{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		pipeline:   pipeline,
	}
}

//AudioTrack returns the audio track for the pipeline
func (t *GSTProducer) AudioTrack() *webrtc.Track {
	return t.audioTrack
}

//VideoTrack returns the video track for the pipeline
func (t *GSTProducer) VideoTrack() *webrtc.Track {
	return t.videoTrack
}

//SeekP to a timestamp
func (t *GSTProducer) SeekP(ts int) {
	t.pipeline.SeekToTime(int64(ts))
}

//Pause the pipeline
func (t *GSTProducer) Pause(pause bool) {
	if pause {
		t.pipeline.Pause()
	} else {
		t.pipeline.Play()
	}
}

//Stop the pipeline
func (t *GSTProducer) Stop() {
}

//Start the pipeline
func (t *GSTProducer) Start() {
	t.pipeline.Start()
}
