package rpc

import (
	"testing"
)

func TestXDRInt32RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  int32
	}{
		{"zero", 0},
		{"positive", 42},
		{"negative", -1},
		{"max", 2147483647},
		{"min", -2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewXDREncoder()
			enc.WriteInt32(tt.val)
			dec := NewXDRDecoder(enc.Bytes())
			got, err := dec.ReadInt32()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.val {
				t.Errorf("got %d, want %d", got, tt.val)
			}
		})
	}
}

func TestXDRUint64RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  uint64
	}{
		{"zero", 0},
		{"large", 1 << 40},
		{"max", ^uint64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewXDREncoder()
			enc.WriteUint64(tt.val)
			dec := NewXDRDecoder(enc.Bytes())
			got, err := dec.ReadUint64()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.val {
				t.Errorf("got %d, want %d", got, tt.val)
			}
		})
	}
}

func TestXDRStringRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{"empty", ""},
		{"short", "hi"},
		{"aligned", "test"},
		{"unaligned", "hello"},
		{"long", "qemu:///system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewXDREncoder()
			enc.WriteString(tt.val)

			// Verify 4-byte alignment
			if len(enc.Bytes())%4 != 0 {
				t.Errorf("encoded length %d not 4-byte aligned", len(enc.Bytes()))
			}

			dec := NewXDRDecoder(enc.Bytes())
			got, err := dec.ReadString()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.val {
				t.Errorf("got %q, want %q", got, tt.val)
			}
		})
	}
}

func TestXDROptionalString(t *testing.T) {
	tests := []struct {
		name string
		val  *string
	}{
		{"nil", nil},
		{"present", strPtr("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewXDREncoder()
			enc.WriteOptionalString(tt.val)
			dec := NewXDRDecoder(enc.Bytes())
			got, err := dec.ReadOptionalString()
			if err != nil {
				t.Fatal(err)
			}
			if tt.val == nil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
			} else {
				if got == nil {
					t.Error("expected non-nil")
				} else if *got != *tt.val {
					t.Errorf("got %q, want %q", *got, *tt.val)
				}
			}
		})
	}
}

func TestXDRUUID(t *testing.T) {
	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

	enc := NewXDREncoder()
	enc.WriteUUID(uuid)
	if len(enc.Bytes()) != 16 {
		t.Errorf("UUID encoded length = %d, want 16", len(enc.Bytes()))
	}

	dec := NewXDRDecoder(enc.Bytes())
	got, err := dec.ReadUUID()
	if err != nil {
		t.Fatal(err)
	}
	if got != uuid {
		t.Errorf("got %v, want %v", got, uuid)
	}
}

func TestXDRMultipleValues(t *testing.T) {
	enc := NewXDREncoder()
	enc.WriteUint32(1)
	enc.WriteString("hello")
	enc.WriteInt64(-42)
	enc.WriteUUID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})

	dec := NewXDRDecoder(enc.Bytes())

	v1, err := dec.ReadUint32()
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 {
		t.Errorf("v1 = %d", v1)
	}

	v2, err := dec.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if v2 != "hello" {
		t.Errorf("v2 = %q", v2)
	}

	v3, err := dec.ReadInt64()
	if err != nil {
		t.Fatal(err)
	}
	if v3 != -42 {
		t.Errorf("v3 = %d", v3)
	}

	v4, err := dec.ReadUUID()
	if err != nil {
		t.Fatal(err)
	}
	if v4[0] != 1 || v4[15] != 16 {
		t.Errorf("v4 = %v", v4)
	}
}

func strPtr(s string) *string {
	return &s
}
