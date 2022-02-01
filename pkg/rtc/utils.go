package rtc

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pion/ion-cluster/pkg/logger"
	"github.com/pion/ion-cluster/pkg/types"
	"github.com/pion/webrtc/v3"
)

const (
	trackIdSeparator = "|"
)

func UnpackStreamID(packed string) (participantID types.ParticipantID, trackID types.TrackID) {
	parts := strings.Split(packed, trackIdSeparator)
	if len(parts) > 1 {
		return types.ParticipantID(parts[0]), types.TrackID(packed[len(parts[0])+1:])
	}
	return types.ParticipantID(packed), ""
}

func PackStreamID(participantID types.ParticipantID, trackID types.TrackID) string {
	return string(participantID) + trackIdSeparator + string(trackID)
}

func PackDataTrackLabel(participantID types.ParticipantID, trackID types.TrackID, label string) string {
	return string(participantID) + trackIdSeparator + string(trackID) + trackIdSeparator + label
}

func UnpackDataTrackLabel(packed string) (peerID types.ParticipantID, trackID types.TrackID, label string) {
	parts := strings.Split(packed, trackIdSeparator)
	if len(parts) != 3 {
		return "", types.TrackID(packed), ""
	}
	peerID = types.ParticipantID(parts[0])
	trackID = types.TrackID(parts[1])
	label = parts[2]
	return
}

func ToProtoParticipants(participants []Peer) []*types.ParticipantInfo {
	infos := make([]*types.ParticipantInfo, 0, len(participants))
	for _, op := range participants {
		infos = append(infos, op.ToProto())
	}
	return infos
}

func ToProtoSessionDescription(sd webrtc.SessionDescription) *types.SessionDescription {
	return &types.SessionDescription{
		Type: sd.Type.String(),
		Sdp:  sd.SDP,
	}
}

func FromProtoSessionDescription(sd *types.SessionDescription) webrtc.SessionDescription {
	var sdType webrtc.SDPType
	switch sd.Type {
	case webrtc.SDPTypeOffer.String():
		sdType = webrtc.SDPTypeOffer
	case webrtc.SDPTypeAnswer.String():
		sdType = webrtc.SDPTypeAnswer
	case webrtc.SDPTypePranswer.String():
		sdType = webrtc.SDPTypePranswer
	case webrtc.SDPTypeRollback.String():
		sdType = webrtc.SDPTypeRollback
	}
	return webrtc.SessionDescription{
		Type: sdType,
		SDP:  sd.Sdp,
	}
}

func ToProtoTrickle(candidateInit webrtc.ICECandidateInit) *types.TrickleRequest {
	data, _ := json.Marshal(candidateInit)
	return &types.TrickleRequest{
		CandidateInit: string(data),
	}
}

func FromProtoTrickle(trickle *types.TrickleRequest) (webrtc.ICECandidateInit, error) {
	ci := webrtc.ICECandidateInit{}
	err := json.Unmarshal([]byte(trickle.CandidateInit), &ci)
	if err != nil {
		return webrtc.ICECandidateInit{}, err
	}
	return ci, nil
}

func ToProtoTrackKind(kind webrtc.RTPCodecType) types.TrackType {
	switch kind {
	case webrtc.RTPCodecTypeVideo:
		return types.TrackType_VIDEO
	case webrtc.RTPCodecTypeAudio:
		return types.TrackType_AUDIO
	}
	panic("unsupported track direction")
}

func IsEOF(err error) bool {
	return err == io.ErrClosedPipe || err == io.EOF
}

func RecoverSilent() {
	recover()
}

func Recover() {
	if r := recover(); r != nil {
		var err error
		switch e := r.(type) {
		case string:
			err = errors.New(e)
		case error:
			err = e
		default:
			err = errors.New("unknown panic")
		}
		logger.GetLogger().Error(err, "recovered panic", "panic", r)
	}
}

// logger helpers
func LoggerWithParticipant(l logger.Logger, identity types.ParticipantIdentity, sid types.ParticipantID) logger.Logger {
	lr := logr.Logger(l)
	if identity != "" {
		lr = lr.WithValues("participant", identity)
	}
	if sid != "" {
		lr = lr.WithValues("pID", sid)
	}
	return logger.Logger(lr)
}

func LoggerWithRoom(l logger.Logger, name types.RoomName, roomID types.RoomID) logger.Logger {
	lr := logr.Logger(l)
	if name != "" {
		lr = lr.WithValues("room", name)
	}
	if roomID != "" {
		lr = lr.WithValues("roomID", roomID)
	}
	return logger.Logger(lr)
}

func LoggerWithTrack(l logger.Logger, trackID types.TrackID) logger.Logger {
	lr := logr.Logger(l)
	if trackID != "" {
		lr = lr.WithValues("trackID", trackID)
	}
	return logger.Logger(lr)
}
