package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0

#include "gst.h"

*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"

	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
)

// CompositorPipeline will decode incoming tracks in a single pipeline and compose the streams
type CompositorPipeline struct {
	Pipeline *C.GstElement

	trackBins map[string]*C.GstElement
}

// NewCompositor will create a new producer for a given client and a videoFile
func NewCompositor() *CompositorPipeline {
	pipelineStr := fmt.Sprintf(`compositor name=vmix ! queue ! glimagesink sync=false videotestsrc ! video/x-raw,width=1920,height=1080,format=UYVY ! vmix.`)
	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	c := &CompositorPipeline{
		Pipeline:  C.gstreamer_create_pipeline(pipelineStrUnsafe),
		trackBins: make(map[string]*C.GstElement),
	}
	// runtime.SetFinalizer(c, func(c *CompositorPipeline) {
	// 	// c.destroy()
	// })
	C.gstreamer_start_pipeline(c.Pipeline)
	return c
}

func (c *CompositorPipeline) AddInputTrack(t *webrtc.TrackRemote) {
	inputBin := fmt.Sprintf("appsrc format=time is-live=true do-timestamp=true name=%s ! application/x-rtp ", t.ID())

	switch strings.ToLower(t.Codec().MimeType) {
	case "audio/opus":
		inputBin += ", encoding-name=OPUS, payload=96 ! rtpopusdepay ! queue ! decodebin "
	case "audio/g722":
		inputBin += " clock-rate=8000 ! rtpg722depay ! decodebin "
	case "video/vp8":
		inputBin += ", encoding-name=VP8-DRAFT-IETF-01 ! rtpvp8depay ! decodebin "
	case "viode/vp9":
		inputBin += " ! rtpvp9depay ! decodebin "
	case "video/h264":
		inputBin += fmt.Sprintf(", payload=%d ! rtph264depay ! h264parse config-interval=1 ! queue ! %s !  queue ", t.PayloadType(), getDecoderString())
	default:
		panic(fmt.Sprintf("couldn't build gst pipeline for codec: %s ", t.Codec().MimeType))
	}

	log.Debugf("adding input track with bin: %s", inputBin)
	inputBinUnsafe := C.CString(inputBin)
	// defer C.free(unsafe.Pointer(&inputBinUnsafe))

	isVideo := t.Kind() == webrtc.RTPCodecTypeVideo
	bin := C.gstreamer_compositor_add_input_track(c.Pipeline, inputBinUnsafe, C.bool(isVideo))
	c.trackBins[t.ID()] = bin
	go c.bindTrackToAppsrc(t)
}

func (c *CompositorPipeline) Play() {
	C.gstreamer_play_pipeline(c.Pipeline)
}

func (c *CompositorPipeline) destroy() {
	for _, b := range c.trackBins {
		C.gst_object_unref(C.gpointer(b))
	}
}

func (c *CompositorPipeline) bindTrackToAppsrc(t *webrtc.TrackRemote) {
	buf := make([]byte, 1400)
	for {
		i, _, readErr := t.Read(buf)
		if readErr != nil {
			log.Warnf("end of track %v: TODO CLEAN UP GST PIPELINE", t.ID())
			panic(readErr)
		}
		c.pushAppsrc(buf[:i], t.ID())
	}
}

// Push pushes a buffer on the appsrc of the GStreamer Pipeline
func (c *CompositorPipeline) pushAppsrc(buffer []byte, appsrc string) {
	b := C.CBytes(buffer)
	defer C.free(b)
	inputElementUnsafe := C.CString(appsrc)
	// defer C.free(unsafe.Pointer(&inputElementUnsafe))
	C.gstreamer_receive_push_buffer(c.Pipeline, b, C.int(len(buffer)), inputElementUnsafe)
}
