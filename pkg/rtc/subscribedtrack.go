package rtc

import (
	"sync/atomic"
	"time"

	"github.com/bep/debounce"
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/ion-cluster/pkg/types"
	"github.com/pion/webrtc/v3"
)

const (
	subscriptionDebounceInterval = 100 * time.Millisecond
)

type SubscribedTrackParams struct {
	PublisherID       types.ParticipantID
	PublisherIdentity types.ParticipantIdentity
	SubscriberID      types.ParticipantID
	MediaTrack        MediaTrack
	DownTrack         *sfu.DownTrack
}
type SubscribedTrack struct {
	params   SubscribedTrackParams
	subMuted AtomicFlag
	pubMuted AtomicFlag
	settings atomic.Value // *types.UpdateTrackSettings

	onBind func()

	debouncer func(func())
}

func NewSubscribedTrack(params SubscribedTrackParams) *SubscribedTrack {
	return &SubscribedTrack{
		params:    params,
		debouncer: debounce.New(subscriptionDebounceInterval),
	}
}

func (t *SubscribedTrack) OnBind(f func()) {
	t.onBind = f
}

func (t *SubscribedTrack) Bound() {
	if t.onBind != nil {
		t.onBind()
	}
}

func (t *SubscribedTrack) ID() types.TrackID {
	return types.TrackID(t.params.DownTrack.ID())
}

func (t *SubscribedTrack) PublisherID() types.ParticipantID {
	return t.params.PublisherID
}

func (t *SubscribedTrack) PublisherIdentity() types.ParticipantIdentity {
	return t.params.PublisherIdentity
}

func (t *SubscribedTrack) SubscriberID() types.ParticipantID {
	return t.params.SubscriberID
}

func (t *SubscribedTrack) DownTrack() *sfu.DownTrack {
	return t.params.DownTrack
}

func (t *SubscribedTrack) MediaTrack() types.MediaTrack {
	return t.params.MediaTrack
}

// has subscriber indicated it wants to mute this track
func (t *SubscribedTrack) IsMuted() bool {
	return t.subMuted.Get()
}

func (t *SubscribedTrack) SetPublisherMuted(muted bool) {
	t.pubMuted.TrySet(muted)
	t.updateDownTrackMute()
}

func (t *SubscribedTrack) UpdateSubscriberSettings(settings *types.UpdateTrackSettings) {
	visibilityChanged := t.subMuted.TrySet(settings.Disabled)
	t.settings.Store(settings)
	// avoid frequent changes to mute & video layers, unless it became visible
	if visibilityChanged && !settings.Disabled {
		t.UpdateVideoLayer()
	} else {
		t.debouncer(t.UpdateVideoLayer)
	}
}

func (t *SubscribedTrack) UpdateVideoLayer() {
	t.updateDownTrackMute()
	if t.DownTrack().Kind() != webrtc.RTPCodecTypeVideo {
		return
	}

	settings, ok := t.settings.Load().(*types.UpdateTrackSettings)
	if !ok {
		return
	}

	quality := settings.Quality
	if settings.Width > 0 {
		quality = t.MediaTrack().GetQualityForDimension(settings.Width, settings.Height)
	}
	t.DownTrack().SetMaxSpatialLayer(SpatialLayerForQuality(quality))
}

func (t *SubscribedTrack) updateDownTrackMute() {
	muted := t.subMuted.Get() || t.pubMuted.Get()
	t.DownTrack().Mute(muted)
}
