package cache

import (
	"testing"
)

type testData struct {
	ID    int64  `json:"id" msgpack:"id"`
	Name  string `json:"name" msgpack:"name"`
	Email string `json:"email" msgpack:"email"`
}

func TestSerializeJSON(t *testing.T) {
	s := SerializeJSON

	data := testData{
		ID:    123,
		Name:  "Test User",
		Email: "test@example.com",
	}

	encoded, err := s.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded testData
	err = s.Unmarshal(encoded, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.ID != data.ID || decoded.Name != data.Name || decoded.Email != data.Email {
		t.Errorf("JSON roundtrip failed: got %+v, want %+v", decoded, data)
	}
}

func TestSerializeMessagePack(t *testing.T) {
	s := SerializeMessagePack

	data := testData{
		ID:    123,
		Name:  "Test User",
		Email: "test@example.com",
	}

	encoded, err := s.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded testData
	err = s.Unmarshal(encoded, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.ID != data.ID || decoded.Name != data.Name || decoded.Email != data.Email {
		t.Errorf("MsgPack roundtrip failed: got %+v, want %+v", decoded, data)
	}
}

func TestSerializeSizeComparison(t *testing.T) {
	data := testData{
		ID:    12345678901234567,
		Name:  "A relatively long user name for testing purposes",
		Email: "verylongemail@subdomain.example.com",
	}

	jsonBytes, _ := SerializeJSON.Marshal(data)
	msgpackBytes, _ := SerializeMessagePack.Marshal(data)

	t.Logf("JSON size: %d bytes", len(jsonBytes))
	t.Logf("MsgPack size: %d bytes", len(msgpackBytes))

	if len(msgpackBytes) >= len(jsonBytes) {
		t.Logf("Warning: MsgPack not smaller for this payload (JSON: %d, MsgPack: %d)", len(jsonBytes), len(msgpackBytes))
	}
}

func TestSerializeValid(t *testing.T) {
	tests := []struct {
		s    Serialize
		want bool
	}{
		{SerializeJSON, true},
		{SerializeMessagePack, true},
		{Serialize(99), false},
	}

	for _, tt := range tests {
		if got := tt.s.Valid(); got != tt.want {
			t.Errorf("Serialize(%d).Valid() = %v, want %v", tt.s, got, tt.want)
		}
	}
}
