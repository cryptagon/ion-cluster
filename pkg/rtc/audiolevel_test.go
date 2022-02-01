package rtc_test

import (
	"testing"

	"github.com/pion/ion-cluster/pkg/rtc"
	"github.com/stretchr/testify/require"
)

const (
	samplesPerBatch    = 25
	defaultActiveLevel = 30
	// requires two noisy samples to count
	defaultPercentile = 10
)

func TestAudioLevel(t *testing.T) {
	t.Run("initially to return not noisy, within a few samples", func(t *testing.T) {
		a := rtc.NewAudioLevel(defaultActiveLevel, defaultPercentile)
		_, noisy := a.GetLevel()
		require.False(t, noisy)

		observeSamples(a, 28, 5)
		_, noisy = a.GetLevel()
		require.False(t, noisy)
	})

	t.Run("not noisy when all samples are below threshold", func(t *testing.T) {
		a := rtc.NewAudioLevel(defaultActiveLevel, defaultPercentile)

		observeSamples(a, 35, 100)
		_, noisy := a.GetLevel()
		require.False(t, noisy)
	})

	t.Run("not noisy when less than percentile samples are above threshold", func(t *testing.T) {
		a := rtc.NewAudioLevel(defaultActiveLevel, defaultPercentile)

		observeSamples(a, 35, samplesPerBatch-2)
		observeSamples(a, 25, 1)
		observeSamples(a, 35, 1)

		_, noisy := a.GetLevel()
		require.False(t, noisy)
	})

	t.Run("noisy when higher than percentile samples are above threshold", func(t *testing.T) {
		a := rtc.NewAudioLevel(defaultActiveLevel, defaultPercentile)

		observeSamples(a, 35, samplesPerBatch-16)
		observeSamples(a, 25, 8)
		observeSamples(a, 29, 8)

		level, noisy := a.GetLevel()
		require.True(t, noisy)
		require.Less(t, level, uint8(defaultActiveLevel))
		require.Greater(t, level, uint8(25))
	})
}

func observeSamples(a *rtc.AudioLevel, level uint8, count int) {
	for i := 0; i < count; i++ {
		a.Observe(level, 20)
	}
}
