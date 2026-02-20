package protocli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

// InputFormat defines how to unmarshal file contents into a proto message.
type InputFormat interface {
	// Name returns the format identifier (e.g., "json", "yaml")
	Name() string

	// Extensions returns file extensions this format handles (e.g., [".json"] or [".yaml", ".yml"])
	Extensions() []string

	// Unmarshal parses the given data into the proto message.
	Unmarshal(data []byte, msg proto.Message) error
}

// protoJSONInputFormat unmarshals protojson-encoded data.
type protoJSONInputFormat struct{}

func (f *protoJSONInputFormat) Name() string { return "json" }

func (f *protoJSONInputFormat) Extensions() []string { return []string{".json"} }

func (f *protoJSONInputFormat) Unmarshal(data []byte, msg proto.Message) error {
	return protojson.Unmarshal(data, msg)
}

// yamlInputFormat unmarshals YAML data by converting to JSON first,
// then using protojson.Unmarshal.
type yamlInputFormat struct{}

func (f *yamlInputFormat) Name() string { return "yaml" }

func (f *yamlInputFormat) Extensions() []string { return []string{".yaml", ".yml"} }

func (f *yamlInputFormat) Unmarshal(data []byte, msg proto.Message) error {
	// Parse YAML into a generic structure
	var intermediate any
	if err := yaml.Unmarshal(data, &intermediate); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Convert to JSON
	jsonData, err := json.Marshal(intermediate)
	if err != nil {
		return fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	// Use protojson to unmarshal into the proto message
	return protojson.Unmarshal(jsonData, msg)
}

// ProtoJSONInput returns an InputFormat that reads protojson-encoded files.
func ProtoJSONInput() InputFormat {
	return &protoJSONInputFormat{}
}

// YAMLInput returns an InputFormat that reads YAML files by converting to JSON
// then using protojson.Unmarshal.
func YAMLInput() InputFormat {
	return &yamlInputFormat{}
}

// DefaultInputFormats returns the default set of input formats: protojson and YAML.
func DefaultInputFormats() []InputFormat {
	return []InputFormat{ProtoJSONInput(), YAMLInput()}
}

// ReadInputFile reads the file at filePath and unmarshals its contents into msg.
//
// Format selection waterfall:
//  1. If formatName is non-empty, find the format by name. Error if not found.
//  2. Match filepath.Ext(filePath) against each format's Extensions(). Use first match.
//  3. Try all formats in order, use the first one that succeeds. If all fail, return a combined error.
func ReadInputFile(filePath, formatName string, formats []InputFormat, msg proto.Message) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read input file %s: %w", filePath, err)
	}

	// 1. Explicit format name
	if formatName != "" {
		for _, f := range formats {
			if f.Name() == formatName {
				if err := f.Unmarshal(data, msg); err != nil {
					return fmt.Errorf("failed to unmarshal input file %s as %s: %w", filePath, formatName, err)
				}
				return nil
			}
		}
		var available []string
		for _, f := range formats {
			available = append(available, f.Name())
		}
		return fmt.Errorf("unknown input format %q (available: %v)", formatName, available)
	}

	// 2. Match by file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, f := range formats {
		for _, fExt := range f.Extensions() {
			if ext == fExt {
				if err := f.Unmarshal(data, msg); err != nil {
					return fmt.Errorf("failed to unmarshal input file %s as %s: %w", filePath, f.Name(), err)
				}
				return nil
			}
		}
	}

	// 3. Try all formats
	var errs []string
	for _, f := range formats {
		// Reset the message before each attempt
		proto.Reset(msg)
		if err := f.Unmarshal(data, msg); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Name(), err))
			continue
		}
		return nil
	}

	return fmt.Errorf("failed to unmarshal input file %s: no format matched (tried: %s)", filePath, strings.Join(errs, "; "))
}
