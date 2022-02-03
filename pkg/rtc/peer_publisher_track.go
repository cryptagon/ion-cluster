package rtc

import (
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/webrtc/v3"
)

type PublishedTrack struct {
	Track    *webrtc.TrackRemote
	Receiver sfu.TrackReceiver
}
