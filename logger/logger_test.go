package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/sirupsen/logrus"
)

func BenchmarkLogStack(b *testing.B) {
	out := &bytes.Buffer{}

	log := New("test-service")
	SetOutput(out)
	err := errors.Wrap(errors.New("error message"), "from error")
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		log.WithStack(err).Error(err)
		out.Reset()
	}
}

func TestMaskSensitiveFields(t *testing.T) {
	tests := []struct {
		name         string
		maskedFields []string
		params       Params
		checkFields  map[string]string // field -> expected value
	}{
		{
			name:         "mask password field",
			maskedFields: []string{"password"},
			params: Params{
				"username": "john",
				"password": "secret123",
			},
			checkFields: map[string]string{
				"password": "[REDACTED]",
				"username": "john",
			},
		},
		{
			name:         "mask multiple fields",
			maskedFields: []string{"password", "token", "secret"},
			params: Params{
				"username": "john",
				"password": "secret123",
				"token":    "abc123",
				"secret":   "mysecret",
			},
			checkFields: map[string]string{
				"password": "[REDACTED]",
				"token":    "[REDACTED]",
				"secret":   "[REDACTED]",
				"username": "john",
			},
		},
		{
			name:         "mask nested fields in map",
			maskedFields: []string{"password"},
			params: Params{
				"user": map[string]any{
					"username": "john",
					"password": "secret123",
				},
			},
			checkFields: map[string]string{
				"user.password": "[REDACTED]",
				"user.username": "john",
			},
		},
		{
			name:         "mask deeply nested fields",
			maskedFields: []string{"password", "api_key"},
			params: Params{
				"data": map[string]any{
					"user": map[string]any{
						"username": "john",
						"password": "secret123",
						"profile": map[string]any{
							"email":   "john@example.com",
							"api_key": "sk-12345",
						},
					},
				},
			},
			checkFields: map[string]string{
				"data.user.password":        "[REDACTED]",
				"data.user.profile.api_key": "[REDACTED]",
				"data.user.username":        "john",
				"data.user.profile.email":   "john@example.com",
			},
		},
		{
			name:         "mask fields in array of maps",
			maskedFields: []string{"password"},
			params: Params{
				"users": []map[string]any{
					{
						"username": "john",
						"password": "secret123",
					},
					{
						"username": "jane",
						"password": "secret456",
					},
				},
			},
			checkFields: map[string]string{
				"users[0].password": "[REDACTED]",
				"users[0].username": "john",
				"users[1].password": "[REDACTED]",
				"users[1].username": "jane",
			},
		},
		{
			name:         "case insensitive masking",
			maskedFields: []string{"password"},
			params: Params{
				"Password": "secret123",
				"PASSWORD": "secret456",
				"PaSsWoRd": "secret789",
			},
			checkFields: map[string]string{
				"Password": "[REDACTED]",
				"PASSWORD": "[REDACTED]",
				"PaSsWoRd": "[REDACTED]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			log := New("test-service", Options{RedactedFields: tt.maskedFields})
			SetOutput(out)
			SetFormatter(&logrus.JSONFormatter{})

			log.WithParams(tt.params).Info("test message")

			// Parse the output JSON
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("Failed to parse log output: %v", err)
			}

			// Check each expected field
			for field, expectedValue := range tt.checkFields {
				actualValue := getNestedField(result, field)
				if actualValue != expectedValue {
					t.Errorf("Field %s: expected %q, got %q", field, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestMaskWithParam(t *testing.T) {
	out := &bytes.Buffer{}
	log := New("test-service", Options{RedactedFields: []string{"password", "token"}})
	SetOutput(out)
	SetFormatter(&logrus.JSONFormatter{})

	log.WithParam("username", "john").
		WithParam("password", "secret123").
		WithParam("token", "abc123").
		Info("test message")

	// Parse the output JSON
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	if result["password"] != "[REDACTED]" {
		t.Errorf("Expected password to be [REDACTED], got %v", result["password"])
	}

	if result["token"] != "[REDACTED]" {
		t.Errorf("Expected token to be [REDACTED], got %v", result["token"])
	}

	if result["username"] != "john" {
		t.Errorf("Expected username to be john, got %v", result["username"])
	}
}

// Helper function to get nested field values from parsed JSON
func getNestedField(data map[string]any, path string) string {
	parts := strings.Split(path, ".")
	current := toMapStringAny(data)

	for i, part := range parts {
		if strings.Contains(part, "[") {
			current = handleArrayAccess(current, part)
			if current == nil {
				return ""
			}
			continue
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			if val, ok := current[part].(string); ok {
				return val
			}
			return ""
		}

		// Navigate to next level
		current = toMapStringAny(current[part])
		if current == nil {
			return ""
		}
	}

	return ""
}

func toMapStringAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	if m, ok := value.(map[string]interface{}); ok {
		result := make(map[string]any)
		for k, v := range m {
			result[k] = v
		}
		return result
	}
	return nil
}

func handleArrayAccess(current map[string]any, part string) map[string]any {
	arrayName := part[:strings.Index(part, "[")]
	indexStr := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

	// Get array and convert index
	arr := getArrayFromMap(current, arrayName)
	if arr == nil {
		return nil
	}

	index := simpleAtoi(indexStr)
	if index >= len(arr) {
		return nil
	}

	return toMapStringAny(arr[index])
}

func getArrayFromMap(m map[string]any, key string) []any {
	if arr, ok := m[key].([]any); ok {
		return arr
	}
	if arr, ok := m[key].([]interface{}); ok {
		return arr
	}
	return nil
}

func simpleAtoi(s string) int {
	if s == "0" {
		return 0
	} else if s == "1" {
		return 1
	} else if s == "2" {
		return 2
	}
	return 0
}
