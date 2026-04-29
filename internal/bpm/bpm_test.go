package bpm

import (
	"math"
	"testing"
)

func TestEstimateSyntheticPulseTrain(t *testing.T) {
	const sampleRate = 11025
	const want = 120
	samples := syntheticPulseTrain(sampleRate, 45, want, 0)

	analysis, err := Analyze(samples, sampleRate, Options{Min: 70, Max: 190})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(analysis.BPM-want) > 1 {
		t.Fatalf("got %.2f BPM, want around %d", analysis.BPM, want)
	}
	if analysis.Confidence <= 0 {
		t.Fatalf("expected positive confidence, got %.2f", analysis.Confidence)
	}
}

func TestEstimateFractionalTempoWithIntroSilence(t *testing.T) {
	const sampleRate = 11025
	const want = 123
	samples := syntheticPulseTrain(sampleRate, 50, want, 8)

	got, err := Estimate(samples, sampleRate, Options{Min: 80, Max: 160})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-want) > 1 {
		t.Fatalf("got %.2f BPM, want around %d", got, want)
	}
}

func TestEstimateDoesNotPreferQuietHiHatSubdivision(t *testing.T) {
	const sampleRate = 11025
	const want = 95
	samples := syntheticBackbeat(sampleRate, 45, want)

	got, err := Estimate(samples, sampleRate, Options{Min: 70, Max: 190})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-want) > 2 {
		t.Fatalf("got %.2f BPM, want around %d", got, want)
	}
}

func TestChooseMusicalTactusPrefersDoubleWhenCompoundIsWeak(t *testing.T) {
	candidates := []Candidate{
		{BPM: 70, Score: 1.00},
		{BPM: 140, Score: 0.94},
		{BPM: 143, Score: 0.90},
	}

	got := chooseMusicalTactus(candidates)

	if got[0].BPM != 140 {
		t.Fatalf("got %.1f BPM, want 140.0", got[0].BPM)
	}
}

func TestChooseMusicalTactusPrefersCompoundMeterWhenItBeatsDouble(t *testing.T) {
	candidates := []Candidate{
		{BPM: 77.2, Score: 1.00},
		{BPM: 102.8, Score: 0.84},
		{BPM: 154.0, Score: 0.71},
	}

	got := chooseMusicalTactus(candidates)

	if got[0].BPM != 102.8 {
		t.Fatalf("got %.1f BPM, want 102.8", got[0].BPM)
	}
}

func TestChooseMusicalTactusPrefersCompoundMeterEvenWhenDoubleIsProminent(t *testing.T) {
	candidates := []Candidate{
		{BPM: 84.8, Score: 1.00},
		{BPM: 169.6, Score: 0.71},
		{BPM: 113.0, Score: 0.50},
	}

	got := chooseMusicalTactus(candidates)

	if got[0].BPM != 113.0 {
		t.Fatalf("got %.1f BPM, want 113.0", got[0].BPM)
	}
}

func TestChooseMusicalTactusPrefersCompoundMeterFromFastCandidate(t *testing.T) {
	candidates := []Candidate{
		{BPM: 170.0, Score: 1.00},
		{BPM: 84.8, Score: 0.99},
		{BPM: 166.8, Score: 0.70},
		{BPM: 112.2, Score: 0.64},
	}

	got := chooseMusicalTactus(candidates)

	if got[0].BPM != 112.2 {
		t.Fatalf("got %.1f BPM, want 112.2", got[0].BPM)
	}
}

func TestChooseMusicalTactusKeepsStrongSlowTempo(t *testing.T) {
	candidates := []Candidate{
		{BPM: 82, Score: 1.00},
		{BPM: 164, Score: 0.60},
	}

	got := chooseMusicalTactus(candidates)

	if got[0].BPM != 82 {
		t.Fatalf("got %.1f BPM, want 82.0", got[0].BPM)
	}
}

func TestAppendRelatedTactusCandidatesKeepsCompoundCandidate(t *testing.T) {
	candidates := []Candidate{
		{BPM: 84.8, Score: 1.00},
		{BPM: 169.6, Score: 0.71},
	}
	raw := []Candidate{
		{BPM: 84.8, Score: 1.00},
		{BPM: 169.6, Score: 0.71},
		{BPM: 113.0, Score: 0.50},
	}

	got := appendRelatedTactusCandidates(candidates, raw)

	if relatedCandidate(got, 113.0, 0.01) < 0 {
		t.Fatalf("expected 113 BPM compound candidate in %v", got)
	}
}

func syntheticPulseTrain(sampleRate int, durationSeconds int, tempo int, introSeconds int) []float64 {
	samples := make([]float64, durationSeconds*sampleRate)
	period := float64(sampleRate) * 60 / float64(tempo)
	for beat := float64(introSeconds * sampleRate); beat < float64(len(samples)); beat += period {
		addKick(samples, int(math.Round(beat)), sampleRate, 1.0)
	}
	return samples
}

func syntheticBackbeat(sampleRate int, durationSeconds int, tempo int) []float64 {
	samples := make([]float64, durationSeconds*sampleRate)
	period := float64(sampleRate) * 60 / float64(tempo)
	for beat := 0.0; beat < float64(len(samples)); beat += period {
		addKick(samples, int(math.Round(beat)), sampleRate, 1.0)
		addClick(samples, int(math.Round(beat+period/2)), sampleRate, 0.18)
	}
	return samples
}

func addKick(samples []float64, start int, sampleRate int, gain float64) {
	length := sampleRate / 18
	for i := 0; i < length && start+i < len(samples); i++ {
		if start+i < 0 {
			continue
		}
		t := float64(i) / float64(sampleRate)
		decay := math.Exp(-18 * t)
		samples[start+i] += gain * decay * math.Sin(2*math.Pi*80*t)
	}
}

func addClick(samples []float64, start int, sampleRate int, gain float64) {
	length := sampleRate / 80
	for i := 0; i < length && start+i < len(samples); i++ {
		if start+i < 0 {
			continue
		}
		t := float64(i) / float64(sampleRate)
		decay := math.Exp(-80 * t)
		samples[start+i] += gain * decay * math.Sin(2*math.Pi*1800*t)
	}
}
