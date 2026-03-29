// Package rpc implements the libvirt RPC protocol for libvirt-sim.
package rpc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// XDREncoder writes XDR-encoded values to a buffer.
type XDREncoder struct {
	buf []byte
}

// NewXDREncoder creates a new XDR encoder.
func NewXDREncoder() *XDREncoder {
	return &XDREncoder{}
}

// Bytes returns the encoded bytes.
func (e *XDREncoder) Bytes() []byte {
	return e.buf
}

// WriteInt32 writes an XDR int32.
func (e *XDREncoder) WriteInt32(v int32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	e.buf = append(e.buf, b...)
}

// WriteUint32 writes an XDR uint32.
func (e *XDREncoder) WriteUint32(v uint32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	e.buf = append(e.buf, b...)
}

// WriteInt64 writes an XDR int64 (hyper).
func (e *XDREncoder) WriteInt64(v int64) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	e.buf = append(e.buf, b...)
}

// WriteUint64 writes an XDR uint64 (unsigned hyper).
func (e *XDREncoder) WriteUint64(v uint64) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	e.buf = append(e.buf, b...)
}

// WriteString writes an XDR variable-length string.
func (e *XDREncoder) WriteString(s string) {
	e.WriteUint32(uint32(len(s)))
	e.buf = append(e.buf, s...)
	// Pad to 4-byte boundary
	pad := (4 - len(s)%4) % 4
	for i := 0; i < pad; i++ {
		e.buf = append(e.buf, 0)
	}
}

// WriteOptionalString writes an XDR optional string (discriminant + string if present).
func (e *XDREncoder) WriteOptionalString(s *string) {
	if s == nil {
		e.WriteUint32(0) // absent
	} else {
		e.WriteUint32(1) // present
		e.WriteString(*s)
	}
}

// WriteFixedOpaque writes fixed-length opaque data with padding.
func (e *XDREncoder) WriteFixedOpaque(data []byte, size int) {
	padded := make([]byte, size)
	copy(padded, data)
	e.buf = append(e.buf, padded...)
	// Pad to 4-byte boundary
	pad := (4 - size%4) % 4
	for i := 0; i < pad; i++ {
		e.buf = append(e.buf, 0)
	}
}

// WriteUUID writes a 16-byte UUID (fixed opaque, no extra padding since 16 is 4-aligned).
func (e *XDREncoder) WriteUUID(uuid [16]byte) {
	e.buf = append(e.buf, uuid[:]...)
}

// WriteBool writes an XDR bool.
func (e *XDREncoder) WriteBool(v bool) {
	if v {
		e.WriteUint32(1)
	} else {
		e.WriteUint32(0)
	}
}

// WriteUint8 writes a uint8 as an XDR unsigned int (4 bytes).
func (e *XDREncoder) WriteUint8(v uint8) {
	e.WriteUint32(uint32(v))
}

// WriteUint16 writes a uint16 as an XDR unsigned int (4 bytes).
func (e *XDREncoder) WriteUint16(v uint16) {
	e.WriteUint32(uint32(v))
}

// XDRDecoder reads XDR-encoded values from a byte slice.
type XDRDecoder struct {
	data []byte
	pos  int
}

// NewXDRDecoder creates a new XDR decoder.
func NewXDRDecoder(data []byte) *XDRDecoder {
	return &XDRDecoder{data: data}
}

// Remaining returns the number of unread bytes.
func (d *XDRDecoder) Remaining() int {
	return len(d.data) - d.pos
}

// ReadInt32 reads an XDR int32.
func (d *XDRDecoder) ReadInt32() (int32, error) {
	if d.pos+4 > len(d.data) {
		return 0, fmt.Errorf("read int32: %w", io.ErrUnexpectedEOF)
	}
	v := int32(binary.BigEndian.Uint32(d.data[d.pos:]))
	d.pos += 4
	return v, nil
}

// ReadUint32 reads an XDR uint32.
func (d *XDRDecoder) ReadUint32() (uint32, error) {
	if d.pos+4 > len(d.data) {
		return 0, fmt.Errorf("read uint32: %w", io.ErrUnexpectedEOF)
	}
	v := binary.BigEndian.Uint32(d.data[d.pos:])
	d.pos += 4
	return v, nil
}

// ReadInt64 reads an XDR int64.
func (d *XDRDecoder) ReadInt64() (int64, error) {
	if d.pos+8 > len(d.data) {
		return 0, fmt.Errorf("read int64: %w", io.ErrUnexpectedEOF)
	}
	v := int64(binary.BigEndian.Uint64(d.data[d.pos:]))
	d.pos += 8
	return v, nil
}

// ReadUint64 reads an XDR uint64.
func (d *XDRDecoder) ReadUint64() (uint64, error) {
	if d.pos+8 > len(d.data) {
		return 0, fmt.Errorf("read uint64: %w", io.ErrUnexpectedEOF)
	}
	v := binary.BigEndian.Uint64(d.data[d.pos:])
	d.pos += 8
	return v, nil
}

// ReadString reads an XDR variable-length string.
func (d *XDRDecoder) ReadString() (string, error) {
	length, err := d.ReadUint32()
	if err != nil {
		return "", fmt.Errorf("read string length: %w", err)
	}
	if d.pos+int(length) > len(d.data) {
		return "", fmt.Errorf("read string data (len=%d): %w", length, io.ErrUnexpectedEOF)
	}
	s := string(d.data[d.pos : d.pos+int(length)])
	d.pos += int(length)
	// Skip padding
	pad := (4 - int(length)%4) % 4
	d.pos += pad
	return s, nil
}

// ReadOptionalString reads an XDR optional string.
func (d *XDRDecoder) ReadOptionalString() (*string, error) {
	present, err := d.ReadUint32()
	if err != nil {
		return nil, fmt.Errorf("read optional string discriminant: %w", err)
	}
	if present == 0 {
		return nil, nil
	}
	s, err := d.ReadString()
	if err != nil {
		return nil, fmt.Errorf("read optional string value: %w", err)
	}
	return &s, nil
}

// ReadUUID reads a 16-byte UUID.
func (d *XDRDecoder) ReadUUID() ([16]byte, error) {
	var uuid [16]byte
	if d.pos+16 > len(d.data) {
		return uuid, fmt.Errorf("read UUID: %w", io.ErrUnexpectedEOF)
	}
	copy(uuid[:], d.data[d.pos:d.pos+16])
	d.pos += 16
	return uuid, nil
}

// ReadFixedOpaque reads fixed-length opaque data with padding.
func (d *XDRDecoder) ReadFixedOpaque(size int) ([]byte, error) {
	if d.pos+size > len(d.data) {
		return nil, fmt.Errorf("read fixed opaque (size=%d): %w", size, io.ErrUnexpectedEOF)
	}
	data := make([]byte, size)
	copy(data, d.data[d.pos:d.pos+size])
	d.pos += size
	// Skip padding
	pad := (4 - size%4) % 4
	d.pos += pad
	return data, nil
}
