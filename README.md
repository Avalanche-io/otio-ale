# ALE Adapter for OpenTimelineIO (Go)

This package provides an Avid Log Exchange (ALE) adapter for OpenTimelineIO in Go.

## Overview

ALE is a tab-separated text file format commonly used in the post-production industry for exchanging video logging data. This adapter allows you to read ALE files into OTIO timelines and write OTIO timelines as ALE files.

## Installation

```bash
go get github.com/mrjoshuak/otio-ale
```

For local development with the gotio library:

```bash
go mod edit -replace github.com/mrjoshuak/gotio=../gotio
go mod tidy
```

## Usage

### Decoding ALE Files

```go
package main

import (
    "os"
    "github.com/mrjoshuak/otio-ale"
)

func main() {
    file, err := os.Open("timeline.ale")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    // Create decoder with options
    decoder := ale.NewDecoder(
        file,
        ale.WithFPS(24.0),
        ale.WithNameColumn("Name"),
    )

    // Decode to OTIO timeline
    timeline, err := decoder.Decode()
    if err != nil {
        panic(err)
    }

    // Use timeline...
}
```

### Encoding OTIO Timelines to ALE

```go
package main

import (
    "os"
    "github.com/mrjoshuak/otio-ale"
    "github.com/mrjoshuak/gotio/opentimelineio"
)

func main() {
    // Create or load a timeline
    timeline := opentimelineio.NewTimeline("My Timeline", nil, nil)
    // ... add tracks and clips ...

    file, err := os.Create("output.ale")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    // Create encoder with options
    encoder := ale.NewEncoder(
        file,
        ale.WithEncoderFPS(24.0),
        ale.WithEncoderDropFrame(false),
    )

    // Encode timeline to ALE
    err = encoder.Encode(timeline)
    if err != nil {
        panic(err)
    }
}
```

## ALE Format

ALE files consist of three sections:

1. **Heading**: Metadata about the file format and project settings
   - `FIELD_DELIM`: Field delimiter (usually `TABS`)
   - `VIDEO_FORMAT`: Video format specification
   - `AUDIO_FORMAT`: Audio format specification
   - `FPS`: Frames per second

2. **Column**: Defines the column headers for the data section
   - Common columns: `Name`, `Start`, `End`, `Duration`, `Tracks`, `Source File`, `Tape`

3. **Data**: Tab-separated values, one row per clip

Example:
```
Heading
FIELD_DELIM	TABS
VIDEO_FORMAT	1920x1080
AUDIO_FORMAT	48kHz
FPS	24.00

Column
Name	Start	End	Duration	Tracks

Data
Clip001	01:00:00:00	01:00:05:00	120	V
Clip002	01:00:05:00	01:00:10:00	120	V
```

## Features

- Parse ALE files into OTIO timelines
- Export OTIO timelines as ALE files
- Support for timecodes (drop-frame and non-drop-frame)
- Configurable frame rates
- Custom column support
- Metadata preservation
- External media references

## Options

### Decoder Options

- `WithFPS(fps float64)`: Set the frame rate (default: 24.0)
- `WithNameColumn(key string)`: Set the column name for clip names (default: "Name")
- `WithDropFrame(dropFrame bool)`: Use drop-frame timecode

### Encoder Options

- `WithEncoderFPS(fps float64)`: Set the frame rate for output (default: 24.0)
- `WithEncoderDropFrame(dropFrame bool)`: Use drop-frame timecode
- `WithColumns(columns []string)`: Specify exact columns to include

## Testing

Run the test suite:

```bash
go test -v
```

## License

Apache-2.0

## References

- [OpenTimelineIO](https://github.com/AcademySoftwareFoundation/OpenTimelineIO)
- [Avid Log Exchange Format](https://pomfort.com/article/ale-avid-log-exchange-files-what-they-are-and-why-you-should-understand-them/)
