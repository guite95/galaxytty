package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxFrameSize uint32 = 1024 * 1024

var (
	ErrEmptyFrame    = errors.New("empty protocol frame")
	ErrFrameTooLarge = errors.New("protocol frame too large")
)

func WriteFrame(writer io.Writer, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate protocol envelope: %w", err)
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal protocol envelope: %w", err)
	}
	if len(payload) == 0 {
		return ErrEmptyFrame
	}
	if len(payload) > int(MaxFrameSize) {
		return fmt.Errorf("%w: %d", ErrFrameTooLarge, len(payload))
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if err := writeAll(writer, header); err != nil {
		return fmt.Errorf("write protocol frame length: %w", err)
	}
	if err := writeAll(writer, payload); err != nil {
		return fmt.Errorf("write protocol frame payload: %w", err)
	}
	return nil
}

func ReadFrame(reader io.Reader) (Envelope, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return Envelope{}, fmt.Errorf("read protocol frame length: %w", err)
	}
	length := binary.BigEndian.Uint32(header)
	switch {
	case length == 0:
		return Envelope{}, ErrEmptyFrame
	case length > MaxFrameSize:
		return Envelope{}, fmt.Errorf("%w: %d", ErrFrameTooLarge, length)
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return Envelope{}, fmt.Errorf("read protocol frame payload: %w", err)
	}
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode protocol envelope: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("validate protocol envelope: %w", err)
	}
	return envelope, nil
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}
