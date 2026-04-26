package vault

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SplitFrontmatter splits Markdown content into its YAML frontmatter and body.
//
// Frontmatter is recognised when the content starts with exactly "---\n" and
// contains a subsequent line that is exactly "---".  The delimiters themselves
// are not included in the returned raw or body strings.
//
// Returns:
//
//	raw   – the YAML text between the delimiters (empty when hasFM is false)
//	body  – the content after the closing delimiter (full content when hasFM is false)
//	hasFM – true only when both delimiters were found
func SplitFrontmatter(content string) (raw, body string, hasFM bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content, false
	}

	rest := content[4:] // skip the opening "---\n"

	for i := 0; i < len(rest); {
		nl := strings.Index(rest[i:], "\n")

		var line string
		var nextI int

		if nl < 0 {
			line = rest[i:]
			nextI = len(rest)
		} else {
			line = rest[i : i+nl]
			nextI = i + nl + 1
		}

		if line == "---" {
			return rest[:i], rest[nextI:], true
		}

		i = nextI

		if nl < 0 {
			break
		}
	}

	return "", content, false
}

// ParseFrontmatter parses raw YAML text into a map[string]any.
//
// An empty or whitespace-only raw string returns an empty map with no error.
// If the YAML is structurally invalid, the returned error wraps ErrInvalidFrontmatter.
func ParseFrontmatter(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := yaml.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFrontmatter, err.Error())
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

// GetFrontmatter reads the note at path and returns its parsed frontmatter map
// and the body text (everything after the closing "---" delimiter).
//
// When the note has no frontmatter, an empty map is returned and body equals
// the full file content.  Read-only; does not hold the mutex.
func (s *Service) GetFrontmatter(ctx context.Context, path string) (fm map[string]any, body string, err error) {
	note, err := s.ReadNote(ctx, path)
	if err != nil {
		return nil, "", err
	}

	raw, body, hasFM := SplitFrontmatter(note.Content)
	if !hasFM {
		return map[string]any{}, note.Content, nil
	}

	fm, err = ParseFrontmatter(raw)
	if err != nil {
		return nil, "", err
	}

	return fm, body, nil
}

// UpdateFrontmatter reads the note at path, applies updates and removals to its
// YAML frontmatter using the yaml.v3 Node API (preserving existing key order),
// and writes the result back to disk under the service mutex.
//
// updates    – keys to set or add; values may be scalars, slices, or maps.
// removeKeys – keys to delete; silently ignored when the key does not exist.
//
// Returns ErrNotFound when the file does not exist.
// Returns ErrSymlinkEscape when the path resolves outside the vault boundary.
func (s *Service) UpdateFrontmatter(ctx context.Context, path string, updates map[string]any, removeKeys []string) error {
	_, absPath, err := s.sanitizePath("update_frontmatter", path)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(absPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return &PathError{Op: "update_frontmatter", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "update_frontmatter", Path: path, Err: statErr}
	}

	if _, err := s.resolveSymlink("update_frontmatter", path, absPath); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &PathError{Op: "update_frontmatter", Path: path, Err: err}
	}

	content := string(data)
	raw, body, hasFM := SplitFrontmatter(content)

	var mapping *yaml.Node

	if hasFM {
		var doc yaml.Node
		if unmarshalErr := yaml.Unmarshal([]byte(raw), &doc); unmarshalErr != nil {
			return fmt.Errorf("%w: %s", ErrInvalidFrontmatter, unmarshalErr.Error())
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			mapping = doc.Content[0]
		}
	}

	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}

	for k, v := range updates {
		valNode, encErr := encodeToNode(v)
		if encErr != nil {
			return fmt.Errorf("update_frontmatter: encode value for key %q: %w", k, encErr)
		}

		found := false
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == k {
				mapping.Content[i+1] = valNode
				found = true
				break
			}
		}
		if !found {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
			mapping.Content = append(mapping.Content, keyNode, valNode)
		}
	}

	for _, k := range removeKeys {
		kept := make([]*yaml.Node, 0, len(mapping.Content))
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value != k {
				kept = append(kept, mapping.Content[i], mapping.Content[i+1])
			}
		}
		mapping.Content = kept
	}

	out, marshalErr := yaml.Marshal(mapping)
	if marshalErr != nil {
		return fmt.Errorf("update_frontmatter: marshal: %w", marshalErr)
	}

	assembled := "---\n" + string(out) + "---\n" + body

	if writeErr := os.WriteFile(absPath, []byte(assembled), 0644); writeErr != nil {
		return &PathError{Op: "update_frontmatter", Path: path, Err: writeErr}
	}

	return nil
}

// encodeToNode converts any Go value to a *yaml.Node.
// yaml.Node.Encode wraps the result in a DocumentNode; this helper unwraps it.
func encodeToNode(v any) (*yaml.Node, error) {
	var doc yaml.Node
	if err := doc.Encode(v); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return &doc, nil
}
