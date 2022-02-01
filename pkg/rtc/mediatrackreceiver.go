package rtc

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/livekit/livekit-server/pkg/rtc/types"

	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils"
	"github.com/pion/rtcp"

	"github.com/livekit/livekit-server/pkg/sfu"
	"github.com/livekit/livekit-server/pkg/sfu/buffer"
	"github.com/livekit/livekit-server/pkg/telemetry"
)

const (
	downLostUpdateDelta     = time.Second
	layerSelectionTolerance = 0.9
)

type MediaTrackReceiver struct {
	params      MediaTrackReceiverParams
	muted       utils.AtomicFlag
	simulcasted utils.AtomicFlag

	lock            sync.RWMutex
	receiver        sfu.TrackReceiver
	layerDimensions sync.Map // types.VideoQuality => *types.VideoLayer

	// track audio fraction lost
	downFracLostLock   sync.Mutex
	maxDownFracLost    uint8
	maxDownFracLostTs  time.Time
	onMediaLossUpdate  func(fractionalLoss uint8)
	onVideoLayerUpdate func(layers []*types.VideoLayer)

	onClose []func()

	*MediaTrackSubscriptions
}

type MediaTrackReceiverParams struct {
	TrackInfo           *types.TrackInfo
	MediaTrack          types.MediaTrack
	ParticipantID       types.ParticipantID
	ParticipantIdentity types.ParticipantIdentity
	BufferFactory       *buffer.Factory
	ReceiverConfig      ReceiverConfig
	SubscriberConfig    DirectionConfig
	Telemetry           telemetry.TelemetryService
	Logger              logger.Logger
}

func NewMediaTrackReceiver(params MediaTrackReceiverParams) *MediaTrackReceiver {
	t := &MediaTrackReceiver{
		params: params,
	}

	t.MediaTrackSubscriptions = NewMediaTrackSubscriptions(MediaTrackSubscriptionsParams{
		MediaTrack:       params.MediaTrack,
		BufferFactory:    params.BufferFactory,
		ReceiverConfig:   params.ReceiverConfig,
		SubscriberConfig: params.SubscriberConfig,
		Telemetry:        params.Telemetry,
		Logger:           params.Logger,
	})

	if params.TrackInfo.Muted {
		t.SetMuted(true)
	}

	if params.TrackInfo != nil && t.Kind() == types.TrackType_VIDEO {
		t.UpdateVideoLayers(params.TrackInfo.Layers)
		// LK-TODO: maybe use this or simulcast flag in TrackInfo to set simulcasted here
	}

	return t
}

func (t *MediaTrackReceiver) SetupReceiver(receiver sfu.TrackReceiver) {
	t.lock.Lock()
	t.receiver = receiver
	t.lock.Unlock()

	t.MediaTrackSubscriptions.Start()
}

func (t *MediaTrackReceiver) ClearReceiver() {
	t.lock.Lock()
	t.receiver = nil
	t.lock.Unlock()

	t.MediaTrackSubscriptions.Close()
}

func (t *MediaTrackReceiver) OnMediaLossUpdate(f func(fractionalLoss uint8)) {
	t.onMediaLossUpdate = f
}

func (t *MediaTrackReceiver) OnVideoLayerUpdate(f func(layers []*types.VideoLayer)) {
	t.onVideoLayerUpdate = f
}

func (t *MediaTrackReceiver) Close() {
	t.lock.Lock()
	t.receiver = nil
	onclose := t.onClose
	t.lock.Unlock()

	t.MediaTrackSubscriptions.Close()

	for _, f := range onclose {
		f()
	}
}

func (t *MediaTrackReceiver) ID() types.TrackID {
	return types.TrackID(t.params.TrackInfo.Sid)
}

func (t *MediaTrackReceiver) Kind() types.TrackType {
	return t.params.TrackInfo.Type
}

func (t *MediaTrackReceiver) Source() types.TrackSource {
	return t.params.TrackInfo.Source
}

func (t *MediaTrackReceiver) PublisherID() types.ParticipantID {
	return t.params.ParticipantID
}

func (t *MediaTrackReceiver) PublisherIdentity() types.ParticipantIdentity {
	return t.params.ParticipantIdentity
}

func (t *MediaTrackReceiver) IsSimulcast() bool {
	return t.simulcasted.Get()
}

func (t *MediaTrackReceiver) SetSimulcast(simulcast bool) {
	t.simulcasted.TrySet(simulcast)
}

func (t *MediaTrackReceiver) Name() string {
	return t.params.TrackInfo.Name
}

func (t *MediaTrackReceiver) IsMuted() bool {
	return t.muted.Get()
}

func (t *MediaTrackReceiver) SetMuted(muted bool) {
	t.muted.TrySet(muted)

	t.lock.RLock()
	if t.receiver != nil {
		t.receiver.SetUpTrackPaused(muted)
	}
	t.lock.RUnlock()

	t.MediaTrackSubscriptions.SetMuted(muted)
}

func (t *MediaTrackReceiver) AddOnClose(f func()) {
	if f == nil {
		return
	}

	t.lock.Lock()
	t.onClose = append(t.onClose, f)
	t.lock.Unlock()
}

// AddSubscriber subscribes sub to current mediaTrack
func (t *MediaTrackReceiver) AddSubscriber(sub types.LocalParticipant) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.receiver == nil {
		// cannot add, no receiver
		return errors.New("cannot subscribe without a receiver in place")
	}

	// using DownTrack from ion-sfu
	streamId := string(t.PublisherID())
	if sub.ProtocolVersion().SupportsPackedStreamId() {
		// when possible, pack both IDs in streamID to allow new streams to be generated
		// react-native-webrtc still uses stream based APIs and require this
		streamId = PackStreamID(t.PublisherID(), t.ID())
	}

	downTrack, err := t.MediaTrackSubscriptions.AddSubscriber(sub, t.receiver.Codec(), NewWrappedReceiver(t.receiver, t.ID(), streamId))
	if err != nil {
		return err
	}

	if downTrack != nil {
		if t.Kind() == types.TrackType_AUDIO {
			downTrack.AddReceiverReportListener(t.handleMaxLossFeedback)
		}

		t.receiver.AddDownTrack(downTrack)
	}
	return nil
}

func (t *MediaTrackReceiver) UpdateTrackInfo(ti *types.TrackInfo) {
	t.params.TrackInfo = ti
	if ti != nil && t.Kind() == types.TrackType_VIDEO {
		t.UpdateVideoLayers(ti.Layers)
	}
}

func (t *MediaTrackReceiver) TrackInfo() *types.TrackInfo {
	return t.params.TrackInfo
}

func (t *MediaTrackReceiver) UpdateVideoLayers(layers []*types.VideoLayer) {
	for _, layer := range layers {
		t.layerDimensions.Store(layer.Quality, layer)
	}

	t.MediaTrackSubscriptions.UpdateVideoLayers()
	if t.onVideoLayerUpdate != nil {
		t.onVideoLayerUpdate(layers)
	}

	// TODO: this might need to trigger a participant update for clients to pick up dimension change
}

func (t *MediaTrackReceiver) GetVideoLayers() []*types.VideoLayer {
	layers := make([]*types.VideoLayer, 0)
	t.layerDimensions.Range(func(q, val interface{}) bool {
		if layer, ok := val.(*types.VideoLayer); ok {
			layers = append(layers, layer)
		}
		return true
	})

	return layers
}

// GetQualityForDimension finds the closest quality to use for desired dimensions
// affords a 20% tolerance on dimension
func (t *MediaTrackReceiver) GetQualityForDimension(width, height uint32) types.VideoQuality {
	quality := types.VideoQuality_HIGH
	if t.Kind() == types.TrackType_AUDIO || t.params.TrackInfo.Height == 0 {
		return quality
	}
	origSize := t.params.TrackInfo.Height
	requestedSize := height
	if t.params.TrackInfo.Width < t.params.TrackInfo.Height {
		// for portrait videos
		origSize = t.params.TrackInfo.Width
		requestedSize = width
	}

	// default sizes representing qualities low - high
	layerSizes := []uint32{180, 360, origSize}
	var providedSizes []uint32
	t.layerDimensions.Range(func(_, val interface{}) bool {
		if layer, ok := val.(*types.VideoLayer); ok {
			providedSizes = append(providedSizes, layer.Height)
		}
		return true
	})
	if len(providedSizes) > 0 {
		layerSizes = providedSizes
		// comparing height always
		requestedSize = height
		sort.Slice(layerSizes, func(i, j int) bool {
			return layerSizes[i] < layerSizes[j]
		})
	}

	// finds the lowest layer that could satisfy client demands
	requestedSize = uint32(float32(requestedSize) * layerSelectionTolerance)
	for i, s := range layerSizes {
		quality = types.VideoQuality(i)
		if s >= requestedSize {
			break
		}
	}

	return quality
}

// handles max loss for audio packets
func (t *MediaTrackReceiver) handleMaxLossFeedback(_ *sfu.DownTrack, report *rtcp.ReceiverReport) {
	t.downFracLostLock.Lock()
	for _, rr := range report.Reports {
		if t.maxDownFracLost < rr.FractionLost {
			t.maxDownFracLost = rr.FractionLost
		}
	}
	t.downFracLostLock.Unlock()

	t.maybeUpdateLoss()
}

func (t *MediaTrackReceiver) NotifySubscriberNodeMediaLoss(_nodeID string, fractionalLoss uint8) {
	t.downFracLostLock.Lock()
	if t.maxDownFracLost < fractionalLoss {
		t.maxDownFracLost = fractionalLoss
	}
	t.downFracLostLock.Unlock()

	t.maybeUpdateLoss()
}

func (t *MediaTrackReceiver) maybeUpdateLoss() {
	var (
		shouldUpdate bool
		maxLost      uint8
	)

	t.downFracLostLock.Lock()
	now := time.Now()
	if now.Sub(t.maxDownFracLostTs) > downLostUpdateDelta {
		shouldUpdate = true
		maxLost = t.maxDownFracLost
		t.maxDownFracLost = 0
		t.maxDownFracLostTs = now
	}
	t.downFracLostLock.Unlock()

	if shouldUpdate {
		if t.onMediaLossUpdate != nil {
			t.onMediaLossUpdate(maxLost)
		}
	}
}

func (t *MediaTrackReceiver) DebugInfo() map[string]interface{} {
	info := map[string]interface{}{
		"ID":       t.ID(),
		"Kind":     t.Kind().String(),
		"PubMuted": t.muted.Get(),
	}

	info["DownTracks"] = t.MediaTrackSubscriptions.DebugInfo()

	t.lock.RLock()
	if t.receiver != nil {
		receiverInfo := t.receiver.DebugInfo()
		for k, v := range receiverInfo {
			info[k] = v
		}
	}
	t.lock.RUnlock()

	return info
}

func (t *MediaTrackReceiver) Receiver() sfu.TrackReceiver {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.receiver
}

func (t *MediaTrackReceiver) OnSubscribedMaxQualityChange(f func(trackID types.TrackID, subscribedQualities []*types.SubscribedQuality, maxSubscribedQuality types.VideoQuality) error) {
	t.MediaTrackSubscriptions.OnSubscribedMaxQualityChange(func(subscribedQualities []*types.SubscribedQuality, maxSubscribedQuality types.VideoQuality) {
		if f != nil && !t.IsMuted() {
			_ = f(t.ID(), subscribedQualities, maxSubscribedQuality)
		}

		t.lock.RLock()
		if t.receiver != nil {
			t.receiver.SetMaxExpectedSpatialLayer(SpatialLayerForQuality(maxSubscribedQuality))
		}
		t.lock.RUnlock()
	})
}

// ---------------------------

func SpatialLayerForQuality(quality types.VideoQuality) int32 {
	switch quality {
	case types.VideoQuality_LOW:
		return 0
	case types.VideoQuality_MEDIUM:
		return 1
	case types.VideoQuality_HIGH:
		return 2
	case types.VideoQuality_OFF:
		return -1
	default:
		return -1
	}
}

func QualityForSpatialLayer(layer int32) types.VideoQuality {
	switch layer {
	case 0:
		return types.VideoQuality_LOW
	case 1:
		return types.VideoQuality_MEDIUM
	case 2:
		return types.VideoQuality_HIGH
	case sfu.InvalidLayerSpatial:
		return types.VideoQuality_OFF
	default:
		return types.VideoQuality_OFF
	}
}

func VideoQualityToRID(q types.VideoQuality) string {
	switch q {
	case types.VideoQuality_HIGH:
		return sfu.FullResolution
	case types.VideoQuality_MEDIUM:
		return sfu.HalfResolution
	default:
		return sfu.QuarterResolution
	}
}
