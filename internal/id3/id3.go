package id3

import (
	"bytes"
	"fmt"
	"os"
)

const (
	headerSize = 10
	tbpmFrame  = "TBPM"
)

func WriteTBPM(path string, bpm int) error {
	if bpm <= 0 {
		return fmt.Errorf("invalid BPM value")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	audioStart := 0
	version := byte(3)
	var frames []byte

	if len(data) >= headerSize && string(data[:3]) == "ID3" {
		version = data[3]
		if version != 3 && version != 4 {
			return fmt.Errorf("unsupported ID3v2.%d tag", version)
		}
		if data[5] != 0 {
			return fmt.Errorf("unsupported ID3 tag flags 0x%02x", data[5])
		}
		tagSize, ok := decodeSynchsafe(data[6:10])
		if !ok {
			return fmt.Errorf("invalid ID3 tag size")
		}
		audioStart = headerSize + tagSize
		if audioStart > len(data) {
			return fmt.Errorf("ID3 tag exceeds file size")
		}
		frames = stripTextFrame(data[headerSize:audioStart], version, tbpmFrame)
	}

	frames = append(frames, makeTextFrame(version, tbpmFrame, fmt.Sprint(bpm))...)
	header := make([]byte, headerSize)
	copy(header[:3], "ID3")
	header[3] = version
	header[4] = 0
	header[5] = 0
	copy(header[6:10], encodeSynchsafe(len(frames)))

	next := make([]byte, 0, len(header)+len(frames)+len(data)-audioStart)
	next = append(next, header...)
	next = append(next, frames...)
	next = append(next, data[audioStart:]...)

	return os.WriteFile(path, next, 0644)
}

func stripTextFrame(frames []byte, version byte, id string) []byte {
	var out bytes.Buffer
	for pos := 0; pos+10 <= len(frames); {
		header := frames[pos : pos+10]
		if isPadding(header) {
			break
		}

		frameID := string(header[:4])
		size, ok := frameSize(header[4:8], version)
		if !ok || size < 0 || pos+10+size > len(frames) {
			out.Write(frames[pos:])
			break
		}

		if frameID != id {
			out.Write(frames[pos : pos+10+size])
		}
		pos += 10 + size
	}
	return out.Bytes()
}

func makeTextFrame(version byte, id string, value string) []byte {
	payload := append([]byte{0}, []byte(value)...)
	frame := make([]byte, 10, 10+len(payload))
	copy(frame[:4], id)
	if version == 4 {
		copy(frame[4:8], encodeSynchsafe(len(payload)))
	} else {
		putUint32(frame[4:8], len(payload))
	}
	frame = append(frame, payload...)
	return frame
}

func frameSize(raw []byte, version byte) (int, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	if version == 4 {
		return decodeSynchsafe(raw)
	}
	return int(raw[0])<<24 | int(raw[1])<<16 | int(raw[2])<<8 | int(raw[3]), true
}

func isPadding(raw []byte) bool {
	for _, b := range raw {
		if b != 0 {
			return false
		}
	}
	return true
}

func putUint32(dst []byte, value int) {
	dst[0] = byte(value >> 24)
	dst[1] = byte(value >> 16)
	dst[2] = byte(value >> 8)
	dst[3] = byte(value)
}

func encodeSynchsafe(value int) []byte {
	return []byte{
		byte((value >> 21) & 0x7f),
		byte((value >> 14) & 0x7f),
		byte((value >> 7) & 0x7f),
		byte(value & 0x7f),
	}
}

func decodeSynchsafe(raw []byte) (int, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	value := 0
	for _, b := range raw {
		if b&0x80 != 0 {
			return 0, false
		}
		value = (value << 7) | int(b)
	}
	return value, true
}
