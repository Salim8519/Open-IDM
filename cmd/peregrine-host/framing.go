package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Chrome native messaging framing: a uint32 length in native byte order (little-endian on
// every platform we support) followed by that many bytes of UTF-8 JSON.
const (
	maxInMessage  = 1 << 20 // browser -> host limit we accept (Chrome allows 64 MiB; we need far less)
	maxOutMessage = 1 << 20 // host -> browser hard limit enforced by Chrome
)

var (
	errTooLarge   = errors.New("message too large")
	errEmptyFrame = errors.New("empty message")
)

// readFrame reads one framed message. It returns io.EOF only on a clean end of input
// (no bytes of a new frame read).
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("truncated header: %w", err)
		}
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, errEmptyFrame
	}
	if n > maxInMessage {
		return nil, fmt.Errorf("%w: %d bytes", errTooLarge, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("truncated body: %w", err)
	}
	return buf, nil
}

// writeFrame writes one framed message.
func writeFrame(w io.Writer, msg []byte) error {
	if len(msg) > maxOutMessage {
		return fmt.Errorf("%w: %d bytes", errTooLarge, len(msg))
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(msg)))
	if _, err := w.Write(append(hdr[:], msg...)); err != nil {
		return err
	}
	return nil
}
