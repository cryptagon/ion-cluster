package rtc

import (
	"sync"

	"github.com/pion/ion-cluster/pkg/sfu"
)

type SubscribedTrack struct {
	mu   sync.Mutex
	peer Peer

	downtrack sfu.DownTrack

	layer int
}
