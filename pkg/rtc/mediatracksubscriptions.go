package rtc

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/ion-cluster/pkg/logger"
	sfu "github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/ion-cluster/pkg/sfu/buffer"
	"github.com/pion/ion-cluster/pkg/types"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/rtcerr"
)

const (
	initialQualityUpdateWait = 10 * time.Second
)

// MediaTrackSubscriptions manages subscriptions of a media track
type MediaTrackSubscriptions struct {
	params MediaTrackSubscriptionsParams

	subscribedTracksMu sync.RWMutex
	subscribedTracks   map[types.ParticipantID]SubscribedTrack // participantID => SubscribedTrack

	onNoSubscribers func()

	// quality level enable/disable
	maxQualityLock               sync.RWMutex
	maxSubscriberQuality         map[types.ParticipantID]types.VideoQuality
	maxSubscriberNodeQuality     map[string]types.VideoQuality // nodeID => types.VideoQuality
	maxSubscribedQuality         types.VideoQuality
	onSubscribedMaxQualityChange func(subscribedQualities []*types.SubscribedQuality, maxSubscribedQuality types.VideoQuality)
	maxQualityTimer              *time.Timer
}

type MediaTrackSubscriptionsParams struct {
	MediaTrack *MediaTrack

	BufferFactory    *buffer.Factory
	ReceiverConfig   ReceiverConfig
	SubscriberConfig DirectionConfig

	// Telemetry telemetry.TelemetryService

	Logger logger.Logger
}

func NewMediaTrackSubscriptions(params MediaTrackSubscriptionsParams) *MediaTrackSubscriptions {
	t := &MediaTrackSubscriptions{
		params:                   params,
		subscribedTracks:         make(map[types.ParticipantID]SubscribedTrack),
		maxSubscriberQuality:     make(map[types.ParticipantID]types.VideoQuality),
		maxSubscriberNodeQuality: make(map[string]types.VideoQuality),
	}

	return t
}

func (t *MediaTrackSubscriptions) Start() {
	t.startMaxQualityTimer()
}

func (t *MediaTrackSubscriptions) Close() {
	t.stopMaxQualityTimer()
}

func (t *MediaTrackSubscriptions) OnNoSubscribers(f func()) {
	t.onNoSubscribers = f
}

func (t *MediaTrackSubscriptions) SetMuted(muted bool) {
	// update mute of all subscribed tracks
	for _, st := range t.getAllSubscribedTracks() {
		st.SetPublisherMuted(muted)
	}

	// update quality based on subscription if unmuting
	if !muted {
		t.UpdateQualityChange(true)
	}
}

func (t *MediaTrackSubscriptions) IsSubscriber(subID types.ParticipantID) bool {
	t.subscribedTracksMu.RLock()
	defer t.subscribedTracksMu.RUnlock()

	_, ok := t.subscribedTracks[subID]
	return ok
}

// AddSubscriber subscribes sub to current mediaTrack
func (t *MediaTrackSubscriptions) AddSubscriber(sub Peer, codec webrtc.RTPCodecCapability, wr WrappedReceiver) (*sfu.DownTrack, error) {
	subscriberID := sub.ID()

	t.subscribedTracksMu.Lock()
	defer t.subscribedTracksMu.Unlock()

	// don't subscribe to the same track multiple times
	if _, ok := t.subscribedTracks[subscriberID]; ok {
		return nil, nil
	}

	var rtcpFeedback []webrtc.RTCPFeedback
	switch t.params.MediaTrack.Kind() {
	case types.TrackType_AUDIO:
		rtcpFeedback = t.params.SubscriberConfig.RTCPFeedback.Audio
	case types.TrackType_VIDEO:
		rtcpFeedback = t.params.SubscriberConfig.RTCPFeedback.Video
	}
	downTrack, err := sfu.NewDownTrack(
		webrtc.RTPCodecCapability{
			MimeType:     codec.MimeType,
			ClockRate:    codec.ClockRate,
			Channels:     codec.Channels,
			SDPFmtpLine:  codec.SDPFmtpLine,
			RTCPFeedback: rtcpFeedback,
		},
		wr,
		t.params.BufferFactory,
		subscriberID,
		t.params.ReceiverConfig.PacketBufferSize,
		LoggerWithParticipant(t.params.Logger, sub.Identity(), sub.ID()),
	)
	if err != nil {
		return nil, err
	}

	subTrack := NewSubscribedTrack(SubscribedTrackParams{
		PublisherID:       t.params.MediaTrack.PublisherID(),
		PublisherIdentity: t.params.MediaTrack.PublisherIdentity(),
		SubscriberID:      subscriberID,
		MediaTrack:        t.params.MediaTrack,
		DownTrack:         downTrack,
	})

	var transceiver *webrtc.RTPTransceiver
	var sender *webrtc.RTPSender
	if sub.ProtocolVersion().SupportsTransceiverReuse() {
		//
		// AddTrack will create a new transceiver or re-use an unused one
		// if the attributes match. This prevents SDP from bloating
		// because of dormant transceivers building up.
		//
		sender, err = sub.SubscriberPC().AddTrack(downTrack)
		if err != nil {
			return nil, err
		}

		// as there is no way to get transceiver from sender, search
		for _, tr := range sub.SubscriberPC().GetTransceivers() {
			if tr.Sender() == sender {
				transceiver = tr
				break
			}
		}
		if transceiver == nil {
			// cannot add, no transceiver
			return nil, errors.New("cannot subscribe without a transceiver in place")
		}
	} else {
		transceiver, err = sub.SubscriberPC().AddTransceiverFromTrack(downTrack, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
		if err != nil {
			return nil, err
		}

		sender = transceiver.Sender()
		if sender == nil {
			// cannot add, no sender
			return nil, errors.New("cannot subscribe without a sender in place")
		}
	}

	sendParameters := sender.GetParameters()
	downTrack.SetRTPHeaderExtensions(sendParameters.HeaderExtensions)

	downTrack.SetTransceiver(transceiver)

	// when outtrack is bound, start loop to send reports
	downTrack.OnBind(func() {
		go subTrack.Bound()
		go t.sendDownTrackBindingReports(sub)
	})

	// trackID := t.params.MediaTrack.ID()
	downTrack.OnStatsUpdate(func(_ *sfu.DownTrack, stat *types.AnalyticsStat) {
		// t.params.Telemetry.TrackStats(types.StreamType_DOWNSTREAM, subscriberID, trackID, stat)
	})

	downTrack.OnMaxLayerChanged(func(dt *sfu.DownTrack, layer int32) {
		t.notifySubscriberMaxQuality(subscriberID, QualityForSpatialLayer(layer))
	})

	downTrack.OnCloseHandler(func() {
		t.subscribedTracksMu.Lock()
		delete(t.subscribedTracks, subscriberID)
		t.subscribedTracksMu.Unlock()

		t.maybeNotifyNoSubscribers()

		// t.params.Telemetry.TrackUnsubscribed(context.Background(), subscriberID, t.params.MediaTrack.ToProto())

		// ignore if the subscribing sub is not connected
		if sub.SubscriberPC().ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}

		// if the source has been terminated, we'll need to terminate all the subscribed tracks
		// however, if the dest sub has disconnected, then we can skip
		if sender == nil {
			return
		}
		t.params.Logger.Debugw("removing peerconnection track",
			"subscriber", sub.Identity(),
			"subscriberID", subscriberID,
			"kind", t.params.MediaTrack.Kind(),
		)
		if err := sub.SubscriberPC().RemoveTrack(sender); err != nil {
			if err == webrtc.ErrConnectionClosed {
				// sub closing, can skip removing subscribedtracks
				return
			}
			if _, ok := err.(*rtcerr.InvalidStateError); !ok {
				// most of these are safe to ignore, since the track state might have already
				// been set to Inactive
				t.params.Logger.Debugw("could not remove remoteTrack from forwarder",
					"error", err,
					"subscriber", sub.Identity(),
					"subscriberID", subscriberID,
				)
			}
		}

		sub.RemoveSubscribedTrack(subTrack)
		sub.Negotiate()
	})

	t.subscribedTracks[subscriberID] = subTrack
	subTrack.SetPublisherMuted(t.params.MediaTrack.IsMuted())

	// since sub will lock, run it in a goroutine to avoid deadlocks
	go func() {
		sub.AddSubscribedTrack(subTrack)
		sub.Negotiate()
	}()

	// t.params.Telemetry.TrackSubscribed(context.Background(), subscriberID, t.params.MediaTrack.ToProto())
	return downTrack, nil
}

// RemoveSubscriber removes participant from subscription
// stop all forwarders to the client
func (t *MediaTrackSubscriptions) RemoveSubscriber(participantID types.ParticipantID, resume bool) {
	subTrack := t.getSubscribedTrack(participantID)

	t.subscribedTracksMu.Lock()
	delete(t.subscribedTracks, participantID)
	t.subscribedTracksMu.Unlock()

	if subTrack != nil {
		subTrack.DownTrack().CloseWithFlush(!resume)
	}
}

func (t *MediaTrackSubscriptions) RemoveAllSubscribers() {
	t.params.Logger.Debugw("removing all subscribers")

	t.subscribedTracksMu.Lock()
	subscribedTracks := t.getAllSubscribedTracksLocked()
	t.subscribedTracks = make(map[types.ParticipantID]SubscribedTrack)
	t.subscribedTracksMu.Unlock()

	for _, subTrack := range subscribedTracks {
		subTrack.DownTrack().Close()
	}
}

func (t *MediaTrackSubscriptions) ResyncAllSubscribers() {
	t.params.Logger.Debugw("resyncing all subscribers")

	for _, subTrack := range t.getAllSubscribedTracks() {
		subTrack.DownTrack().Resync()
	}
}

func (t *MediaTrackSubscriptions) RevokeDisallowedSubscribers(allowedSubscriberIDs []types.ParticipantID) []types.ParticipantID {
	var revokedSubscriberIDs []types.ParticipantID

	// LK-TODO: large number of subscribers needs to be solved for this loop
	for _, subTrack := range t.getAllSubscribedTracks() {
		found := false
		for _, allowedID := range allowedSubscriberIDs {
			if subTrack.SubscriberID() == allowedID {
				found = true
				break
			}
		}

		if !found {
			go subTrack.DownTrack().Close()
			revokedSubscriberIDs = append(revokedSubscriberIDs, subTrack.SubscriberID())
		}
	}

	return revokedSubscriberIDs
}

func (t *MediaTrackSubscriptions) UpdateVideoLayers() {
	for _, st := range t.getAllSubscribedTracks() {
		st.UpdateVideoLayer()
	}
}

func (t *MediaTrackSubscriptions) getSubscribedTrack(subscriberID types.ParticipantID) SubscribedTrack {
	t.subscribedTracksMu.RLock()
	defer t.subscribedTracksMu.RUnlock()

	return t.subscribedTracks[subscriberID]
}

func (t *MediaTrackSubscriptions) getAllSubscribedTracks() []SubscribedTrack {
	t.subscribedTracksMu.RLock()
	defer t.subscribedTracksMu.RUnlock()

	return t.getAllSubscribedTracksLocked()
}

func (t *MediaTrackSubscriptions) getAllSubscribedTracksLocked() []SubscribedTrack {
	subTracks := make([]SubscribedTrack, 0, len(t.subscribedTracks))
	for _, subTrack := range t.subscribedTracks {
		subTracks = append(subTracks, subTrack)
	}
	return subTracks
}

// TODO: send for all down tracks from the source participant
// https://tools.ietf.org/html/rfc7941
func (t *MediaTrackSubscriptions) sendDownTrackBindingReports(sub Peer) {
	var sd []rtcp.SourceDescriptionChunk

	subTrack := t.getSubscribedTrack(sub.ID())
	if subTrack == nil {
		return
	}

	chunks := subTrack.DownTrack().CreateSourceDescriptionChunks()
	if chunks == nil {
		return
	}
	sd = append(sd, chunks...)

	pkts := []rtcp.Packet{
		&rtcp.SourceDescription{Chunks: sd},
	}

	go func() {
		defer RecoverSilent()
		batch := pkts
		i := 0
		for {
			if err := sub.SubscriberPC().WriteRTCP(batch); err != nil {
				t.params.Logger.Errorw("could not write RTCP", err)
				return
			}
			if i > 5 {
				return
			}
			i++
			time.Sleep(20 * time.Millisecond)
		}
	}()
}

func (t *MediaTrackSubscriptions) DebugInfo() []map[string]interface{} {
	subscribedTrackInfo := make([]map[string]interface{}, 0)
	for _, val := range t.getAllSubscribedTracks() {
		if st, ok := val.(*SubscribedTrack); ok {
			dt := st.DownTrack().DebugInfo()
			dt["PubMuted"] = st.pubMuted.Get()
			dt["SubMuted"] = st.subMuted.Get()
			subscribedTrackInfo = append(subscribedTrackInfo, dt)
		}
	}

	return subscribedTrackInfo
}

func (t *MediaTrackSubscriptions) OnSubscribedMaxQualityChange(f func(subscribedQualities []*types.SubscribedQuality, maxSubscribedQuality types.VideoQuality)) {
	t.onSubscribedMaxQualityChange = f
}

func (t *MediaTrackSubscriptions) notifySubscriberMaxQuality(subscriberID types.ParticipantID, quality types.VideoQuality) {
	if t.params.MediaTrack.Kind() != types.TrackType_VIDEO {
		return
	}

	t.maxQualityLock.Lock()
	if quality == types.VideoQuality_OFF {
		_, ok := t.maxSubscriberQuality[subscriberID]
		if !ok {
			t.maxQualityLock.Unlock()
			return
		}

		delete(t.maxSubscriberQuality, subscriberID)
	} else {
		maxQuality, ok := t.maxSubscriberQuality[subscriberID]
		if ok && maxQuality == quality {
			t.maxQualityLock.Unlock()
			return
		}

		t.maxSubscriberQuality[subscriberID] = quality
	}
	t.maxQualityLock.Unlock()

	t.UpdateQualityChange(false)
}

func (t *MediaTrackSubscriptions) NotifySubscriberNodeMaxQuality(nodeID string, quality types.VideoQuality) {
	if t.params.MediaTrack.Kind() != types.TrackType_VIDEO {
		return
	}

	t.maxQualityLock.Lock()
	if quality == types.VideoQuality_OFF {
		_, ok := t.maxSubscriberNodeQuality[nodeID]
		if !ok {
			t.maxQualityLock.Unlock()
			return
		}

		delete(t.maxSubscriberNodeQuality, nodeID)
	} else {
		maxQuality, ok := t.maxSubscriberNodeQuality[nodeID]
		if ok && maxQuality == quality {
			t.maxQualityLock.Unlock()
			return
		}

		t.maxSubscriberNodeQuality[nodeID] = quality
	}
	t.maxQualityLock.Unlock()

	t.UpdateQualityChange(false)
}

func (t *MediaTrackSubscriptions) UpdateQualityChange(force bool) {
	if t.params.MediaTrack.Kind() != types.TrackType_VIDEO {
		return
	}

	t.maxQualityLock.Lock()
	maxSubscribedQuality := types.VideoQuality_OFF
	for _, subQuality := range t.maxSubscriberQuality {
		if maxSubscribedQuality == types.VideoQuality_OFF || subQuality > maxSubscribedQuality {
			maxSubscribedQuality = subQuality
		}
	}

	for _, subQuality := range t.maxSubscriberNodeQuality {
		if maxSubscribedQuality == types.VideoQuality_OFF || subQuality > maxSubscribedQuality {
			maxSubscribedQuality = subQuality
		}
	}

	if maxSubscribedQuality == t.maxSubscribedQuality && !force {
		t.maxQualityLock.Unlock()
		return
	}

	t.maxSubscribedQuality = maxSubscribedQuality

	var subscribedQualities []*types.SubscribedQuality
	if t.maxSubscribedQuality == types.VideoQuality_OFF {
		subscribedQualities = []*types.SubscribedQuality{
			{Quality: types.VideoQuality_LOW, Enabled: false},
			{Quality: types.VideoQuality_MEDIUM, Enabled: false},
			{Quality: types.VideoQuality_HIGH, Enabled: false},
		}
	} else {
		for q := types.VideoQuality_LOW; q <= types.VideoQuality_HIGH; q++ {
			subscribedQualities = append(subscribedQualities, &types.SubscribedQuality{
				Quality: q,
				Enabled: q <= t.maxSubscribedQuality,
			})
		}
	}
	t.maxQualityLock.Unlock()

	if t.onSubscribedMaxQualityChange != nil {
		t.onSubscribedMaxQualityChange(subscribedQualities, maxSubscribedQuality)
	}
}

func (t *MediaTrackSubscriptions) startMaxQualityTimer() {
	t.maxQualityLock.Lock()
	defer t.maxQualityLock.Unlock()

	if t.params.MediaTrack.Kind() != types.TrackType_VIDEO {
		return
	}

	t.maxQualityTimer = time.AfterFunc(initialQualityUpdateWait, func() {
		t.stopMaxQualityTimer()
		t.UpdateQualityChange(false)
	})
}

func (t *MediaTrackSubscriptions) stopMaxQualityTimer() {
	t.maxQualityLock.Lock()
	defer t.maxQualityLock.Unlock()

	if t.maxQualityTimer != nil {
		t.maxQualityTimer.Stop()
		t.maxQualityTimer = nil
	}
}

func (t *MediaTrackSubscriptions) numSubscribedLayers() uint32 {
	t.maxQualityLock.RLock()
	numSubscribedLayers := uint32(0)
	if t.maxSubscribedQuality != types.VideoQuality_OFF {
		numSubscribedLayers = uint32(SpatialLayerForQuality(t.maxSubscribedQuality) + 1)
	}
	t.maxQualityLock.RUnlock()

	return numSubscribedLayers
}

func (t *MediaTrackSubscriptions) maybeNotifyNoSubscribers() {
	if t.onNoSubscribers == nil {
		return
	}

	t.subscribedTracksMu.RLock()
	empty := len(t.subscribedTracks) == 0
	t.subscribedTracksMu.RUnlock()

	if empty {
		t.onNoSubscribers()
	}
}
