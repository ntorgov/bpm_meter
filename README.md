# BPM Meter

CLI utility for scanning an MP3 collection, estimating track BPM, and optionally writing the value to the ID3 `TBPM` tag.

## Why Go

Go is a good fit for this project because the tool is mostly batch file processing: directory walking, parallel workers, external process orchestration, and a small amount of DSP. The resulting binary is easy to run on macOS/Linux/Windows. MP3 decoding is delegated to `ffmpeg`, which is more reliable than maintaining decoder code in the app.

## Requirements

- Go 1.25+
- `ffmpeg` in `PATH`

## Usage

Scan without changing files:

```bash
go run . -path /path/to/music
```

Write detected BPM to tags:

```bash
go run . -path /path/to/music -write
```

Tune the expected tempo range:

```bash
go run . -path /path/to/music -min 80 -max 180 -write
```

Process a single file:

```bash
go run . -path ./track.mp3
```

## Notes

- The BPM estimator uses a lightweight onset-envelope autocorrelation approach. It is suitable as a first pass for running playlists, but electronic tracks, half-time/double-time material, live recordings, and tracks with long quiet intros may need manual review.
- `-write` updates or creates an ID3v2 `TBPM` frame. Existing ID3v2.3 and ID3v2.4 frames are preserved when the tag does not use advanced flags.
- Keep a backup of the collection before running bulk tag writes.
