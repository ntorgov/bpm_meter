package bpm

import (
	"math"
	"testing"
)

func TestEstimateSyntheticPulseTrain(t *testing.T) {
	const sampleRate = 11025
	const want = 120
	duration := 45 * sampleRate
	samples := make([]float64, duration)
	period := sampleRate * 60 / want

	for i := 0; i < duration; i += period {
		for j := 0; j < 500 && i+j < len(samples); j++ {
			samples[i+j] = math.Sin(float64(j) * 0.1)
		}
	}

	got, err := Estimate(samples, sampleRate, Options{Min: 70, Max: 190})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-want) > 3 {
		t.Fatalf("got %.2f BPM, want around %d", got, want)
	}
}
