package rtc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pion/ion-cluster/pkg/sfu"
	"github.com/pion/webrtc/v3"
)

const (
	highValue         = "high"
	mediumValue       = "medium"
	lowValue          = "low"
	mutedValue        = "none"
	ActiveLayerMethod = "activeLayer"
)

type setRemoteMedia struct {
	StreamID  string   `json:"streamId"`
	Video     string   `json:"video"`
	Framerate string   `json:"framerate"`
	Audio     bool     `json:"audio"`
	Layers    []string `json:"layers"`
}

type activeLayerMessage struct {
	StreamID        string   `json:"streamId"`
	ActiveLayer     string   `json:"activeLayer"`
	AvailableLayers []string `json:"availableLayers"`
}

func layerStrToInt(layer string) (int, error) {
	switch layer {
	case highValue:
		return 2, nil
	case mediumValue:
		return 1, nil
	case lowValue:
		return 0, nil
	default:
		// unknown value
		return -1, fmt.Errorf("Unknown value")
	}
}

func layerIntToStr(layer int) (string, error) {
	switch layer {
	case 0:
		return lowValue, nil
	case 1:
		return mediumValue, nil
	case 2:
		return highValue, nil
	default:
		return "", fmt.Errorf("Unknown value: %d", layer)
	}
}

func transformLayers(layers []string) ([]uint16, error) {
	res := make([]uint16, len(layers))
	for _, layer := range layers {
		if l, err := layerStrToInt(layer); err == nil {
			res = append(res, uint16(l))
		} else {
			return nil, fmt.Errorf("Unknown layer value: %v", layer)
		}
	}
	return res, nil
}

func sendMessage(streamID string, peer Peer, layers []string, activeLayer int) {
	al, _ := layerIntToStr(activeLayer)
	payload := activeLayerMessage{
		StreamID:        streamID,
		ActiveLayer:     al,
		AvailableLayers: layers,
	}
	msg := ChannelAPIMessage{
		Method: ActiveLayerMethod,
		Params: payload,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		sfu.Logger.Error(err, "unable to marshal active layer message")
	}

	if err := peer.SendDCMessage(APIChannelLabel, bytes); err != nil {
		sfu.Logger.Error(err, "unable to send ActiveLayerMessage to peer", "peer_id", peer.ID())
	}
}

func SubscriberAPI(next MessageProcessor) MessageProcessor {
	return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
		srm := &setRemoteMedia{}
		if err := json.Unmarshal(args.Message.Data, srm); err != nil {
			return
		}
		// Publisher changing active layers
		if srm.Layers != nil && len(srm.Layers) > 0 {
			_layers, err := transformLayers(srm.Layers)
			if err != nil {
				sfu.Logger.Error(err, "error reading layers")
				next.Process(ctx, args)
				return
			}

			session := args.Peer.Session()
			peers := session.Peers()
			for _, peer := range peers {
				if peer.ID() != args.Peer.ID() {
					downTracks := peer.Subscriber().GetDownTracks(srm.StreamID)
					for _, dt := range downTracks {
						if dt.Kind() == webrtc.RTPCodecTypeVideo {
							// newLayer, _ := dt.UptrackLayersChange(layers)
							// sendMessage(srm.StreamID, peer, srm.Layers, int(newLayer))
						}
					}
				}
			}
		} else {
			downTracks := args.Peer.Subscriber().GetDownTracks(srm.StreamID)
			for _, dt := range downTracks {
				switch dt.Kind() {
				case webrtc.RTPCodecTypeAudio:
					dt.Mute(!srm.Audio)
				case webrtc.RTPCodecTypeVideo:
					switch srm.Video {
					case highValue:
						dt.Mute(false)
						dt.SetMaxSpatialLayer(2)
					case mediumValue:
						dt.Mute(false)
						dt.SetMaxSpatialLayer(1)
					case lowValue:
						dt.Mute(false)
						dt.SetMaxSpatialLayer(0)
					case mutedValue:
						dt.Mute(true)
					}
					switch srm.Framerate {
					case highValue:
						dt.SetMaxTemporalLayer(2)
					case mediumValue:
						dt.SetMaxTemporalLayer(1)
					case lowValue:
						dt.SetMaxTemporalLayer(0)
					}
				}

			}
		}
		next.Process(ctx, args)
	})
}
