package sfu

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/ion-cluster/pkg/types"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"

	"github.com/pion/ion-cluster/pkg/config"
)

const (
	ChannelCapacityInfinity = 100 * 1000 * 1000 // 100 Mbps

	EstimateEpsilon = 2000 // 2 kbps

	GratuitousProbeHeadroomBps = 1 * 1000 * 1000 // if headroom > 1 Mbps, don't probe
	GratuitousProbePct         = 10
	GratuitousProbeMinBps      = 100 * 1000 // 100 kbps
	GratuitousProbeMaxBps      = 300 * 1000 // 300 kbps
	GratuitousProbeMinDuration = 500 * time.Millisecond
	GratuitousProbeMaxDuration = 600 * time.Millisecond

	AudioLossWeight = 0.75
	VideoLossWeight = 0.25

	// LK-TODO-START
	// These constants will definitely require more tweaking.
	// In fact, simple time threshold rules most probably will not be enough.
	// LK-TODO-END
	EstimateCommit          = 2 * 1000 * time.Millisecond // 2 seconds
	ProbeWait               = 8 * 1000 * time.Millisecond // 8 seconds
	BoostWait               = 5 * 1000 * time.Millisecond // 5 seconds
	GratuitousProbeWait     = 8 * 1000 * time.Millisecond // 8 seconds
	GratuitousProbeMoreWait = 5 * 1000 * time.Millisecond // 5 seconds
)

type State int

const (
	StateStable State = iota
	StateDeficient
)

func (s State) String() string {
	switch s {
	case StateStable:
		return "STABLE"
	case StateDeficient:
		return "DEFICIENT"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

type Signal int

const (
	SignalAddTrack Signal = iota
	SignalRemoveTrack
	SignalEstimate
	SignalTargetBitrate
	SignalReceiverReport
	SignalAvailableLayersChange
	SignalSubscriptionChange
	SignalSubscribedLayersChange
	SignalPeriodicPing
	SignalSendProbe
)

func (s Signal) String() string {
	switch s {
	case SignalAddTrack:
		return "ADD_TRACK"
	case SignalRemoveTrack:
		return "REMOVE_TRACK"
	case SignalEstimate:
		return "ESTIMATE"
	case SignalTargetBitrate:
		return "TARGET_BITRATE"
	case SignalReceiverReport:
		return "RECEIVER_REPORT"
	case SignalSubscriptionChange:
		return "SUBSCRIPTION_CHANGE"
	case SignalSubscribedLayersChange:
		return "SUBSCRIBED_LAYERS_CHANGE"
	case SignalPeriodicPing:
		return "PERIODIC_PING"
	case SignalSendProbe:
		return "SEND_PROBE"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

type Event struct {
	Signal    Signal
	DownTrack *DownTrack
	Data      interface{}
}

func (e Event) String() string {
	return fmt.Sprintf("StreamAllocator:Event{signal: %s, data: %s}", e.Signal, e.Data)
}

type StreamAllocatorParams struct {
	Config config.CongestionControlConfig
	Logger logger.Logger
}

type StreamAllocator struct {
	params StreamAllocatorParams

	onStreamStateChange func(update *StreamStateUpdate) error

	rembTrackingSSRC uint32

	bwe cc.BandwidthEstimator

	committedChannelCapacity int64
	lastCommitTime           time.Time
	prevReceivedEstimate     int64
	receivedEstimate         int64
	lastEstimateDecreaseTime time.Time

	lastBoostTime time.Time

	lastGratuitousProbeTime time.Time

	audioTracks              map[types.TrackID]*Track
	videoTracks              map[types.TrackID]*Track
	exemptVideoTracksSorted  TrackSorter
	managedVideoTracksSorted TrackSorter

	prober *Prober

	state State

	chMu      sync.RWMutex
	eventCh   chan Event
	runningCh chan struct{}
}

func NewStreamAllocator(params StreamAllocatorParams) *StreamAllocator {
	s := &StreamAllocator{
		params:      params,
		audioTracks: make(map[types.TrackID]*Track),
		videoTracks: make(map[types.TrackID]*Track),
		prober: NewProber(ProberParams{
			Logger: params.Logger,
		}),
		eventCh:   make(chan Event, 20),
		runningCh: make(chan struct{}),
	}

	s.initializeEstimate()

	s.prober.OnSendProbe(s.onSendProbe)

	return s
}

func (s *StreamAllocator) Start() {
	go s.processEvents()
	go s.ping()
}

func (s *StreamAllocator) Stop() {
	s.chMu.Lock()
	defer s.chMu.Unlock()

	close(s.runningCh)
	close(s.eventCh)
}

func (s *StreamAllocator) OnStreamStateChange(f func(update *StreamStateUpdate) error) {
	s.onStreamStateChange = f
}

func (s *StreamAllocator) SetBandwidthEstimator(bwe cc.BandwidthEstimator) {
	if bwe != nil {
		bwe.OnTargetBitrateChange(s.onTargetBitrateChange)
	}
	s.bwe = bwe
}

type AddTrackParams struct {
	Source      types.TrackSource
	IsSimulcast bool
}

func (s *StreamAllocator) AddTrack(downTrack *DownTrack, params AddTrackParams) {
	s.postEvent(Event{
		Signal:    SignalAddTrack,
		DownTrack: downTrack,
		Data:      params,
	})

	if downTrack.Kind() == webrtc.RTPCodecTypeVideo {
		downTrack.OnREMB(s.onREMB)
		downTrack.OnTransportCCFeedback(s.onTransportCCFeedback)
		downTrack.OnAvailableLayersChanged(s.onAvailableLayersChanged)
		downTrack.OnSubscriptionChanged(s.onSubscriptionChanged)
		downTrack.OnSubscribedLayersChanged(s.onSubscribedLayersChanged)
		downTrack.OnPacketSent(s.onPacketSent)
	}
	downTrack.AddReceiverReportListener(s.onReceiverReport)
}

func (s *StreamAllocator) RemoveTrack(downTrack *DownTrack) {
	s.postEvent(Event{
		Signal:    SignalRemoveTrack,
		DownTrack: downTrack,
	})
}

func (s *StreamAllocator) initializeEstimate() {
	s.committedChannelCapacity = ChannelCapacityInfinity
	s.lastCommitTime = time.Now().Add(-EstimateCommit)
	s.receivedEstimate = ChannelCapacityInfinity
	s.lastEstimateDecreaseTime = time.Now()

	s.state = StateStable
}

// called when a new REMB is received
func (s *StreamAllocator) onREMB(downTrack *DownTrack, remb *rtcp.ReceiverEstimatedMaximumBitrate) {
	s.postEvent(Event{
		Signal:    SignalEstimate,
		DownTrack: downTrack,
		Data:      remb,
	})
}

// called when a new transport-cc feedback is received
func (s *StreamAllocator) onTransportCCFeedback(downTrack *DownTrack, fb *rtcp.TransportLayerCC) {
	if s.bwe != nil {
		s.bwe.WriteRTCP([]rtcp.Packet{fb}, nil)
	}
}

// called when target bitrate changes
func (s *StreamAllocator) onTargetBitrateChange(bitrate int) {
	s.postEvent(Event{
		Signal: SignalTargetBitrate,
		Data:   bitrate,
	})
}

// called when a new RTCP Receiver Report is received
func (s *StreamAllocator) onReceiverReport(downTrack *DownTrack, rr *rtcp.ReceiverReport) {
	s.postEvent(Event{
		Signal:    SignalReceiverReport,
		DownTrack: downTrack,
		Data:      rr,
	})
}

// called when feeding track's layer availability changes
func (s *StreamAllocator) onAvailableLayersChanged(downTrack *DownTrack) {
	s.postEvent(Event{
		Signal:    SignalAvailableLayersChange,
		DownTrack: downTrack,
	})
}

// called when subscription settings changes (muting/unmuting of track)
func (s *StreamAllocator) onSubscriptionChanged(downTrack *DownTrack) {
	s.postEvent(Event{
		Signal:    SignalSubscriptionChange,
		DownTrack: downTrack,
	})
}

// called when subscribed layers changes (limiting max layers)
func (s *StreamAllocator) onSubscribedLayersChanged(downTrack *DownTrack, layers VideoLayers) {
	s.postEvent(Event{
		Signal:    SignalSubscribedLayersChange,
		DownTrack: downTrack,
		Data:      layers,
	})
}

// called when a video DownTrack sends a packet
func (s *StreamAllocator) onPacketSent(downTrack *DownTrack, size int) {
	s.prober.PacketSent(size)
}

// called when prober wants to send packet(s)
func (s *StreamAllocator) onSendProbe(bytesToSend int) {
	s.postEvent(Event{
		Signal: SignalSendProbe,
		Data:   bytesToSend,
	})
}

func (s *StreamAllocator) postEvent(event Event) {
	s.chMu.RLock()
	defer s.chMu.RUnlock()

	if !s.isRunning() {
		return
	}

	s.eventCh <- event
}

func (s *StreamAllocator) processEvents() {
	for event := range s.eventCh {
		s.handleEvent(&event)
	}
}

func (s *StreamAllocator) isRunning() bool {
	select {
	case <-s.runningCh:
		return false
	default:
		return true
	}
}

func (s *StreamAllocator) ping() {
	ticker := time.NewTicker(time.Second)

	for s.isRunning() {
		<-ticker.C
		if !s.isRunning() {
			return
		}

		s.postEvent(Event{
			Signal: SignalPeriodicPing,
		})
	}
}

func (s *StreamAllocator) handleEvent(event *Event) {
	switch event.Signal {
	case SignalAddTrack:
		s.handleSignalAddTrack(event)
	case SignalRemoveTrack:
		s.handleSignalRemoveTrack(event)
	case SignalEstimate:
		s.handleSignalEstimate(event)
	case SignalTargetBitrate:
		s.handleSignalTargetBitrate(event)
	case SignalReceiverReport:
		s.handleSignalReceiverReport(event)
	case SignalAvailableLayersChange:
		s.handleSignalAvailableLayersChange(event)
	case SignalSubscriptionChange:
		s.handleSignalSubscriptionChange(event)
	case SignalSubscribedLayersChange:
		s.handleSignalSubscribedLayersChange(event)
	case SignalPeriodicPing:
		s.handleSignalPeriodicPing(event)
	case SignalSendProbe:
		s.handleSignalSendProbe(event)
	}
}

func (s *StreamAllocator) handleSignalAddTrack(event *Event) {
	params, _ := event.Data.(AddTrackParams)
	isManaged := (params.Source != types.TrackSource_SCREEN_SHARE && params.Source != types.TrackSource_SCREEN_SHARE_AUDIO) || params.IsSimulcast
	track := newTrack(event.DownTrack, isManaged)

	trackID := types.TrackID(event.DownTrack.ID())
	switch event.DownTrack.Kind() {
	case webrtc.RTPCodecTypeAudio:
		s.audioTracks[trackID] = track
	case webrtc.RTPCodecTypeVideo:
		s.videoTracks[trackID] = track

		if isManaged {
			s.managedVideoTracksSorted = append(s.managedVideoTracksSorted, track)
			sort.Sort(s.managedVideoTracksSorted)
		} else {
			s.exemptVideoTracksSorted = append(s.exemptVideoTracksSorted, track)
			sort.Sort(s.exemptVideoTracksSorted)
		}

		s.allocateTrack(track)
	}
}

func (s *StreamAllocator) handleSignalRemoveTrack(event *Event) {
	trackID := types.TrackID(event.DownTrack.ID())
	switch event.DownTrack.Kind() {
	case webrtc.RTPCodecTypeAudio:
		if _, ok := s.audioTracks[trackID]; !ok {
			return
		}

		delete(s.audioTracks, trackID)
	case webrtc.RTPCodecTypeVideo:
		track, ok := s.videoTracks[trackID]
		if !ok {
			return
		}

		delete(s.videoTracks, trackID)

		if track.IsManaged() {
			n := len(s.managedVideoTracksSorted)
			for idx, videoTrack := range s.managedVideoTracksSorted {
				if videoTrack.DownTrack() == event.DownTrack {
					s.managedVideoTracksSorted[idx] = s.managedVideoTracksSorted[n-1]
					s.managedVideoTracksSorted = s.managedVideoTracksSorted[:n-1]
					break
				}
			}
			sort.Sort(s.managedVideoTracksSorted)
		} else {
			n := len(s.exemptVideoTracksSorted)
			for idx, videoTrack := range s.exemptVideoTracksSorted {
				if videoTrack.DownTrack() == event.DownTrack {
					s.exemptVideoTracksSorted[idx] = s.exemptVideoTracksSorted[n-1]
					s.exemptVideoTracksSorted = s.exemptVideoTracksSorted[:n-1]
					break
				}
			}
			sort.Sort(s.exemptVideoTracksSorted)
		}

		// re-initialize estimate if all managed tracks are removed, let it get a fresh start
		if len(s.managedVideoTracksSorted) == 0 {
			s.initializeEstimate()
			return
		}

		// LK-TODO: use any saved bandwidth to re-distribute
		s.adjustState()
	}
}

func (s *StreamAllocator) handleSignalEstimate(event *Event) {
	// the channel capacity is estimated at a peer connection level. All down tracks
	// in the peer connection will end up calling this for a REMB report with
	// the same estimated channel capacity. Use a tracking SSRC to lock onto to
	// one report. As SSRCs can be dropped over time, update tracking SSRC as needed
	//
	// A couple of things to keep in mind
	//   - REMB reports could be sent gratuitously as a way of providing
	//     periodic feedback, i.e. even if the estimated capacity does not
	//     change, there could be REMB packets on the wire. Those gratuitous
	//     REMBs should not trigger anything bad.
	//   - As each down track will issue this callback for the same REMB packet
	//     from the wire, theoretically it is possible that one down track's
	//     callback from previous REMB comes after another down track's callback
	//     from the new REMB. REMBs could fire very quickly especially when
	//     the network is entering congestion.
	// LK-TODO-START
	// Need to check if the same SSRC reports can somehow race, i.e. does pion send
	// RTCP dispatch for same SSRC on different threads? If not, the tracking SSRC
	// should prevent racing
	// LK-TODO-END

	// if there are no video tracks, ignore any straggler REMB
	if len(s.managedVideoTracksSorted) == 0 {
		return
	}

	remb, _ := event.Data.(*rtcp.ReceiverEstimatedMaximumBitrate)

	found := false
	for _, ssrc := range remb.SSRCs {
		if ssrc == s.rembTrackingSSRC {
			found = true
			break
		}
	}
	if !found {
		if len(remb.SSRCs) == 0 {
			s.params.Logger.Warnw("no SSRC to track REMB", nil)
			return
		}

		// try to lock to track which is sending this update
		for _, ssrc := range remb.SSRCs {
			if ssrc == event.DownTrack.SSRC() {
				s.rembTrackingSSRC = event.DownTrack.SSRC()
				found = true
				break
			}
		}

		if !found {
			s.rembTrackingSSRC = remb.SSRCs[0]
		}
	}

	if s.rembTrackingSSRC != event.DownTrack.SSRC() {
		return
	}

	s.prevReceivedEstimate = s.receivedEstimate
	s.receivedEstimate = int64(remb.Bitrate)
	/*
		if s.prevReceivedEstimate != s.receivedEstimate {
			s.params.Logger.Debugw("received new estimate",
				"old(bps)", s.prevReceivedEstimate,
				"new(bps)", s.receivedEstimate,
			)
		}
	*/

	if s.maybeCommitEstimate() {
		s.allocateAllTracks()
	}
}

func (s *StreamAllocator) handleSignalTargetBitrate(event *Event) {
	receivedEstimate, _ := event.Data.(int)
	s.prevReceivedEstimate = s.receivedEstimate
	s.receivedEstimate = int64(receivedEstimate)
	/*
		if s.prevReceivedEstimate != s.receivedEstimate {
			s.params.Logger.Debugw("received new estimate",
				"old(bps)", s.prevReceivedEstimate,
				"new(bps)", s.receivedEstimate,
			)
		}
	*/

	if s.maybeCommitEstimate() {
		s.allocateAllTracks()
	}
}

// LK-TODO-START
// Receiver report stats are not used in the current implementation.
//
// The idea is to use a loss/rtt based estimator and compare against REMB like outlined here
// https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02#section-6
//
// But the implementation could get quite tricky. So, a separate PR dedicated effort for that
// is required. Something like from Chrome, but hopefully much less complicated :-)
// https://source.chromium.org/chromium/chromium/src/+/main:third_party/webrtc/modules/congestion_controller/goog_cc/loss_based_bandwidth_estimation.cc;bpv=0;bpt=1
// LK-TODO-END
func (s *StreamAllocator) handleSignalReceiverReport(event *Event) {
	var track *Track
	ok := false

	trackID := types.TrackID(event.DownTrack.ID())
	switch event.DownTrack.Kind() {
	case webrtc.RTPCodecTypeAudio:
		track, ok = s.audioTracks[trackID]
	case webrtc.RTPCodecTypeVideo:
		track, ok = s.videoTracks[trackID]
	}
	if !ok {
		return
	}

	rr, _ := event.Data.(*rtcp.ReceiverReport)
	track.UpdatePacketStats(rr)
}

func (s *StreamAllocator) handleSignalAvailableLayersChange(event *Event) {
	track, ok := s.videoTracks[types.TrackID(event.DownTrack.ID())]
	if !ok {
		return
	}

	s.allocateTrack(track)
}

func (s *StreamAllocator) handleSignalSubscriptionChange(event *Event) {
	track, ok := s.videoTracks[types.TrackID(event.DownTrack.ID())]
	if !ok {
		return
	}

	s.allocateTrack(track)
}

func (s *StreamAllocator) handleSignalSubscribedLayersChange(event *Event) {
	track, ok := s.videoTracks[types.TrackID(event.DownTrack.ID())]
	if !ok {
		return
	}

	layers := event.Data.(VideoLayers)
	track.UpdateMaxLayers(layers)
	if track.IsManaged() {
		sort.Sort(s.managedVideoTracksSorted)
	} else {
		sort.Sort(s.exemptVideoTracksSorted)
	}

	s.allocateTrack(track)
}

func (s *StreamAllocator) handleSignalPeriodicPing(event *Event) {
	if s.maybeCommitEstimate() {
		s.allocateAllTracks()
	}

	// catch up on all optimistically streamed tracks
	s.finalizeTracks()

	if s.state == StateDeficient {
		s.maybeProbe()
	}
}

func (s *StreamAllocator) handleSignalSendProbe(event *Event) {
	bytesToSend := event.Data.(int)
	if bytesToSend <= 0 {
		return
	}

	bytesSent := 0
	for _, track := range s.videoTracks {
		sent := track.WritePaddingRTP(bytesToSend)
		bytesSent += sent
		bytesToSend -= sent
		if bytesToSend <= 0 {
			break
		}
	}

	if bytesSent != 0 {
		s.prober.ProbeSent(bytesSent)
	}
}

func (s *StreamAllocator) setState(state State) {
	if s.state != state {
		s.params.Logger.Infow("state change", "from", s.state, "to", state)
	}

	s.state = state
}

func (s *StreamAllocator) adjustState() {
	for _, videoTrack := range s.managedVideoTracksSorted {
		if videoTrack.IsDeficient() {
			s.setState(StateDeficient)
			return
		}
	}

	s.setState(StateStable)
}

func (s *StreamAllocator) maybeCommitEstimate() (isDecreasing bool) {
	// commit channel capacity estimate under following rules
	//   1. Abs(receivedEstimate - prevReceivedEstimate) < EstimateEpsilon => estimate stable
	//   2. time.Since(lastCommitTime) > EstimateCommitMs => to catch long oscillating estimate
	if math.Abs(float64(s.receivedEstimate)-float64(s.prevReceivedEstimate)) > EstimateEpsilon {
		// too large a change, wait for estimate to settle.
		// Unless estimate has been oscillating for too long.
		if time.Since(s.lastCommitTime) < EstimateCommit {
			return
		}
	}

	// don't commit too often even if the change is small.
	// Small changes will also get picked up during periodic check.
	if time.Since(s.lastCommitTime) < EstimateCommit {
		return
	}

	if s.receivedEstimate == s.committedChannelCapacity {
		// no change in estimate, no need to commit
		return
	}

	if s.committedChannelCapacity > s.receivedEstimate && s.committedChannelCapacity != ChannelCapacityInfinity {
		// this prevents declaring a decrease when coming out of init state.
		// But, this bypasses the case where streaming starts on a bunch of
		// tracks simultaneously (imagine a participant joining a large room
		// with a lot of video tracks). In that case, it is possible that the
		// channel is hitting congestion. It will be caught on the next estimate
		// decrease.
		s.lastEstimateDecreaseTime = time.Now()
		isDecreasing = true
	}

	if s.committedChannelCapacity > s.receivedEstimate {
		s.params.Logger.Debugw("committing channel capacity(bps)", "from", s.committedChannelCapacity, "to", s.receivedEstimate)
	}
	s.committedChannelCapacity = s.receivedEstimate
	s.lastCommitTime = time.Now()

	return
}

func (s *StreamAllocator) allocateTrack(track *Track) {
	// if not deficient, free pass allocate track
	if !s.params.Config.Enabled || s.state == StateStable || !track.IsManaged() {
		update := NewStreamStateUpdate()
		allocation := track.Allocate(ChannelCapacityInfinity, s.params.Config.AllowPause)
		update.HandleStreamingChange(allocation.change, track)
		s.maybeSendUpdate(update)
		return
	}

	//
	// In DEFICIENT state,
	//   1. Find cooperative transition from track that needs allocation.
	//   2. If track is currently streaming at minimum, do not do anything.
	//   3. If that track is giving back bits, apply the transition.
	//   4. If this track needs more, ask for best offer from others and try to use it.
	//
	track.ProvisionalAllocatePrepare()
	transition := track.ProvisionalAllocateGetCooperativeTransition()

	// track is currently streaming at minimum
	if transition.bandwidthDelta == 0 {
		return
	}

	// downgrade, giving back bits
	if transition.from.GreaterThan(transition.to) {
		allocation := track.ProvisionalAllocateCommit()

		update := NewStreamStateUpdate()
		update.HandleStreamingChange(allocation.change, track)
		s.maybeSendUpdate(update)

		s.adjustState()
		return
		// LK-TODO-START
		// Should use the bits given back to start any paused track.
		// Note layer downgrade may actually have positive delta (i.e. consume more bits)
		// because of when the measurement is done. Watch for that.
		// LK-TODO-END
	}

	//
	// This track is currently not streaming and needs bits to start.
	// Try to redistribute starting with tracks that are closest to their desired.
	//
	var minDistanceSorted MinDistanceSorter
	for _, t := range s.managedVideoTracksSorted {
		if t != track {
			minDistanceSorted = append(minDistanceSorted, t)
		}
	}
	sort.Sort(minDistanceSorted)

	bandwidthAcquired := int64(0)
	var contributingTracks []*Track

	for _, t := range minDistanceSorted {
		t.ProvisionalAllocatePrepare()
	}

	for _, t := range minDistanceSorted {
		tx := t.ProvisionalAllocateGetBestWeightedTransition()
		if tx.bandwidthDelta < 0 {
			contributingTracks = append(contributingTracks, t)

			bandwidthAcquired += -tx.bandwidthDelta
			if bandwidthAcquired >= transition.bandwidthDelta {
				break
			}
		}
	}

	update := NewStreamStateUpdate()
	if bandwidthAcquired >= transition.bandwidthDelta {
		// commit the tracks that contributed
		for _, t := range contributingTracks {
			allocation := t.ProvisionalAllocateCommit()
			update.HandleStreamingChange(allocation.change, t)
		}

		// LK-TODO if got too much extra, can potentially give it to some deficient track
	}

	// commit the track that needs change if enough could be acquired or pause not allowed
	if !s.params.Config.AllowPause || bandwidthAcquired >= transition.bandwidthDelta {
		allocation := track.ProvisionalAllocateCommit()
		update.HandleStreamingChange(allocation.change, track)
	}

	s.maybeSendUpdate(update)

	s.adjustState()
}

func (s *StreamAllocator) allocateAllTracks() {
	if !s.params.Config.Enabled {
		// nothing else to do when disabled
		return
	}

	s.resetBoost()

	//
	// Goals:
	//   1. Stream as many tracks as possible, i.e. no pauses.
	//   2. Try to give fair allocation to all track.
	//
	// Start with the lowest layers and give each track a chance at that layer and keep going up.
	// As long as there is enough bandwidth for tracks to stream at the lowest layers, the first goal is achieved.
	//
	// Tracks that have higher subscribed layers can use any additional available bandwidth. This tried to achieve the second goal.
	//
	// If there is not enough bandwidth even for the lowest layers, tracks at lower priorities will be paused.
	//
	update := NewStreamStateUpdate()

	availableChannelCapacity := s.committedChannelCapacity

	//
	// This pass is just to find out if there is any leftover channel capacity.
	// Infinite channel capacity is given so that exempt tracks do not stall
	//
	for _, track := range s.exemptVideoTracksSorted {
		allocation := track.Allocate(ChannelCapacityInfinity, s.params.Config.AllowPause)
		update.HandleStreamingChange(allocation.change, track)

		// LK-TODO: optimistic allocation before bitrate is available will return 0. How to account for that?
		availableChannelCapacity -= allocation.bandwidthRequested
	}

	if availableChannelCapacity < 0 {
		availableChannelCapacity = 0
	}
	if availableChannelCapacity == 0 && s.params.Config.AllowPause {
		// nothing left for managed tracks, pause them all
		for _, track := range s.managedVideoTracksSorted {
			allocation := track.Pause()
			update.HandleStreamingChange(allocation.change, track)
		}
	} else {
		for _, track := range s.managedVideoTracksSorted {
			track.ProvisionalAllocatePrepare()
		}

		for spatial := int32(0); spatial <= DefaultMaxLayerSpatial; spatial++ {
			for temporal := int32(0); temporal <= DefaultMaxLayerTemporal; temporal++ {
				layers := VideoLayers{
					spatial:  spatial,
					temporal: temporal,
				}

				for _, track := range s.managedVideoTracksSorted {
					usedChannelCapacity := track.ProvisionalAllocate(availableChannelCapacity, layers, s.params.Config.AllowPause)
					availableChannelCapacity -= usedChannelCapacity
					if availableChannelCapacity < 0 {
						availableChannelCapacity = 0
					}
				}
			}
		}

		for _, track := range s.managedVideoTracksSorted {
			allocation := track.ProvisionalAllocateCommit()
			update.HandleStreamingChange(allocation.change, track)
		}
	}

	s.maybeSendUpdate(update)

	s.adjustState()
}

func (s *StreamAllocator) maybeSendUpdate(update *StreamStateUpdate) {
	if update.Empty() {
		return
	}

	s.params.Logger.Debugw("streamed tracks changed", "update", update)
	if s.onStreamStateChange != nil {
		err := s.onStreamStateChange(update)
		if err != nil {
			s.params.Logger.Errorw("could not send streamed tracks update", err)
		}
	}
}

func (s *StreamAllocator) finalizeTracks() {
	for _, t := range s.exemptVideoTracksSorted {
		t.FinalizeAllocate()
	}

	for _, t := range s.managedVideoTracksSorted {
		t.FinalizeAllocate()
	}

	s.adjustState()
}

func (s *StreamAllocator) getExpectedBandwidthUsage() int64 {
	expected := int64(0)
	for _, track := range s.videoTracks {
		expected += track.BandwidthRequested()
	}

	return expected
}

// LK-TODO: unused till loss based estimation is done, but just a sample impl of weighting audio higher
func (s *StreamAllocator) calculateLoss() float32 {
	packetsAudio := uint32(0)
	packetsLostAudio := uint32(0)
	for _, track := range s.audioTracks {
		packets, packetsLost := track.GetPacketStats()

		packetsAudio += packets
		packetsLostAudio += packetsLost
	}

	audioLossPct := float32(0.0)
	if packetsAudio != 0 {
		audioLossPct = (float32(packetsLostAudio) * 100.0) / float32(packetsAudio)
	}

	packetsVideo := uint32(0)
	packetsLostVideo := uint32(0)
	for _, track := range s.videoTracks {
		packets, packetsLost := track.GetPacketStats()

		packetsVideo += packets
		packetsLostVideo += packetsLost
	}

	videoLossPct := float32(0.0)
	if packetsVideo != 0 {
		videoLossPct = (float32(packetsLostVideo) * 100.0) / float32(packetsVideo)
	}

	return AudioLossWeight*audioLossPct + VideoLossWeight*videoLossPct
}

func (s *StreamAllocator) maybeProbe() {
	if !s.isTimeToBoost() {
		return
	}

	s.maybeBoostLayer()
	s.adjustState()
}

func (s *StreamAllocator) maybeBoostLayer() {
	var maxDistanceSorted MaxDistanceSorter
	for _, track := range s.managedVideoTracksSorted {
		maxDistanceSorted = append(maxDistanceSorted, track)
	}
	sort.Sort(maxDistanceSorted)

	// boost first deficient track in priority order
	for _, track := range maxDistanceSorted {
		if !track.IsDeficient() {
			continue
		}

		allocation, boosted := track.AllocateNextHigher()
		if boosted {
			s.lastBoostTime = time.Now()

			update := NewStreamStateUpdate()
			update.HandleStreamingChange(allocation.change, track)
			s.maybeSendUpdate(update)

			break
		}
	}
}

func (s *StreamAllocator) isTimeToBoost() bool {
	// if enough time has passed since last estimate drop or last estimate boost,
	// artificially boost estimate before allocating.
	// Checking against last estimate boost prevents multiple artificial boosts
	// in situations where multiple tracks become available in a short span.
	if !s.lastBoostTime.IsZero() {
		return time.Since(s.lastBoostTime) > BoostWait
	} else {
		return time.Since(s.lastEstimateDecreaseTime) > ProbeWait
	}
}

func (s *StreamAllocator) resetBoost() {
	s.lastBoostTime = time.Time{}
}

func (s *StreamAllocator) maybeGratuitousProbe() bool {
	if time.Since(s.lastEstimateDecreaseTime) < GratuitousProbeWait || len(s.managedVideoTracksSorted) == 0 {
		return false
	}

	// don't gratuitously probe too often
	if time.Since(s.lastGratuitousProbeTime) < GratuitousProbeMoreWait {
		return false
	}

	// use last received estimate for gratuitous probing base as
	// more updates may have been received since the last commit
	expectedRateBps := s.getExpectedBandwidthUsage()
	headroomBps := s.receivedEstimate - expectedRateBps
	if headroomBps > GratuitousProbeHeadroomBps {
		return false
	}

	probeRateBps := (s.receivedEstimate * GratuitousProbePct) / 100
	if probeRateBps < GratuitousProbeMinBps {
		probeRateBps = GratuitousProbeMinBps
	}
	if probeRateBps > GratuitousProbeMaxBps {
		probeRateBps = GratuitousProbeMaxBps
	}

	s.prober.AddCluster(
		int(s.receivedEstimate+probeRateBps),
		int(expectedRateBps),
		GratuitousProbeMinDuration,
		GratuitousProbeMaxDuration,
	)

	s.lastGratuitousProbeTime = time.Now()
	return true
}

func (s *StreamAllocator) resetGratuitousProbe() {
	s.prober.Reset()
	s.lastGratuitousProbeTime = time.Now()
}

// ------------------------------------------------

type StreamState int

const (
	StreamStateActive StreamState = iota
	StreamStatePaused
)

type StreamStateInfo struct {
	ParticipantID types.ParticipantID
	TrackID       types.TrackID
	State         StreamState
}

type StreamStateUpdate struct {
	StreamStates []*StreamStateInfo
}

func NewStreamStateUpdate() *StreamStateUpdate {
	return &StreamStateUpdate{}
}

func (s *StreamStateUpdate) HandleStreamingChange(change VideoStreamingChange, track *Track) {
	switch change {
	case VideoStreamingChangePausing:
		s.StreamStates = append(s.StreamStates, &StreamStateInfo{
			ParticipantID: track.PeerID(),
			TrackID:       track.ID(),
			State:         StreamStatePaused,
		})
	case VideoStreamingChangeResuming:
		s.StreamStates = append(s.StreamStates, &StreamStateInfo{
			ParticipantID: track.PeerID(),
			TrackID:       track.ID(),
			State:         StreamStateActive,
		})
	}
}

func (s *StreamStateUpdate) Empty() bool {
	return len(s.StreamStates) == 0
}

// ------------------------------------------------

type Track struct {
	downTrack *DownTrack
	isManaged bool

	highestSN       uint32
	packetsLost     uint32
	lastHighestSN   uint32
	lastPacketsLost uint32

	maxLayers VideoLayers
}

func newTrack(downTrack *DownTrack, isManaged bool) *Track {
	t := &Track{
		downTrack: downTrack,
		isManaged: isManaged,
	}
	t.UpdateMaxLayers(downTrack.MaxLayers())

	return t
}

func (t *Track) DownTrack() *DownTrack {
	return t.downTrack
}

func (t *Track) IsManaged() bool {
	return t.isManaged
}

func (t *Track) ID() types.TrackID {
	return types.TrackID(t.downTrack.ID())
}

func (t *Track) PeerID() types.ParticipantID {
	return t.downTrack.PeerID()
}

// LK-TODO this should probably be maintained in downTrack and this module can query what it needs
func (t *Track) UpdatePacketStats(rr *rtcp.ReceiverReport) {
	t.lastHighestSN = t.highestSN
	t.lastPacketsLost = t.packetsLost

	for _, report := range rr.Reports {
		if report.LastSequenceNumber > t.highestSN {
			t.highestSN = report.LastSequenceNumber
		}
		if report.TotalLost > t.packetsLost {
			t.packetsLost = report.TotalLost
		}
	}
}

func (t *Track) UpdateMaxLayers(layers VideoLayers) {
	t.maxLayers = layers
}

func (t *Track) GetPacketStats() (uint32, uint32) {
	return t.highestSN - t.lastHighestSN, t.packetsLost - t.lastPacketsLost
}

func (t *Track) WritePaddingRTP(bytesToSend int) int {
	return t.downTrack.WritePaddingRTP(bytesToSend)
}

func (t *Track) Allocate(availableChannelCapacity int64, allowPause bool) VideoAllocation {
	return t.downTrack.Allocate(availableChannelCapacity, allowPause)
}

func (t *Track) ProvisionalAllocatePrepare() {
	t.downTrack.ProvisionalAllocatePrepare()
}

func (t *Track) ProvisionalAllocate(availableChannelCapacity int64, layers VideoLayers, allowPause bool) int64 {
	return t.downTrack.ProvisionalAllocate(availableChannelCapacity, layers, allowPause)
}

func (t *Track) ProvisionalAllocateGetCooperativeTransition() VideoTransition {
	return t.downTrack.ProvisionalAllocateGetCooperativeTransition()
}

func (t *Track) ProvisionalAllocateGetBestWeightedTransition() VideoTransition {
	return t.downTrack.ProvisionalAllocateGetBestWeightedTransition()
}

func (t *Track) ProvisionalAllocateCommit() VideoAllocation {
	return t.downTrack.ProvisionalAllocateCommit()
}

func (t *Track) AllocateNextHigher() (VideoAllocation, bool) {
	return t.downTrack.AllocateNextHigher()
}

func (t *Track) FinalizeAllocate() {
	t.downTrack.FinalizeAllocate()
}

func (t *Track) Pause() VideoAllocation {
	return t.downTrack.Pause()
}

func (t *Track) IsDeficient() bool {
	return t.downTrack.IsDeficient()
}

func (t *Track) BandwidthRequested() int64 {
	return t.downTrack.BandwidthRequested()
}

func (t *Track) DistanceToDesired() int32 {
	return t.downTrack.DistanceToDesired()
}

// ------------------------------------------------

type TrackSorter []*Track

func (t TrackSorter) Len() int {
	return len(t)
}

func (t TrackSorter) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t TrackSorter) Less(i, j int) bool {
	if t[i].maxLayers.spatial != t[j].maxLayers.spatial {
		return t[i].maxLayers.spatial > t[j].maxLayers.spatial
	}

	return t[i].maxLayers.temporal > t[j].maxLayers.temporal
}

// ------------------------------------------------

type MaxDistanceSorter []*Track

func (m MaxDistanceSorter) Len() int {
	return len(m)
}

func (m MaxDistanceSorter) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m MaxDistanceSorter) Less(i, j int) bool {
	return m[i].DistanceToDesired() > m[j].DistanceToDesired()
}

// ------------------------------------------------

type MinDistanceSorter []*Track

func (m MinDistanceSorter) Len() int {
	return len(m)
}

func (m MinDistanceSorter) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m MinDistanceSorter) Less(i, j int) bool {
	return m[i].DistanceToDesired() < m[j].DistanceToDesired()
}

// ------------------------------------------------
