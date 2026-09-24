// Package core: rule enforcement for MCP and other frontends.
//
// These functions encapsulate the rules that the server promises: id generation,
// record validation, text provenance, deletion prevention, view-write restrictions.
// Used by MCP tools, HTTP API, and CLI to ensure consistent behavior.

package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// EntityID generates a new entity id (e_...) or returns the provided one if valid.
// Returns an error if the id is invalid.
func EntityID(id string) (string, error) {
	if id == "" {
		// Generate new id: e_<hash or sequence>
		// For now, return an error - caller should provide or use a generator
		return "", fmt.Errorf("entity id required")
	}
	if !strings.HasPrefix(id, "e_") {
		return "", fmt.Errorf("entity id must start with e_: %s", id)
	}
	// Validate format: e_<alphanumeric_underscore>
	if !regexp.MustCompile(`^e_[a-zA-Z0-9_]+$`).MatchString(id) {
		return "", fmt.Errorf("invalid entity id format: %s", id)
	}
	return id, nil
}

// RelationID generates a relation id (r_<from>_<to>_<type>) or validates the provided one.
func RelationID(from, to, relType string) (string, error) {
	if !strings.HasPrefix(from, "e_") || !strings.HasPrefix(to, "e_") {
		return "", fmt.Errorf("relation endpoints must be entity ids (e_...)")
	}
	// Remove e_ prefix for id construction
	fromBase := strings.TrimPrefix(from, "e_")
	toBase := strings.TrimPrefix(to, "e_")
	id := fmt.Sprintf("r_%s_%s_%s", fromBase, toBase, relType)
	if !regexp.MustCompile(`^r_[a-zA-Z0-9_]+_[a-zA-Z0-9_]+_[a-zA-Z0-9_.]+$`).MatchString(id) {
		return "", fmt.Errorf("invalid relation id format: %s", id)
	}
	return id, nil
}

// TextValue creates a text record with origin and timestamp.
// origin must be "authored", "code", or "translated".
type TextValue struct {
	V        string `json:"v"`
	Origin   string `json:"origin"`
	At       string `json:"at"`
	From     string `json:"from,omitempty"`
	FromHash string `json:"fromHash,omitempty"`
}

// NewTextValue creates a new text value with origin: authored and current timestamp.
func NewTextValue(text string, origin string) TextValue {
	if origin == "" {
		origin = "authored"
	}
	return TextValue{
		V:      text,
		Origin: origin,
		At:     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ValidateTextValue checks that a text value has required fields.
func ValidateTextValue(v TextValue) error {
	if v.V == "" {
		return fmt.Errorf("text value must not be empty")
	}
	if v.Origin == "" {
		return fmt.Errorf("text origin required (authored, code, or translated)")
	}
	if v.At == "" {
		return fmt.Errorf("text timestamp required (at field)")
	}
	// Validate timestamp format (ISO 8601)
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).MatchString(v.At) {
		return fmt.Errorf("invalid timestamp format (expected ISO 8601 with Z): %s", v.At)
	}
	return nil
}

// CanDeleteEntity checks if an entity can be deleted.
// Entities with origin: "code" cannot be deleted; they become status: missing.
func CanDeleteEntity(entity map[string]interface{}) error {
	origin, _ := entity["origin"].(string)
	if origin == "code" {
		return fmt.Errorf("cannot delete code entity; set status: missing instead")
	}
	return nil
}

// CanDeleteRelation checks if a relation can be deleted.
// Relations with origin: "code" cannot be deleted.
func CanDeleteRelation(relation map[string]interface{}) error {
	origin, _ := relation["origin"].(string)
	if origin == "code" {
		return fmt.Errorf("cannot delete code relation; set status: missing instead")
	}
	return nil
}

// WriteViewOptions controls when a view can be written.
type WriteViewOptions struct {
	RequestedByHuman bool // Only write views if requested by a human
}

// CanWriteView checks if a view file can be written.
func CanWriteView(opts WriteViewOptions) error {
	if !opts.RequestedByHuman {
		return fmt.Errorf("view files can only be written when requested by human (CONTRACT §8.2)")
	}
	return nil
}

// SaveTextJSON saves a text catalog to a file, ensuring proper structure.
func SaveTextJSON(path string, texts map[string]map[string]interface{}) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Marshal with 2-space indentation (CONTRACT requirement)
	data, err := json.MarshalIndent(texts, "", "  ")
	if err != nil {
		return err
	}

	// Add final newline (CONTRACT §3.1)
	data = append(data, '\n')

	return os.WriteFile(path, data, 0644)
}

// SaveEntitiesJSON saves entities to a file with proper structure.
func SaveEntitiesJSON(path string, entities map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entities, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// SaveRelationsJSON saves relations to a file with proper structure.
func SaveRelationsJSON(path string, relations map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(relations, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
