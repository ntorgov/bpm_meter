package bpm

import (
	"fmt"
	"math"
)

type Options struct {
	Min int
	Max int
}

func Estimate(samples []float64, sampleRate int, opts Options) (float64, error) {
	if sampleRate <= 0 {
		return 0, fmt.Errorf("invalid sample rate")
	}
	if len(samples) < sampleRate*15 {
		return 0, fmt.Errorf("audio is too short for BPM estimation")
	}

	const frame = 1024
	const hop = 512

	energy := make([]float64, 0, len(samples)/hop)
	for start := 0; start+frame <= len(samples); start += hop {
		var sum float64
		for _, sample := range samples[start : start+frame] {
			sum += sample * sample
		}
		energy = append(energy, math.Sqrt(sum/float64(frame)))
	}
	if len(energy) < 4 {
		return 0, fmt.Errorf("not enough audio frames")
	}

	envelope := positiveDiffEnvelope(energy)
	normalize(envelope)

	rate := float64(sampleRate) / hop
	minLag := int(math.Floor(60.0 * rate / float64(opts.Max)))
	maxLag := int(math.Ceil(60.0 * rate / float64(opts.Min)))
	if minLag < 1 {
		minLag = 1
	}
	if maxLag >= len(envelope)/2 {
		maxLag = len(envelope)/2 - 1
	}
	if maxLag <= minLag {
		return 0, fmt.Errorf("BPM range is too wide for the audio length")
	}

	bestLag := minLag
	bestScore := math.Inf(-1)
	for lag := minLag; lag <= maxLag; lag++ {
		score := autocorrelation(envelope, lag)
		score += 0.45 * autocorrelation(envelope, lag*2)
		if lag%2 == 0 {
			score += 0.20 * autocorrelation(envelope, lag/2)
		}
		if score > bestScore {
			bestScore = score
			bestLag = lag
		}
	}

	tempo := 60.0 * rate / float64(bestLag)
	for tempo < float64(opts.Min) {
		tempo *= 2
	}
	for tempo > float64(opts.Max) {
		tempo /= 2
	}
	return tempo, nil
}

func positiveDiffEnvelope(values []float64) []float64 {
	out := make([]float64, len(values)-1)
	prev := values[0]
	for i := 1; i < len(values); i++ {
		diff := values[i] - prev
		if diff > 0 {
			out[i-1] = diff
		}
		prev = values[i]
	}
	return out
}

func normalize(values []float64) {
	var mean float64
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))

	var variance float64
	for i, value := range values {
		centered := value - mean
		values[i] = centered
		variance += centered * centered
	}

	stddev := math.Sqrt(variance/float64(len(values))) + 1e-12
	for i := range values {
		values[i] /= stddev
	}
}

func autocorrelation(values []float64, lag int) float64 {
	if lag <= 0 || lag >= len(values) {
		return 0
	}
	var sum float64
	limit := len(values) - lag
	for i := 0; i < limit; i++ {
		sum += values[i] * values[i+lag]
	}
	return sum / float64(limit)
}
