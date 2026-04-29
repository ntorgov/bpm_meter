package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bpm_meter/internal/bpm"
	"bpm_meter/internal/id3"
)

type result struct {
	Path string
	BPM  int
	Err  error
}

func main() {
	var (
		root      = flag.String("path", ".", "file or directory to scan")
		writeTags = flag.Bool("write", false, "write detected BPM to the ID3 TBPM tag")
		workers   = flag.Int("workers", max(1, runtime.NumCPU()-1), "number of files to process in parallel")
		minBPM    = flag.Int("min", 70, "minimum BPM to search")
		maxBPM    = flag.Int("max", 190, "maximum BPM to search")
	)
	flag.Parse()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg is required and was not found in PATH")
		os.Exit(1)
	}
	if *minBPM <= 0 || *maxBPM <= *minBPM {
		fmt.Fprintln(os.Stderr, "invalid BPM range")
		os.Exit(1)
	}

	files, err := collectMP3s(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("No MP3 files found.")
		return
	}

	fmt.Printf("Found %d MP3 file(s). write=%t range=%d-%d\n", len(files), *writeTags, *minBPM, *maxBPM)

	jobs := make(chan string)
	results := make(chan result)
	var wg sync.WaitGroup

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				results <- process(path, *writeTags, *minBPM, *maxBPM)
			}
		}()
	}

	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	failed := 0
	for res := range results {
		if res.Err != nil {
			failed++
			fmt.Printf("ERR  %s: %v\n", res.Path, res.Err)
			continue
		}
		action := "scan"
		if *writeTags {
			action = "wrote"
		}
		fmt.Printf("OK   %3d BPM  %-5s %s\n", res.BPM, action, res.Path)
	}

	if failed > 0 {
		os.Exit(1)
	}
}

func process(path string, writeTag bool, minBPM int, maxBPM int) result {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	samples, sampleRate, err := decodeWithFFmpeg(ctx, path)
	if err != nil {
		return result{Path: path, Err: err}
	}

	tempo, err := bpm.Estimate(samples, sampleRate, bpm.Options{Min: minBPM, Max: maxBPM})
	if err != nil {
		return result{Path: path, Err: err}
	}

	rounded := int(tempo + 0.5)
	if writeTag {
		if err := id3.WriteTBPM(path, rounded); err != nil {
			return result{Path: path, BPM: rounded, Err: err}
		}
	}

	return result{Path: path, BPM: rounded}
}

func collectMP3s(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(root), ".mp3") {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not an MP3 file", root)
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".mp3") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func decodeWithFFmpeg(ctx context.Context, path string) ([]float64, int, error) {
	const sampleRate = 11025

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-i", path,
		"-vn",
		"-ac", "1",
		"-ar", fmt.Sprint(sampleRate),
		"-f", "s16le",
		"pipe:1",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, 0, fmt.Errorf("ffmpeg timed out")
	}
	if err != nil {
		if stderr.Len() > 0 {
			return nil, 0, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, 0, err
	}
	if len(out) < 2 {
		return nil, 0, fmt.Errorf("ffmpeg produced no audio")
	}

	samples := make([]float64, len(out)/2)
	for i := range samples {
		lo := uint16(out[i*2])
		hi := uint16(out[i*2+1]) << 8
		v := int16(hi | lo)
		samples[i] = float64(v) / 32768.0
	}
	return samples, sampleRate, nil
}
