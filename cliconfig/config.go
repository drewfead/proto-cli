package cliconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"
)

var (
	// ErrInvalidKey is returned when a config key doesn't exist in the schema
	ErrInvalidKey = errors.New("invalid config key")

	// ErrListNotSupported is returned when attempting to set a list/array field
	ErrListNotSupported = errors.New("setting list fields not supported, edit config file manually")
)

// Manager handles configuration file operations
type Manager struct {
	// configMsg is the proto message type that defines the config schema
	configMsg proto.Message

	// globalPath is the path to the global config file (~/.config/appname/config.yaml)
	globalPath string

	// localPath is the path to the local config file (./.appname/config.yaml)
	localPath string

	// appName is the application name used for default paths
	appName string

	// serviceName is the service name for scoped config (e.g., "userservice")
	serviceName string
}

// NewManager creates a new config manager for the given proto message type
func NewManager(configMsg proto.Message, appName string) *Manager {
	homeDir, _ := os.UserHomeDir()
	globalPath := filepath.Join(homeDir, ".config", appName, "config.yaml")
	localPath := filepath.Join(".", "."+appName, "config.yaml")

	return &Manager{
		configMsg:   configMsg,
		globalPath:  globalPath,
		localPath:   localPath,
		appName:     appName,
		serviceName: "", // Will be set when needed
	}
}

// SetServiceName sets the service name for service-scoped config
func (m *Manager) SetServiceName(serviceName string) {
	m.serviceName = serviceName
}

// SetGlobalPath sets a custom global config path
func (m *Manager) SetGlobalPath(path string) {
	m.globalPath = path
}

// SetLocalPath sets a custom local config path
func (m *Manager) SetLocalPath(path string) {
	m.localPath = path
}

// GlobalPath returns the global config file path
func (m *Manager) GlobalPath() string {
	return m.globalPath
}

// LocalPath returns the local config file path
func (m *Manager) LocalPath() string {
	return m.localPath
}

// ReadConfig reads and merges config from global and local files
// Returns the merged config, with local taking precedence over global
func (m *Manager) ReadConfig() (proto.Message, error) {
	result := proto.Clone(m.configMsg)

	// Read global config if it exists
	if _, err := os.Stat(m.globalPath); err == nil {
		if err := m.readConfigFile(m.globalPath, result); err != nil {
			return nil, fmt.Errorf("failed to read global config: %w", err)
		}
	}

	// Read local config if it exists (overrides global)
	if _, err := os.Stat(m.localPath); err == nil {
		if err := m.readConfigFile(m.localPath, result); err != nil {
			return nil, fmt.Errorf("failed to read local config: %w", err)
		}
	}

	return result, nil
}

// WriteConfig writes config to the specified path
func (m *Manager) WriteConfig(path string, msg proto.Message) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read existing file to preserve other services' config
	var existingData map[string]any
	if fileData, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(fileData, &existingData)
	}
	if existingData == nil {
		existingData = make(map[string]any)
	}

	// Convert proto to JSON
	marshaler := protojson.MarshalOptions{
		Indent:          "  ",
		EmitUnpopulated: false,
		UseEnumNumbers:  false,
	}
	jsonData, err := marshaler.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal proto: %w", err)
	}

	// Convert JSON to map
	// Note: protojson serializes int64/uint64 as strings to avoid precision loss in JavaScript
	// We need to convert these back to numbers for YAML
	var serviceData map[string]any
	if err := json.Unmarshal(jsonData, &serviceData); err != nil {
		return err
	}

	// Convert stringified int64/uint64 back to numbers using proto schema
	m.convertInt64StringsToNumbers(serviceData, msg.ProtoReflect())

	// Wrap in services structure if serviceName is set
	var finalData any
	if m.serviceName != "" {
		// Ensure services map exists
		services, ok := existingData["services"].(map[string]any)
		if !ok {
			services = make(map[string]any)
		}
		// Set this service's config
		services[m.serviceName] = serviceData
		existingData["services"] = services
		finalData = existingData
	} else {
		// Flat config (backward compatibility)
		finalData = serviceData
	}

	yamlData, err := yaml.Marshal(finalData)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(path, yamlData, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetValue retrieves a config value by dot-notation key (e.g., "server.host")
// Returns the value and the source (file path or "default")
func (m *Manager) GetValue(key string) (string, string, error) {
	// Validate key exists in schema
	if err := m.validateKey(key); err != nil {
		return "", "", err
	}

	// Check default value from schema
	defaultVal := m.getDefaultValue()

	// Read global config
	var globalVal string
	if _, err := os.Stat(m.globalPath); err == nil {
		globalMsg := proto.Clone(m.configMsg)
		if err := m.readConfigFile(m.globalPath, globalMsg); err == nil {
			globalVal = m.getFieldValue(globalMsg, key)
		}
	}

	// Read local config
	var localVal string
	if _, err := os.Stat(m.localPath); err == nil {
		localMsg := proto.Clone(m.configMsg)
		if err := m.readConfigFile(m.localPath, localMsg); err == nil {
			localVal = m.getFieldValue(localMsg, key)
		}
	}

	// Determine value and source
	if localVal != "" {
		return localVal, m.localPath, nil
	}
	if globalVal != "" {
		return globalVal, m.globalPath, nil
	}
	if defaultVal != "" {
		return defaultVal, "default", nil
	}

	return "", "", nil
}

// SetValue sets a config value by dot-notation key
func (m *Manager) SetValue(path string, keyValues map[string]string) error {
	// Read existing config or create new
	var msg proto.Message
	if _, err := os.Stat(path); err == nil {
		msg = proto.Clone(m.configMsg)
		if err := m.readConfigFile(path, msg); err != nil {
			return err
		}
	} else {
		msg = proto.Clone(m.configMsg)
	}

	// Set each key-value pair
	for key, value := range keyValues {
		if err := m.setFieldValue(msg, key, value); err != nil {
			return err
		}
	}

	// Validate the entire config after changes
	// Note: Future enhancement - add proto validation support

	// Write back to file
	return m.WriteConfig(path, msg)
}

// ListAll returns all config values with their sources
func (m *Manager) ListAll() (map[string]ValueWithSource, error) {
	result := make(map[string]ValueWithSource)

	// Get all keys from schema
	keys := m.getAllKeys()

	for _, key := range keys {
		val, source, err := m.GetValue(key)
		if err != nil {
			return nil, err
		}
		result[key] = ValueWithSource{
			Value:  val,
			Source: source,
		}
	}

	return result, nil
}

// ValueWithSource holds a config value and its source
type ValueWithSource struct {
	Value  string
	Source string
}

// readConfigFile reads a YAML config file into a proto message
func (m *Manager) readConfigFile(path string, msg proto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Parse YAML to map
	var yamlData map[string]any
	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract service-scoped config if serviceName is set
	var configData map[string]any
	if m.serviceName != "" {
		services, ok := yamlData["services"].(map[string]any)
		if !ok {
			// No services section, config file is empty for this service
			return nil
		}
		serviceConfig, ok := services[m.serviceName].(map[string]any)
		if !ok {
			// No config for this service
			return nil
		}
		configData = serviceConfig
	} else {
		// Flat config (backward compatibility)
		configData = yamlData
	}

	// Convert map to JSON then to proto
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return err
	}

	if err := protojson.Unmarshal(jsonData, msg); err != nil {
		return fmt.Errorf("failed to unmarshal into proto: %w", err)
	}

	return nil
}

// validateKey checks if a key exists in the proto schema
func (m *Manager) validateKey(key string) error {
	parts := strings.Split(key, ".")
	msg := m.configMsg.ProtoReflect()

	for i, part := range parts {
		fields := msg.Descriptor().Fields()
		var fd protoreflect.FieldDescriptor

		// Find field by JSON name
		for j := 0; j < fields.Len(); j++ {
			f := fields.Get(j)
			if f.JSONName() == part || string(f.Name()) == part {
				fd = f
				break
			}
		}

		if fd == nil {
			return fmt.Errorf("%w: %s", ErrInvalidKey, key)
		}

		// If not at the end, descend into nested message
		if i < len(parts)-1 {
			if fd.Kind() != protoreflect.MessageKind {
				return fmt.Errorf("%w: %s (not a message)", ErrInvalidKey, key)
			}
			// Get the nested message descriptor for further validation
			msg = msg.NewField(fd).Message()
		}
	}

	return nil
}

// getDefaultValue gets the default value for a field from the schema
func (m *Manager) getDefaultValue() string {
	// Proto3 has zero values as defaults
	// This would need to be enhanced for custom defaults via annotations
	return ""
}

// getFieldValue retrieves a field value by dot-notation key
func (m *Manager) getFieldValue(msg proto.Message, key string) string {
	parts := strings.Split(key, ".")
	current := msg.ProtoReflect()

	for i, part := range parts {
		fields := current.Descriptor().Fields()
		var fd protoreflect.FieldDescriptor

		// Find field by JSON name
		for j := 0; j < fields.Len(); j++ {
			f := fields.Get(j)
			if f.JSONName() == part || string(f.Name()) == part {
				fd = f
				break
			}
		}

		if fd == nil {
			return ""
		}

		if !current.Has(fd) {
			return ""
		}

		val := current.Get(fd)

		// If at the end, return the value
		if i == len(parts)-1 {
			return fmt.Sprint(val.Interface())
		}

		// Otherwise descend into nested message
		if fd.Kind() == protoreflect.MessageKind {
			current = val.Message()
		} else {
			return ""
		}
	}

	return ""
}

// setFieldValue sets a field value by dot-notation key
func (m *Manager) setFieldValue(msg proto.Message, key, value string) error {
	if err := m.validateKey(key); err != nil {
		return err
	}

	parts := strings.Split(key, ".")
	current := msg.ProtoReflect()

	for i, part := range parts {
		fields := current.Descriptor().Fields()
		var fd protoreflect.FieldDescriptor

		// Find field by JSON name
		for j := 0; j < fields.Len(); j++ {
			f := fields.Get(j)
			if f.JSONName() == part || string(f.Name()) == part {
				fd = f
				break
			}
		}

		if fd == nil {
			return fmt.Errorf("%w: %s", ErrInvalidKey, key)
		}

		// Check for list fields
		if fd.IsList() {
			return fmt.Errorf("%w: %s", ErrListNotSupported, key)
		}

		// If at the end, set the value
		if i == len(parts)-1 {
			return m.setScalarValue(current, fd, value)
		}

		// Otherwise descend into nested message
		if fd.Kind() == protoreflect.MessageKind {
			if !current.Has(fd) {
				// Create nested message if it doesn't exist
				current.Set(fd, protoreflect.ValueOfMessage(current.NewField(fd).Message()))
			}
			current = current.Mutable(fd).Message()
		} else {
			return fmt.Errorf("%w: %s (not a message)", ErrInvalidKey, key)
		}
	}

	return nil
}

// setScalarValue sets a scalar field value from a string, parsing according to the proto field type
func (m *Manager) setScalarValue(msg protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {
	// Parse and validate the value according to the field type
	protoValue, err := m.parseValueForField(fd, value)
	if err != nil {
		return fmt.Errorf("invalid value for field %s (%s): %w", fd.Name(), fd.Kind(), err)
	}

	// Set the parsed value
	msg.Set(fd, protoValue)
	return nil
}

// parseValueForField parses a string value according to the proto field descriptor
func (m *Manager) parseValueForField(fd protoreflect.FieldDescriptor, value string) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected boolean (true/false), got %q", value)
		}
		return protoreflect.ValueOfBool(b), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		i, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected 32-bit integer, got %q", value)
		}
		return protoreflect.ValueOfInt32(int32(i)), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected 64-bit integer, got %q", value)
		}
		return protoreflect.ValueOfInt64(i), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		u, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected unsigned 32-bit integer, got %q", value)
		}
		return protoreflect.ValueOfUint32(uint32(u)), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		u, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected unsigned 64-bit integer, got %q", value)
		}
		return protoreflect.ValueOfUint64(u), nil

	case protoreflect.FloatKind:
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected float, got %q", value)
		}
		return protoreflect.ValueOfFloat32(float32(f)), nil

	case protoreflect.DoubleKind:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("expected double, got %q", value)
		}
		return protoreflect.ValueOfFloat64(f), nil

	case protoreflect.StringKind:
		return protoreflect.ValueOfString(value), nil

	case protoreflect.BytesKind:
		// Accept as string, will be converted to bytes
		return protoreflect.ValueOfBytes([]byte(value)), nil

	case protoreflect.EnumKind:
		// Try to find enum value by name
		enumDesc := fd.Enum()
		enumValue := enumDesc.Values().ByName(protoreflect.Name(value))
		if enumValue == nil {
			// Try by number
			if num, err := strconv.ParseInt(value, 10, 32); err == nil {
				enumValue = enumDesc.Values().ByNumber(protoreflect.EnumNumber(num))
			}
		}
		if enumValue == nil {
			// Build list of valid values for error message
			validValues := make([]string, enumDesc.Values().Len())
			for i := 0; i < enumDesc.Values().Len(); i++ {
				validValues[i] = string(enumDesc.Values().Get(i).Name())
			}
			return protoreflect.Value{}, fmt.Errorf("expected one of [%s], got %q",
				strings.Join(validValues, ", "), value)
		}
		return protoreflect.ValueOfEnum(enumValue.Number()), nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return protoreflect.Value{}, fmt.Errorf("cannot set message field directly, use nested field path")

	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field type: %s", fd.Kind())
	}
}

// convertInt64StringsToNumbers converts stringified int64/uint64 fields back to numbers
// protojson serializes int64/uint64 as strings, but YAML should have them as numbers
func (m *Manager) convertInt64StringsToNumbers(data map[string]any, msg protoreflect.Message) {
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fieldName := fd.JSONName()
		value, ok := data[fieldName]
		if !ok {
			continue
		}

		// Check if this is an int64/uint64 field
		switch fd.Kind() {
		case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			if strVal, ok := value.(string); ok {
				if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
					data[fieldName] = intVal
				}
			}
		case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			if strVal, ok := value.(string); ok {
				if uintVal, err := strconv.ParseUint(strVal, 10, 64); err == nil {
					data[fieldName] = uintVal
				}
			}
		case protoreflect.MessageKind:
			// Recurse into nested messages
			if nestedMap, ok := value.(map[string]any); ok {
				nestedMsg := msg.NewField(fd).Message()
				m.convertInt64StringsToNumbers(nestedMap, nestedMsg)
			}
		}
	}
}

// getAllKeys returns all possible keys from the schema
func (m *Manager) getAllKeys() []string {
	var keys []string
	m.collectKeys(m.configMsg.ProtoReflect(), "", &keys)
	return keys
}

// collectKeys recursively collects all keys from a proto message
func (m *Manager) collectKeys(msg protoreflect.Message, prefix string, keys *[]string) {
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fieldName := fd.JSONName()
		fullKey := fieldName
		if prefix != "" {
			fullKey = prefix + "." + fieldName
		}

		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			// Recurse into nested message
			nestedMsg := msg.NewField(fd).Message()
			m.collectKeys(nestedMsg, fullKey, keys)
		} else {
			*keys = append(*keys, fullKey)
		}
	}
}

