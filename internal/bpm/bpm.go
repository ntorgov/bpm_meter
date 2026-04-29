package bpm

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"
)

type Options struct {
	Min       int
	Max       int
	PreferMin int
	PreferMax int
}

type Analysis struct {
	BPM          float64
	Confidence   float64
	Alternatives []Candidate
}

type Candidate struct {
	BPM   float64
	Score float64
	Pulse float64
}

type onsetPeak struct {
	Index int
	Value float64
}

func Estimate(samples []float64, sampleRate int, opts Options) (float64, error) {
	analysis, err := Analyze(samples, sampleRate, opts)
	if err != nil {
		return 0, err
	}
	return analysis.BPM, nil
}

func Analyze(samples []float64, sampleRate int, opts Options) (Analysis, error) {
	if sampleRate <= 0 {
		return Analysis{}, fmt.Errorf("invalid sample rate")
	}
	if opts.Min <= 0 || opts.Max <= opts.Min {
		return Analysis{}, fmt.Errorf("invalid BPM range")
	}
	if len(samples) < sampleRate*15 {
		return Analysis{}, fmt.Errorf("audio is too short for BPM estimation")
	}

	envelope, envelopeRate, err := onsetEnvelope(samples, sampleRate)
	if err != nil {
		return Analysis{}, err
	}

	candidates, err := tempoCandidates(envelope, envelopeRate, opts)
	if err != nil {
		return Analysis{}, err
	}
	candidates = chooseMusicalTactus(candidates)
	candidates = choosePreferredTempo(candidates, opts)

	return Analysis{
		BPM:          candidates[0].BPM,
		Confidence:   confidence(candidates),
		Alternatives: candidates[:min(5, len(candidates))],
	}, nil
}

func onsetEnvelope(samples []float64, sampleRate int) ([]float64, float64, error) {
	const frame = 1024
	const hop = 256

	if len(samples) < frame*4 {
		return nil, 0, fmt.Errorf("not enough audio frames")
	}

	window := hann(frame)
	spectrum := make([]complex128, frame)
	prevMagnitude := make([]float64, frame/2+1)
	hasPrev := false

	spectralFlux := make([]float64, 0, len(samples)/hop)
	energy := make([]float64, 0, len(samples)/hop)

	for start := 0; start+frame <= len(samples); start += hop {
		var frameEnergy float64
		for i := 0; i < frame; i++ {
			sample := samples[start+i]
			frameEnergy += sample * sample
			spectrum[i] = complex(sample*window[i], 0)
		}

		fft(spectrum)

		var flux float64
		for bin := 1; bin <= frame/2; bin++ {
			frequency := float64(bin) * float64(sampleRate) / frame
			if frequency < 35 || frequency > 5200 {
				continue
			}

			magnitude := math.Log1p(cmplx.Abs(spectrum[bin]) * 10)
			if hasPrev {
				diff := magnitude - prevMagnitude[bin]
				if diff > 0 {
					flux += bandWeight(frequency) * diff
				}
			}
			prevMagnitude[bin] = magnitude
		}
		hasPrev = true

		spectralFlux = append(spectralFlux, flux)
		energy = append(energy, math.Sqrt(frameEnergy/float64(frame)))
	}

	if len(spectralFlux) < 8 {
		return nil, 0, fmt.Errorf("not enough onset frames")
	}

	spectralFlux = spectralFlux[1:]
	energyFlux := positiveDiffEnvelope(energy)

	spectralFlux = enhanceOnsets(spectralFlux, 16)
	energyFlux = enhanceOnsets(energyFlux, 16)

	envelope := make([]float64, min(len(spectralFlux), len(energyFlux)))
	for i := range envelope {
		envelope[i] = spectralFlux[i] + 0.35*energyFlux[i]
	}
	normalize(envelope)

	return envelope, float64(sampleRate) / hop, nil
}

func tempoCandidates(envelope []float64, envelopeRate float64, opts Options) ([]Candidate, error) {
	minLag := 60.0 * envelopeRate / float64(opts.Max)
	maxLag := 60.0 * envelopeRate / float64(opts.Min)
	if minLag < 2 || maxLag >= float64(len(envelope)-2) {
		return nil, fmt.Errorf("BPM range is too wide for the audio length")
	}

	const step = 0.25
	var raw []Candidate
	for tempo := float64(opts.Min); tempo <= float64(opts.Max)+1e-9; tempo += step {
		score, pulse := scoreTempo(envelope, envelopeRate, tempo)
		raw = append(raw, Candidate{BPM: tempo, Score: score, Pulse: pulse})
	}

	segmentScores, segmentPulses := segmentTempoEvidence(envelope, envelopeRate, raw)
	ioiScores := ioiTempoEvidence(envelope, envelopeRate, raw, opts)
	globalMax := maxCandidateScore(raw)
	segmentMax := maxFloat(segmentScores)
	ioiMax := maxFloat(ioiScores)
	for i := range raw {
		globalScore := normalizedPositive(raw[i].Score, globalMax)
		segmentScore := normalizedPositive(segmentScores[i], segmentMax)
		ioiScore := normalizedPositive(ioiScores[i], ioiMax)
		raw[i].Score = 0.45*globalScore + 0.25*segmentScore + 0.30*ioiScore
		if segmentPulses[i] > 0 {
			raw[i].Pulse = 0.55*math.Max(0, raw[i].Pulse) + 0.25*segmentPulses[i] + 0.20*ioiScores[i]
		}
	}

	sort.Slice(raw, func(i, j int) bool {
		return raw[i].Score > raw[j].Score
	})

	distinct := make([]Candidate, 0, min(10, len(raw)))
	for _, candidate := range raw {
		if isDistinctTempo(candidate.BPM, distinct) {
			distinct = append(distinct, candidate)
			if len(distinct) == 10 {
				break
			}
		}
	}
	if len(distinct) == 0 || distinct[0].Score <= 0 {
		return nil, fmt.Errorf("could not find a stable tempo")
	}
	return distinct, nil
}

func scoreTempo(envelope []float64, envelopeRate float64, tempo float64) (float64, float64) {
	lag := 60.0 * envelopeRate / tempo
	main := autocorrelation(envelope, lag)
	secondHarmonic := math.Max(0, autocorrelation(envelope, lag*2))
	thirdHarmonic := math.Max(0, autocorrelation(envelope, lag*3))

	score := main + 0.20*secondHarmonic + 0.08*thirdHarmonic
	if lag >= 4 {
		subdivision := autocorrelation(envelope, lag/2)
		if subdivision > main {
			score -= 0.20 * (subdivision - main)
		}
	}
	return score, main
}

func segmentTempoEvidence(envelope []float64, envelopeRate float64, tempoGrid []Candidate) ([]float64, []float64) {
	scores := make([]float64, len(tempoGrid))
	pulses := make([]float64, len(tempoGrid))

	window := int(30 * envelopeRate)
	step := int(10 * envelopeRate)
	if window <= 0 || step <= 0 || len(envelope) < window*2 {
		return scores, pulses
	}

	for start := 0; start+window <= len(envelope); start += step {
		segment := envelope[start : start+window]
		local := make([]Candidate, len(tempoGrid))
		for i, candidate := range tempoGrid {
			score, pulse := scoreTempo(segment, envelopeRate, candidate.BPM)
			local[i] = Candidate{BPM: candidate.BPM, Score: score, Pulse: pulse}
		}

		sort.Slice(local, func(i, j int) bool {
			return local[i].Score > local[j].Score
		})

		var voted []Candidate
		for _, candidate := range local {
			if candidate.Score <= 0 || !isDistinctTempo(candidate.BPM, voted) {
				continue
			}
			index := nearestTempoIndex(tempoGrid, candidate.BPM)
			weight := 1.0 / float64(len(voted)+1)
			scores[index] += weight * candidate.Score
			pulses[index] += weight * math.Max(0, candidate.Pulse)
			voted = append(voted, candidate)
			if len(voted) == 4 {
				break
			}
		}
	}

	return scores, pulses
}

func ioiTempoEvidence(envelope []float64, envelopeRate float64, tempoGrid []Candidate, opts Options) []float64 {
	scores := make([]float64, len(tempoGrid))
	peaks := onsetPeaks(envelope, envelopeRate)
	if len(peaks) < 4 {
		return scores
	}

	maxInterval := 4 * 60.0 / float64(opts.Min)
	for i := 0; i < len(peaks); i++ {
		for j := i + 1; j < len(peaks); j++ {
			distanceSeconds := float64(peaks[j].Index-peaks[i].Index) / envelopeRate
			if distanceSeconds > maxInterval {
				break
			}
			if distanceSeconds <= 0 {
				continue
			}

			for beatSpan := 1; beatSpan <= 4; beatSpan++ {
				tempo := 60.0 * float64(beatSpan) / distanceSeconds
				if tempo < float64(opts.Min) || tempo > float64(opts.Max) {
					continue
				}
				index := nearestTempoIndex(tempoGrid, tempo)
				weight := math.Sqrt(peaks[i].Value*peaks[j].Value) / float64(beatSpan)
				scores[index] += weight
			}
		}
	}

	return scores
}

func onsetPeaks(envelope []float64, envelopeRate float64) []onsetPeak {
	minDistance := max(1, int(0.10*envelopeRate))
	threshold := 0.35
	var peaks []onsetPeak

	for i := 1; i+1 < len(envelope); i++ {
		value := envelope[i]
		if value < threshold || value < envelope[i-1] || value < envelope[i+1] {
			continue
		}
		peak := onsetPeak{Index: i, Value: value}
		if len(peaks) == 0 || i-peaks[len(peaks)-1].Index >= minDistance {
			peaks = append(peaks, peak)
			continue
		}
		if value > peaks[len(peaks)-1].Value {
			peaks[len(peaks)-1] = peak
		}
	}

	return peaks
}

func nearestTempoIndex(tempoGrid []Candidate, tempo float64) int {
	if len(tempoGrid) == 0 {
		return 0
	}
	best := 0
	bestDistance := math.Abs(tempoGrid[0].BPM - tempo)
	for i := 1; i < len(tempoGrid); i++ {
		distance := math.Abs(tempoGrid[i].BPM - tempo)
		if distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func maxCandidateScore(candidates []Candidate) float64 {
	var best float64
	for _, candidate := range candidates {
		if candidate.Score > best {
			best = candidate.Score
		}
	}
	return best
}

func maxFloat(values []float64) float64 {
	var best float64
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func normalizedPositive(value float64, maxValue float64) float64 {
	if value <= 0 || maxValue <= 0 {
		return 0
	}
	return value / maxValue
}

func chooseMusicalTactus(candidates []Candidate) []Candidate {
	if len(candidates) < 2 {
		return candidates
	}

	best := candidates[0]
	if best.BPM >= 90 {
		return candidates
	}

	doubleIndex := relatedCandidate(candidates, best.BPM*2, 0.08)
	compoundIndex := relatedCandidate(candidates, best.BPM*4.0/3.0, 0.08)

	if compoundIndex >= 0 && candidates[compoundIndex].Score >= best.Score*0.55 {
		if doubleIndex < 0 || candidates[compoundIndex].Score >= candidates[doubleIndex].Score*0.90 {
			return promoteCandidate(candidates, compoundIndex)
		}
	}
	if doubleIndex >= 0 && candidates[doubleIndex].Score >= best.Score*0.65 {
		return promoteCandidate(candidates, doubleIndex)
	}

	return candidates
}

func choosePreferredTempo(candidates []Candidate, opts Options) []Candidate {
	if len(candidates) < 2 {
		return candidates
	}

	preferMin, preferMax := preferredRange(opts)
	if preferMin <= 0 || preferMax <= preferMin {
		return candidates
	}

	best := candidates[0]
	if best.BPM >= preferMin && best.BPM <= preferMax {
		return candidates
	}

	var chosen Candidate
	chosenIndex := -1
	for i := 1; i < len(candidates); i++ {
		candidate := candidates[i]
		if candidate.BPM < preferMin || candidate.BPM > preferMax {
			continue
		}
		if !isTempoOctave(best.BPM, candidate.BPM) {
			continue
		}
		if candidate.Score < best.Score*0.65 {
			continue
		}
		if chosenIndex == -1 || candidatePulse(candidate) > candidatePulse(chosen) {
			chosen = candidate
			chosenIndex = i
		}
	}
	if chosenIndex == -1 {
		return candidates
	}

	return promoteCandidate(candidates, chosenIndex)
}

func promoteCandidate(candidates []Candidate, index int) []Candidate {
	if index <= 0 || index >= len(candidates) {
		return candidates
	}

	out := make([]Candidate, 0, len(candidates))
	out = append(out, candidates[index])
	for i, candidate := range candidates {
		if i != index {
			out = append(out, candidate)
		}
	}
	return out
}

func relatedCandidate(candidates []Candidate, target float64, tolerance float64) int {
	bestIndex := -1
	bestDistance := math.Inf(1)
	for i := 1; i < len(candidates); i++ {
		distance := math.Abs(candidates[i].BPM-target) / target
		if distance <= tolerance && distance < bestDistance {
			bestIndex = i
			bestDistance = distance
		}
	}
	return bestIndex
}

func candidatePulse(candidate Candidate) float64 {
	if candidate.Pulse != 0 {
		return candidate.Pulse
	}
	return candidate.Score
}

func preferredRange(opts Options) (float64, float64) {
	if opts.PreferMin == 0 && opts.PreferMax == 0 {
		return 0, 0
	}

	preferMin := float64(opts.PreferMin)
	preferMax := float64(opts.PreferMax)
	if preferMin <= 0 {
		preferMin = float64(opts.Min)
	}
	if preferMax <= 0 {
		preferMax = float64(opts.Max)
	}
	preferMin = math.Max(preferMin, float64(opts.Min))
	preferMax = math.Min(preferMax, float64(opts.Max))
	return preferMin, preferMax
}

func isTempoOctave(a float64, b float64) bool {
	lo := math.Min(a, b)
	hi := math.Max(a, b)
	if lo <= 0 {
		return false
	}
	ratio := hi / lo
	for ratio > 2.05 {
		ratio /= 2
	}
	return math.Abs(ratio-2) <= 0.12
}

func bandWeight(frequency float64) float64 {
	switch {
	case frequency < 120:
		return 1.25
	case frequency < 500:
		return 1.15
	case frequency < 2500:
		return 1.0
	default:
		return 0.75
	}
}

func enhanceOnsets(values []float64, radius int) []float64 {
	localMean := movingAverage(values, radius)
	out := make([]float64, len(values))
	var peak float64
	for i, value := range values {
		onset := value - localMean[i]
		if onset < 0 {
			onset = 0
		}
		out[i] = onset
		if onset > peak {
			peak = onset
		}
	}
	if peak <= 0 {
		return out
	}
	for i := range out {
		out[i] /= peak
	}
	return out
}

func movingAverage(values []float64, radius int) []float64 {
	out := make([]float64, len(values))
	prefix := make([]float64, len(values)+1)
	for i, value := range values {
		prefix[i+1] = prefix[i] + value
	}

	for i := range values {
		start := max(0, i-radius)
		end := min(len(values), i+radius+1)
		out[i] = (prefix[end] - prefix[start]) / float64(end-start)
	}
	return out
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
	if len(values) == 0 {
		return
	}

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

func autocorrelation(values []float64, lag float64) float64 {
	if lag <= 0 || lag >= float64(len(values)-1) {
		return 0
	}

	limit := len(values) - int(math.Ceil(lag)) - 1
	if limit <= 0 {
		return 0
	}

	var sumXY, sumX2, sumY2 float64
	for i := 0; i < limit; i++ {
		x := values[i]
		y := interpolate(values, float64(i)+lag)
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}
	if sumX2 <= 0 || sumY2 <= 0 {
		return 0
	}
	return sumXY / math.Sqrt(sumX2*sumY2)
}

func interpolate(values []float64, position float64) float64 {
	left := int(position)
	right := left + 1
	if left < 0 || right >= len(values) {
		return 0
	}
	fraction := position - float64(left)
	return values[left]*(1-fraction) + values[right]*fraction
}

func isDistinctTempo(tempo float64, candidates []Candidate) bool {
	for _, candidate := range candidates {
		if math.Abs(tempo-candidate.BPM) < 3.0 {
			return false
		}
	}
	return true
}

func confidence(candidates []Candidate) float64 {
	if len(candidates) == 0 || candidates[0].Score <= 0 {
		return 0
	}
	if len(candidates) == 1 {
		return clamp01(candidates[0].Score)
	}

	best := candidates[0].Score
	second := candidates[1].Score
	gap := (best - second) / (math.Abs(best) + 1e-12)
	return clamp01(gap)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func hann(size int) []float64 {
	window := make([]float64, size)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size-1))
	}
	return window
}

func fft(values []complex128) {
	n := len(values)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wlen := complex(math.Cos(angle), math.Sin(angle))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for j := 0; j < length/2; j++ {
				u := values[i+j]
				v := values[i+j+length/2] * w
				values[i+j] = u + v
				values[i+j+length/2] = u - v
				w *= wlen
			}
		}
	}
}
