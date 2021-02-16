package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0

#include "gst.h"

*/
import "C"
import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

func init() {
	go C.gstreamer_send_start_mainloop()
}

// Pipeline is a wrapper for a GStreamer Pipeline
type Pipeline struct {
	Pipeline   *C.GstElement
	audioTrack *webrtc.TrackLocalStaticSample
	videoTrack *webrtc.TrackLocalStaticSample
}

var pipeline = &Pipeline{}
var pipelinesLock sync.Mutex

func getEncoderString() string {
	if runtime.GOOS == "darwin" {
		return "vtenc_h264 realtime=true allow-frame-reordering=false max-keyframe-interval=60 ! video/x-h264, profile=baseline ! h264parse config-interval=1"
	}
	return "x264enc bframes=0 speed-preset=ultrafast key-int-max=60 ! video/x-h264, profile=baseline ! h264parse config-interval=1"
}

// CreatePipeline creates a GStreamer Pipeline
func CreatePipeline(containerPath string, audioTrack, videoTrack *webrtc.TrackLocalStaticSample) *Pipeline {
	pipelineStr := fmt.Sprintf(`
		filesrc location="%s" !
		decodebin name=demux !
			queue !
			%s ! 
			video/x-h264,stream-format=byte-stream,profile=baseline !
			appsink name=video 
		demux. ! 
			queue ! 
			audioconvert ! 
			audioresample ! 
			audio/x-raw,rate=48000,channels=2 !
			opusenc ! appsink name=audio
	`, containerPath, getEncoderString())

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()
	pipeline = &Pipeline{
		Pipeline:   C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		audioTrack: audioTrack,
		videoTrack: videoTrack,
	}
	return pipeline
}

// CreateTestSrcPipeline creates a GStreamer Pipeline with test sources
func CreateTestSrcPipeline(audioTrack, videoTrack *webrtc.TrackLocalStaticSample) *Pipeline {
	pipelineStr := fmt.Sprintf(`
		videotestsrc ! 
			video/x-raw,width=1280,height=720 !
			queue !
			%s ! 
			video/x-h264,stream-format=byte-stream,profile=baseline !
			appsink name=video 
		audiotestsrc wave=6 ! 
			queue ! 
			audioconvert ! 
			audioresample ! 
			audio/x-raw,rate=48000,channels=2 !
			opusenc ! appsink name=audio
	`, getEncoderString())

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()
	pipeline = &Pipeline{
		Pipeline:   C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		audioTrack: audioTrack,
		videoTrack: videoTrack,
	}
	return pipeline
}

// Start starts the GStreamer Pipeline
func (p *Pipeline) Start() {
	// This will signal to goHandlePipelineBuffer
	// and provide a method for cancelling sends.
	C.gstreamer_send_start_pipeline(p.Pipeline)
}

// Play sets the pipeline to PLAYING
func (p *Pipeline) Play() {
	C.gstreamer_send_play_pipeline(p.Pipeline)
}

// Pause sets the pipeline to PAUSED
func (p *Pipeline) Pause() {
	C.gstreamer_send_pause_pipeline(p.Pipeline)
}

// SeekToTime seeks on the pipeline
func (p *Pipeline) SeekToTime(seekPos int64) {
	C.gstreamer_send_seek(p.Pipeline, C.int64_t(seekPos))
}

const (
	videoClockRate = 90000
	audioClockRate = 48000
)

//export goHandlePipelineBuffer
func goHandlePipelineBuffer(buffer unsafe.Pointer, bufferLen C.int, duration C.int, isVideo C.int) {
	var track *webrtc.TrackLocalStaticSample

	if isVideo == 1 {
		// samples = uint32(videoClockRate * (float32(duration) / 1000000000))
		track = pipeline.videoTrack
	} else {
		// samples = uint32(audioClockRate * (float32(duration) / 1000000000))
		track = pipeline.audioTrack
	}

	goDuration := time.Duration(duration)
	// log.Debugf("writing buffer: duration=%v", duration)

	if err := track.WriteSample(media.Sample{Data: C.GoBytes(buffer, bufferLen), Duration: goDuration}); err != nil && err != io.ErrClosedPipe {
		panic(err)
	}

	C.free(buffer)
}
