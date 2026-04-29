# BPM Meter

CLI utility for scanning an MP3 collection, estimating track BPM, and optionally writing the value to the ID3 `TBPM` tag.

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

Prefer a target range after musical tempo detection, useful when you want running-cadence tags instead of musical BPM:

```bash
go run . -path /path/to/music -prefer-min 120 -prefer-max 180
```

Alternative tempo candidates are shown by default. Add scores when you want to inspect candidate strength:

```bash
go run . -path /path/to/music -scores
```

Process a single file:

```bash
go run . -path ./track.mp3
```

## Verification

Inspect ambiguous tracks before writing tags:

```bash
go run . -path ./track.mp3 -scores
```

If another tool reports a plausible BPM, test that hypothesis with a tighter range:

```bash
go run . -path ./track.mp3 -min 90 -max 115
```

When the tight-range result has much higher confidence, the track probably has a real meter ambiguity rather than a simple detector miss. This is common with half-time, double-time, and compound meters such as 6/8.

## Notes

- The BPM estimator combines spectral flux onset detection, fractional autocorrelation, local-window voting, and inter-onset interval evidence. This is much less coarse than simple energy autocorrelation, but electronic tracks, half-time/double-time material, live recordings, and tracks with variable tempo may still need manual review.
- Low `conf` values mean the detector found competing tempo candidates. Run with `-scores` to inspect likely half/double-tempo and compound-meter choices before bulk writing tags. By default the tool writes musical BPM; use `-prefer-min 120 -prefer-max 180` only when you deliberately want running-cadence-oriented values.
- `-write` updates or creates an ID3v2 `TBPM` frame. Existing ID3v2.3 and ID3v2.4 frames are preserved when the tag does not use advanced flags.
- Keep a backup of the collection before running bulk tag writes.
