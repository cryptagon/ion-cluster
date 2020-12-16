package client

import (
	"fmt"
	"log"

	"github.com/lucsky/cuid"
	"github.com/pion/ion-cluster/pkg/client/gst"
	"github.com/pion/webrtc/v3"
)

//producer interface
type producer interface {
	Start()
	Stop()

	AudioTrack() *webrtc.TrackLocalStaticSample
	VideoTrack() *webrtc.TrackLocalStaticSample
}

// GSTProducer will produce audio + video from a gstreamer pipeline and can be published to a client
type GSTProducer struct {
	name       string
	audioTrack *webrtc.TrackLocalStaticSample
	videoTrack *webrtc.TrackLocalStaticSample
	pipeline   *gst.Pipeline
	paused     bool
}

// NewGSTProducer will create a new producer for a given client and a videoFile
func NewGSTProducer(c *Client, kind string, path string) *GSTProducer {
	stream := fmt.Sprintf("gst-%v-%v", kind, cuid.New())
	videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/h264"}, cuid.New(), stream)
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, cuid.New(), stream)
	if err != nil {
		log.Fatal(err)
	}

	var pipeline *gst.Pipeline
	if path != "" {
		pipeline = gst.CreatePipeline(path, audioTrack, videoTrack)
	} else {
		pipeline = gst.CreateTestSrcPipeline(audioTrack, videoTrack)
	}

	return &GSTProducer{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		pipeline:   pipeline,
	}
}

//AudioTrack returns the audio track for the pipeline
func (t *GSTProducer) AudioTrack() *webrtc.TrackLocalStaticSample {
	return t.audioTrack
}

//VideoTrack returns the video track for the pipeline
func (t *GSTProducer) VideoTrack() *webrtc.TrackLocalStaticSample {
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
