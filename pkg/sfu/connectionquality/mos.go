package connectionquality

import "github.com/pion/ion-cluster/pkg/types"

// MOS score calculation is based on webrtc-stats
// available @ https://github.com/oanguenot/webrtc-stats

const (
	rtt = 70
)

func Score2Rating(score float64) types.ConnectionQuality {
	if score > 3.9 {
		return types.ConnectionQuality_EXCELLENT
	}

	if score > 2.5 {
		return types.ConnectionQuality_GOOD
	}
	return types.ConnectionQuality_POOR
}

func mosAudioEmodel(pctLoss float64, jitter uint32) float64 {
	rx := 93.2 - pctLoss
	ry := 0.18*rx*rx - 27.9*rx + 1126.62

	// Jitter is in MicroSecs (1/1e6) units. Convert it to MilliSecs
	d := float64(rtt + (jitter / 1000))
	h := d - 177.3
	if h < 0 {
		h = 0
	} else {
		h = 1
	}
	id := 0.024*d + 0.11*(d-177.3)*h
	r := ry - (id)
	if r < 0 {
		return 1
	}
	if r > 100 {
		return 4.5
	}
	score := 1 + (0.035 * r) + (7.0/1000000)*r*(r-60)*(100-r)

	return score
}

func loss2Score(pctLoss float64, reducedQuality bool) float64 {
	// No Loss, excellent
	if pctLoss == 0.0 && !reducedQuality {
		return 5
	}
	// default when loss is minimal, but reducedQuality
	score := 3.5
	// loss is bad
	if pctLoss >= 4.0 {
		score = 2.0
	} else if pctLoss <= 2.0 && !reducedQuality {
		// loss is acceptable and at reduced quality
		score = 4.5
	}
	return score
}

func AudioConnectionScore(pctLoss float64, jitter uint32) float64 {
	return mosAudioEmodel(pctLoss, jitter)
}

func VideoConnectionScore(pctLoss float64, reducedQuality bool) float64 {
	return loss2Score(pctLoss, reducedQuality)
}
