package rpc

import (
	"net"
	"testing"
)

func TestHeaderEncodeDecode(t *testing.T) {
	tests := []struct {
		name   string
		header Header
	}{
		{
			name: "call",
			header: Header{
				Program:   RemoteProgram,
				Version:   RemoteProtocolVersion,
				Procedure: 1,
				Type:      MessageTypeCall,
				Serial:    42,
				Status:    StatusOK,
			},
		},
		{
			name: "reply",
			header: Header{
				Program:   RemoteProgram,
				Version:   RemoteProtocolVersion,
				Procedure: 350,
				Type:      MessageTypeReply,
				Serial:    100,
				Status:    StatusError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.header.Encode()
			if len(encoded) != HeaderSize {
				t.Fatalf("header size = %d, want %d", len(encoded), HeaderSize)
			}

			decoded, err := DecodeHeader(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Program != tt.header.Program {
				t.Errorf("program = %x, want %x", decoded.Program, tt.header.Program)
			}
			if decoded.Version != tt.header.Version {
				t.Errorf("version = %d, want %d", decoded.Version, tt.header.Version)
			}
			if decoded.Procedure != tt.header.Procedure {
				t.Errorf("procedure = %d, want %d", decoded.Procedure, tt.header.Procedure)
			}
			if decoded.Type != tt.header.Type {
				t.Errorf("type = %d, want %d", decoded.Type, tt.header.Type)
			}
			if decoded.Serial != tt.header.Serial {
				t.Errorf("serial = %d, want %d", decoded.Serial, tt.header.Serial)
			}
			if decoded.Status != tt.header.Status {
				t.Errorf("status = %d, want %d", decoded.Status, tt.header.Status)
			}
		})
	}
}

func TestMessageReadWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	msg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: 1,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: []byte{0, 0, 0, 42}, // XDR uint32 = 42
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteMessage(client, msg)
	}()

	got, err := ReadMessage(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if got.Header.Procedure != msg.Header.Procedure {
		t.Errorf("procedure = %d, want %d", got.Header.Procedure, msg.Header.Procedure)
	}
	if got.Header.Serial != msg.Header.Serial {
		t.Errorf("serial = %d, want %d", got.Header.Serial, msg.Header.Serial)
	}
	if len(got.Body) != 4 {
		t.Errorf("body length = %d, want 4", len(got.Body))
	}
}

func TestNewReply(t *testing.T) {
	reqHeader := &Header{
		Program:   RemoteProgram,
		Version:   RemoteProtocolVersion,
		Procedure: 42,
		Type:      MessageTypeCall,
		Serial:    7,
		Status:    StatusOK,
	}

	reply := NewReply(reqHeader, StatusOK, nil)
	if reply.Header.Procedure != 42 {
		t.Errorf("procedure = %d, want 42", reply.Header.Procedure)
	}
	if reply.Header.Serial != 7 {
		t.Errorf("serial = %d, want 7", reply.Header.Serial)
	}
	if reply.Header.Type != MessageTypeReply {
		t.Errorf("type = %d, want %d", reply.Header.Type, MessageTypeReply)
	}
}
