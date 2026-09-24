package core

import (
	"testing"
)

func TestEntityID(t *testing.T) {
	tests := []struct {
		input   string
		valid   bool
		wantErr bool
	}{
		{"e_foo", true, false},
		{"e_f", true, false},
		{"e_foo_bar", true, false},
		{"e_foo123", true, false},
		{"e_", false, true},
		{"foo", false, true},
		{"e", false, true},
		{"e_foo-bar", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := EntityID(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("EntityID(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("EntityID(%q) unexpected error: %v", tt.input, err)
			}
			if !tt.wantErr && got != tt.input {
				t.Errorf("EntityID(%q) = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}

func TestRelationID(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		relType string
		valid   bool
		wantErr bool
	}{
		{"e_a", "e_b", "uses", true, false},
		{"e_foo", "e_bar", "contains", true, false},
		{"e_x", "e_y", "holds.one", true, false},
		{"a", "e_b", "uses", false, true},
		{"e_a", "b", "uses", false, true},
		{"e_a", "e_b", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.to+"_"+tt.relType, func(t *testing.T) {
			got, err := RelationID(tt.from, tt.to, tt.relType)
			if tt.wantErr && err == nil {
				t.Errorf("RelationID(%q, %q, %q) expected error, got nil", tt.from, tt.to, tt.relType)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("RelationID(%q, %q, %q) unexpected error: %v", tt.from, tt.to, tt.relType, err)
			}
			if !tt.wantErr && got == "" {
				t.Errorf("RelationID(%q, %q, %q) returned empty id", tt.from, tt.to, tt.relType)
			}
		})
	}
}

func TestNewTextValue(t *testing.T) {
	text := "Sample text"
	tv := NewTextValue(text, "authored")

	if tv.V != text {
		t.Errorf("NewTextValue text = %q, want %q", tv.V, text)
	}
	if tv.Origin != "authored" {
		t.Errorf("NewTextValue origin = %q, want authored", tv.Origin)
	}
	if tv.At == "" {
		t.Errorf("NewTextValue at = empty, want timestamp")
	}
}

func TestValidateTextValue(t *testing.T) {
	tests := []struct {
		name    string
		value   TextValue
		wantErr bool
	}{
		{
			name:    "valid",
			value:   TextValue{V: "text", Origin: "authored", At: "2024-01-01T00:00:00Z"},
			wantErr: false,
		},
		{
			name:    "missing v",
			value:   TextValue{Origin: "authored", At: "2024-01-01T00:00:00Z"},
			wantErr: true,
		},
		{
			name:    "missing origin",
			value:   TextValue{V: "text", At: "2024-01-01T00:00:00Z"},
			wantErr: true,
		},
		{
			name:    "missing at",
			value:   TextValue{V: "text", Origin: "authored"},
			wantErr: true,
		},
		{
			name:    "invalid at format",
			value:   TextValue{V: "text", Origin: "authored", At: "2024-01-01"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTextValue(tt.value)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateTextValue expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateTextValue unexpected error: %v", err)
			}
		})
	}
}

func TestCanDeleteEntity(t *testing.T) {
	tests := []struct {
		name    string
		entity  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "code entity cannot be deleted",
			entity:  map[string]interface{}{"origin": "code"},
			wantErr: true,
		},
		{
			name:    "authored entity can be deleted",
			entity:  map[string]interface{}{"origin": "authored"},
			wantErr: false,
		},
		{
			name:    "no origin defaults to allowed",
			entity:  map[string]interface{}{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanDeleteEntity(tt.entity)
			if tt.wantErr && err == nil {
				t.Errorf("CanDeleteEntity expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("CanDeleteEntity unexpected error: %v", err)
			}
		})
	}
}

func TestCanDeleteRelation(t *testing.T) {
	tests := []struct {
		name      string
		relation  map[string]interface{}
		wantErr   bool
	}{
		{
			name:      "code relation cannot be deleted",
			relation:  map[string]interface{}{"origin": "code"},
			wantErr:   true,
		},
		{
			name:      "authored relation can be deleted",
			relation:  map[string]interface{}{"origin": "authored"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanDeleteRelation(tt.relation)
			if tt.wantErr && err == nil {
				t.Errorf("CanDeleteRelation expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("CanDeleteRelation unexpected error: %v", err)
			}
		})
	}
}

func TestCanWriteView(t *testing.T) {
	tests := []struct {
		name    string
		opts    WriteViewOptions
		wantErr bool
	}{
		{
			name:    "human request allowed",
			opts:    WriteViewOptions{RequestedByHuman: true},
			wantErr: false,
		},
		{
			name:    "non-human request refused",
			opts:    WriteViewOptions{RequestedByHuman: false},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanWriteView(tt.opts)
			if tt.wantErr && err == nil {
				t.Errorf("CanWriteView expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("CanWriteView unexpected error: %v", err)
			}
		})
	}
}
