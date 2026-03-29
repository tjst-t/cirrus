package rpc

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	// RemoteProgram is the libvirt remote program number.
	RemoteProgram uint32 = 0x20008086
	// RemoteProtocolVersion is the protocol version.
	RemoteProtocolVersion uint32 = 1

	// HeaderSize is the XDR-encoded header size (6 * 4 = 24 bytes).
	HeaderSize = 24

	// MaxMessageSize is the maximum allowed message size.
	MaxMessageSize = 4 * 1024 * 1024 // 4 MiB
)

// MessageType represents the RPC message type.
type MessageType int32

const (
	// MessageTypeCall is a procedure call.
	MessageTypeCall MessageType = 0
	// MessageTypeReply is a reply to a call.
	MessageTypeReply MessageType = 1
	// MessageTypeMessage is an async message.
	MessageTypeMessage MessageType = 2
	// MessageTypeStream is a stream message.
	MessageTypeStream MessageType = 3
	// MessageTypeCallWithFDs is a call with file descriptors.
	MessageTypeCallWithFDs MessageType = 4
	// MessageTypeReplyWithFDs is a reply with file descriptors.
	MessageTypeReplyWithFDs MessageType = 5
)

// MessageStatus represents the RPC message status.
type MessageStatus int32

const (
	// StatusOK indicates success.
	StatusOK MessageStatus = 0
	// StatusError indicates an error.
	StatusError MessageStatus = 1
)

// Header represents a libvirt RPC message header.
type Header struct {
	Program   uint32
	Version   uint32
	Procedure int32
	Type      MessageType
	Serial    uint32
	Status    MessageStatus
}

// Message represents a complete libvirt RPC message.
type Message struct {
	Header Header
	Body   []byte
}

// EncodeHeader XDR-encodes the header.
func (h *Header) Encode() []byte {
	enc := NewXDREncoder()
	enc.WriteUint32(h.Program)
	enc.WriteUint32(h.Version)
	enc.WriteInt32(h.Procedure)
	enc.WriteInt32(int32(h.Type))
	enc.WriteUint32(h.Serial)
	enc.WriteInt32(int32(h.Status))
	return enc.Bytes()
}

// DecodeHeader parses a header from XDR bytes.
func DecodeHeader(data []byte) (*Header, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("header too short: %d bytes", len(data))
	}
	dec := NewXDRDecoder(data)

	prog, _ := dec.ReadUint32()
	vers, _ := dec.ReadUint32()
	proc, _ := dec.ReadInt32()
	typ, _ := dec.ReadInt32()
	serial, _ := dec.ReadUint32()
	status, _ := dec.ReadInt32()

	return &Header{
		Program:   prog,
		Version:   vers,
		Procedure: proc,
		Type:      MessageType(typ),
		Serial:    serial,
		Status:    MessageStatus(status),
	}, nil
}

// ReadMessage reads a framed libvirt RPC message from a connection.
func ReadMessage(conn net.Conn) (*Message, error) {
	// Read 4-byte length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, fmt.Errorf("read message length: %w", err)
	}

	totalLen := binary.BigEndian.Uint32(lenBuf)
	if totalLen < 4+HeaderSize {
		return nil, fmt.Errorf("message too short: %d bytes", totalLen)
	}
	if totalLen > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", totalLen)
	}

	// Read the rest of the message (totalLen includes the 4-byte length itself)
	payloadLen := totalLen - 4
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, fmt.Errorf("read message payload: %w", err)
	}

	header, err := DecodeHeader(payload[:HeaderSize])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	return &Message{
		Header: *header,
		Body:   payload[HeaderSize:],
	}, nil
}

// WriteMessage writes a framed libvirt RPC message to a connection.
func WriteMessage(conn net.Conn, msg *Message) error {
	headerBytes := msg.Header.Encode()
	payloadLen := len(headerBytes) + len(msg.Body)
	totalLen := 4 + payloadLen

	buf := make([]byte, totalLen)
	binary.BigEndian.PutUint32(buf, uint32(totalLen))
	copy(buf[4:], headerBytes)
	copy(buf[4+len(headerBytes):], msg.Body)

	_, err := conn.Write(buf)
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// NewReply creates a reply message for a given request header.
func NewReply(reqHeader *Header, status MessageStatus, body []byte) *Message {
	return &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: reqHeader.Procedure,
			Type:      MessageTypeReply,
			Serial:    reqHeader.Serial,
			Status:    status,
		},
		Body: body,
	}
}
