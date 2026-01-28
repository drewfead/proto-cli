package protocli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/drewfead/proto-cli/internal/clipb"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"
)

// ConfigMode determines which config sources are used.
type ConfigMode int

const (
	// SingleCommandMode uses files + env + flags (all sources).
	SingleCommandMode ConfigMode = iota
	// DaemonMode uses files + env only (no CLI flag overrides).
	DaemonMode
)

// ConfigDebugInfo tracks config loading for debugging.
type ConfigDebugInfo struct {
	PathsChecked   []string          // All paths that were checked
	FilesLoaded    []string          // Paths that were successfully loaded
	FilesFailed    map[string]string // Paths that failed with error message
	EnvVarsApplied map[string]string // Env vars that were applied (name -> value)
	FlagsApplied   map[string]string // CLI flags that were applied (name -> value)
	FinalConfig    any               // Final merged config (for display)
}

// ConfigLoader loads configuration with precedence: CLI flags > env vars > files.
type ConfigLoader struct {
	configPaths   []string
	configReaders []io.Reader
	envPrefix     string
	mode          ConfigMode
	debug         bool
	debugInfo     *ConfigDebugInfo
}

// ConfigLoaderOption is a functional option for configuring a ConfigLoader.
type ConfigLoaderOption func(*ConfigLoader)

// FileConfig adds config file paths to load.
func FileConfig(paths ...string) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.configPaths = append(l.configPaths, paths...)
	}
}

// ReaderConfig adds io.Readers to load config from (for testing).
func ReaderConfig(readers ...io.Reader) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.configReaders = append(l.configReaders, readers...)
	}
}

// EnvPrefix sets the environment variable prefix for config overrides.
func EnvPrefix(prefix string) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.envPrefix = prefix
	}
}

// DebugMode enables config loading debug information.
func DebugMode(enabled bool) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.debug = enabled
		if enabled && l.debugInfo == nil {
			l.debugInfo = &ConfigDebugInfo{
				PathsChecked:   []string{},
				FilesLoaded:    []string{},
				FilesFailed:    make(map[string]string),
				EnvVarsApplied: make(map[string]string),
				FlagsApplied:   make(map[string]string),
			}
		}
	}
}

// NewConfigLoader creates a new config loader with options.
func NewConfigLoader(mode ConfigMode, opts ...ConfigLoaderOption) *ConfigLoader {
	loader := &ConfigLoader{
		mode: mode,
	}
	for _, opt := range opts {
		opt(loader)
	}
	return loader
}

// DebugInfo returns the config debug information (only populated if debug mode is enabled).
func (l *ConfigLoader) DebugInfo() *ConfigDebugInfo {
	return l.debugInfo
}

// DefaultConfigPaths returns default paths for config files.
func DefaultConfigPaths(rootCommandName string) []string {
	home, _ := os.UserHomeDir()

	return []string{
		fmt.Sprintf("./%s.yaml", rootCommandName),
		filepath.Join(home, ".config", rootCommandName, "config.yaml"),
	}
}

// LoadServiceConfig loads config for a specific service.
//
// serviceName: lowercase service name (e.g., "userservice")
// target: pointer to config message instance
func (l *ConfigLoader) LoadServiceConfig(
	cmd *cli.Command,
	serviceName string,
	target proto.Message,
) error {
	// 1. Load and deep merge from all config files
	if err := l.loadFromFiles(serviceName, target); err != nil {
		return fmt.Errorf("failed to load config from files: %w", err)
	}

	// 2. Override with environment variables
	if err := l.applyEnvVars(target); err != nil {
		return fmt.Errorf("failed to apply environment variables: %w", err)
	}

	// 3. Override with CLI flags (only if mode == SingleCommandMode)
	if l.mode == SingleCommandMode && cmd != nil {
		if err := l.applyFlags(cmd, target); err != nil {
			return fmt.Errorf("failed to apply CLI flags: %w", err)
		}
	}

	// 4. Save final config for debugging
	if l.debug {
		l.debugInfo.FinalConfig = target
	}

	return nil
}

// loadFromFiles loads and deep merges config from multiple YAML files and readers.
func (l *ConfigLoader) loadFromFiles(serviceName string, target proto.Message) error {
	// Load from file paths
	for _, path := range l.configPaths {
		if l.debug {
			l.debugInfo.PathsChecked = append(l.debugInfo.PathsChecked, path)
		}

		// Skip if file doesn't exist (silent ignore for default paths)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if l.debug {
				l.debugInfo.FilesFailed[path] = "file does not exist"
			}
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if l.debug {
				l.debugInfo.FilesFailed[path] = err.Error()
			}
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		if err := l.loadYAMLServiceFromData(data, serviceName, target); err != nil {
			if l.debug {
				l.debugInfo.FilesFailed[path] = err.Error()
			}
			return fmt.Errorf("failed to load %s: %w", path, err)
		}

		if l.debug {
			l.debugInfo.FilesLoaded = append(l.debugInfo.FilesLoaded, path)
		}
	}

	// Load from readers (for testing)
	for i, reader := range l.configReaders {
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read config reader %d: %w", i, err)
		}

		if err := l.loadYAMLServiceFromData(data, serviceName, target); err != nil {
			return fmt.Errorf("failed to load config reader %d: %w", i, err)
		}
	}

	return nil
}

// loadYAMLServiceFromData loads YAML from bytes and extracts service section.
func (l *ConfigLoader) loadYAMLServiceFromData(data []byte, serviceName string, target proto.Message) error {
	// Parse YAML into map
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Extract services section
	services, ok := root["services"].(map[string]any)
	if !ok {
		// No services section, skip this file
		return nil
	}

	// Extract service-specific section
	serviceConfig, ok := services[serviceName].(map[string]any)
	if !ok {
		// No config for this service, skip
		return nil
	}

	// Merge config into target
	return l.mergeConfig(serviceConfig, target)
}

// mergeConfig deep merges YAML data into proto message using reflection.
func (l *ConfigLoader) mergeConfig(data map[string]any, target proto.Message) error {
	return l.mergeConfigWithPath(data, target, "")
}

var ErrUnknownField = errors.New("unknown field")

func (l *ConfigLoader) mergeConfigWithPath(data map[string]any, target proto.Message, fieldPath string) error {
	msg := target.ProtoReflect()
	fields := msg.Descriptor().Fields()

	for key, value := range data {
		// Convert kebab-case to snake_case for field lookup
		fieldName := strings.ReplaceAll(key, "-", "_")

		// Build nested field path for error messages
		currentPath := key
		if fieldPath != "" {
			currentPath = fieldPath + "." + key
		}

		// Find field by name
		field := fields.ByName(protoreflect.Name(fieldName))
		if field == nil {
			// Also try JSON name
			field = fields.ByJSONName(key)
		}
		if field == nil {
			return fmt.Errorf("%w: %s", ErrUnknownField, currentPath)
		}

		// Set field value
		if err := l.setFieldValueWithPath(msg, field, value, currentPath); err != nil {
			return err
		}
	}

	return nil
}

var (
	ErrUnexpectedFieldValueType = errors.New("unexpected field value type")
	ErrOverflow                 = errors.New("overflow")
)

// setFieldValueWithPath sets a proto field value based on type.
//
//nolint:gocyclo // Complexity comes from handling all proto kinds exhaustively
func (l *ConfigLoader) setFieldValueWithPath(msg protoreflect.Message, field protoreflect.FieldDescriptor, value any, fieldPath string) error {
	// Handle repeated fields (lists)
	if field.IsList() {
		if err := l.setListField(msg, field, value); err != nil {
			return fmt.Errorf("field %s: %w", fieldPath, err)
		}
		return nil
	}

	// Handle map fields
	if field.IsMap() {
		if err := l.setMapField(msg, field, value); err != nil {
			return fmt.Errorf("field %s: %w", fieldPath, err)
		}
		return nil
	}

	switch field.Kind() {
	case protoreflect.MessageKind:
		// Handle nested message types with field path
		return l.setMessageFieldWithPath(msg, field, value, fieldPath)

	case protoreflect.StringKind:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w - field %s: expected string, got %T", ErrUnexpectedFieldValueType, fieldPath, value)
		}
		msg.Set(field, protoreflect.ValueOfString(str))

	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		var intVal int64
		switch v := value.(type) {
		case int:
			intVal = int64(v)
		case int32:
			intVal = int64(v)
		case int64:
			intVal = v
		case float64:
			intVal = int64(v)
		default:
			return fmt.Errorf("%w - field %s: expected int, got %T", ErrUnexpectedFieldValueType, fieldPath, value)
		}
		msg.Set(field, protoreflect.ValueOfInt64(intVal))

	case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		var uintVal uint64
		switch v := value.(type) {
		case int:
			if v < 0 {
				return fmt.Errorf("%w: cannot convert negative int %d to uint64", ErrOverflow, v)
			}
			uintVal = uint64(v)
		case uint:
			uintVal = uint64(v)
		case uint32:
			uintVal = uint64(v)
		case uint64:
			uintVal = v
		case float64:
			uintVal = uint64(v)
		default:
			return fmt.Errorf("%w: expected uint, got %T", ErrUnexpectedFieldValueType, value)
		}
		msg.Set(field, protoreflect.ValueOfUint64(uintVal))

	case protoreflect.BoolKind:
		boolVal, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%w: expected bool, got %T", ErrUnexpectedFieldValueType, value)
		}
		msg.Set(field, protoreflect.ValueOfBool(boolVal))

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		var floatVal float64
		switch v := value.(type) {
		case float32:
			floatVal = float64(v)
		case float64:
			floatVal = v
		case int:
			floatVal = float64(v)
		default:
			return fmt.Errorf("%w: expected float, got %T", ErrUnexpectedFieldValueType, value)
		}
		msg.Set(field, protoreflect.ValueOfFloat64(floatVal))

	case protoreflect.BytesKind:
		var bytesVal []byte
		switch v := value.(type) {
		case string:
			bytesVal = []byte(v)
		case []byte:
			bytesVal = v
		default:
			return fmt.Errorf("%w: expected string or []byte for bytes field, got %T", ErrUnexpectedFieldValueType, value)
		}
		msg.Set(field, protoreflect.ValueOfBytes(bytesVal))

	case protoreflect.EnumKind:
		return l.setEnumField(msg, field, value)

	case protoreflect.GroupKind:
		return fmt.Errorf("%w: GroupKind is deprecated and not supported", ErrUnexpectedFieldValueType)

	default:
		return fmt.Errorf("%w: unsupported field type: %s", ErrUnexpectedFieldValueType, field.Kind())
	}

	return nil
}

func (l *ConfigLoader) setMessageFieldWithPath(msg protoreflect.Message, field protoreflect.FieldDescriptor, value any, fieldPath string) error {
	// Value should be a map for nested messages
	nestedMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: field %s: expected map for message field, got %T", ErrUnexpectedFieldValueType, fieldPath, value)
	}

	// Check if this field belongs to a oneof
	if oneof := field.ContainingOneof(); oneof != nil {
		return l.setOneofField(msg, field, oneof, nestedMap)
	}

	// Create new message instance
	nestedMsg := msg.NewField(field).Message()

	// Recursively merge config into nested message with path
	if err := l.mergeConfigWithPath(nestedMap, nestedMsg.Interface(), fieldPath); err != nil {
		return err
	}

	// Set the nested message on the parent
	msg.Set(field, protoreflect.ValueOfMessage(nestedMsg))

	return nil
}

// setOneofField handles oneof (union) types.
func (l *ConfigLoader) setOneofField(msg protoreflect.Message, field protoreflect.FieldDescriptor, _ protoreflect.OneofDescriptor, value map[string]any) error {
	// Clear any currently set field in this oneof
	msg.Clear(field)

	// Create new message instance for the oneof variant
	nestedMsg := msg.NewField(field).Message()

	// Recursively merge config into nested message
	if err := l.mergeConfig(value, nestedMsg.Interface()); err != nil {
		return fmt.Errorf("%w: failed to merge oneof field: %w", ErrUnexpectedFieldValueType, err)
	}

	// Set the oneof field
	msg.Set(field, protoreflect.ValueOfMessage(nestedMsg))

	return nil
}

// setListField handles repeated fields.
func (l *ConfigLoader) setListField(msg protoreflect.Message, field protoreflect.FieldDescriptor, value any) error {
	// Value should be a slice
	slice, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%w: expected slice for list field, got %T", ErrUnexpectedFieldValueType, value)
	}

	list := msg.Mutable(field).List()

	// Clear existing values
	for list.Len() > 0 {
		list.Truncate(0)
	}

	// Append each element
	for i, elem := range slice {
		if err := l.appendListElement(list, field, elem); err != nil {
			return fmt.Errorf("%w: failed to append element %d: %w", ErrUnexpectedFieldValueType, i, err)
		}
	}

	return nil
}

// appendListElement appends a single element to a list field.
//
//nolint:gocyclo // Complexity comes from handling all proto kinds exhaustively
func (l *ConfigLoader) appendListElement(list protoreflect.List, field protoreflect.FieldDescriptor, value any) error {
	switch field.Kind() {
	case protoreflect.MessageKind:
		// Nested message in list
		nestedMap, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: expected map for message element, got %T", ErrUnexpectedFieldValueType, value)
		}

		// Create new message
		elemMsg := list.NewElement().Message()
		if err := l.mergeConfig(nestedMap, elemMsg.Interface()); err != nil {
			return err
		}
		list.Append(protoreflect.ValueOfMessage(elemMsg))

	case protoreflect.StringKind:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: expected string, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfString(str))

	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		var intVal int64
		switch v := value.(type) {
		case int:
			intVal = int64(v)
		case int64:
			intVal = v
		case float64:
			intVal = int64(v)
		default:
			return fmt.Errorf("%w: expected int, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfInt64(intVal))

	case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		var uintVal uint64
		switch v := value.(type) {
		case int:
			if v < 0 {
				return fmt.Errorf("%w: cannot convert negative int %d to uint64", ErrOverflow, v)
			}
			uintVal = uint64(v)
		case uint:
			uintVal = uint64(v)
		case uint32:
			uintVal = uint64(v)
		case uint64:
			uintVal = v
		case float64:
			uintVal = uint64(v)
		default:
			return fmt.Errorf("%w: expected uint, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfUint64(uintVal))

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		var floatVal float64
		switch v := value.(type) {
		case float32:
			floatVal = float64(v)
		case float64:
			floatVal = v
		case int:
			floatVal = float64(v)
		default:
			return fmt.Errorf("%w: expected float, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfFloat64(floatVal))

	case protoreflect.BytesKind:
		var bytesVal []byte
		switch v := value.(type) {
		case string:
			bytesVal = []byte(v)
		case []byte:
			bytesVal = v
		default:
			return fmt.Errorf("%w: expected string or []byte for bytes field, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfBytes(bytesVal))

	case protoreflect.EnumKind:
		// Enum values in lists
		switch v := value.(type) {
		case string:
			enumDesc := field.Enum()
			enumVal := enumDesc.Values().ByName(protoreflect.Name(v))
			if enumVal == nil {
				return fmt.Errorf("%w: unknown enum value: %s", ErrUnknownField, v)
			}
			list.Append(protoreflect.ValueOfEnum(enumVal.Number()))
		case int, int32, int64, float64:
			var num int32
			switch val := v.(type) {
			case int:
				if val < -2147483648 || val > 2147483647 {
					return fmt.Errorf("%w: int value %d overflows int32", ErrOverflow, val)
				}
				num = int32(val)
			case int32:
				num = val
			case int64:
				if val < -2147483648 || val > 2147483647 {
					return fmt.Errorf("%w: int64 value %d overflows int32", ErrOverflow, val)
				}
				num = int32(val)
			case float64:
				if val < -2147483648 || val > 2147483647 {
					return fmt.Errorf("%w: float64 value %f overflows int32", ErrOverflow, val)
				}
				num = int32(val)
			}
			list.Append(protoreflect.ValueOfEnum(protoreflect.EnumNumber(num)))
		default:
			return fmt.Errorf("%w: expected string or int for enum, got %T", ErrUnexpectedFieldValueType, value)
		}

	case protoreflect.BoolKind:
		boolVal, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%w: expected bool, got %T", ErrUnexpectedFieldValueType, value)
		}
		list.Append(protoreflect.ValueOfBool(boolVal))

	case protoreflect.GroupKind:
		return fmt.Errorf("%w: GroupKind is deprecated and not supported", ErrUnexpectedFieldValueType)

	default:
		return fmt.Errorf("%w: unsupported list element type: %s", ErrUnexpectedFieldValueType, field.Kind())
	}

	return nil
}

// setMapField handles map fields.
//
//nolint:gocyclo // Complexity comes from handling all proto kinds exhaustively
func (l *ConfigLoader) setMapField(msg protoreflect.Message, field protoreflect.FieldDescriptor, value any) error {
	// Value should be a map
	yamlMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: expected map for map field, got %T", ErrUnexpectedFieldValueType, value)
	}

	mapValue := msg.Mutable(field).Map()

	// Clear existing entries
	mapValue.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		mapValue.Clear(k)
		return true
	})

	// Add each entry
	valueField := field.MapValue()
	for k, v := range yamlMap {
		mapKey := protoreflect.ValueOfString(k).MapKey()

		var mapVal protoreflect.Value
		switch valueField.Kind() {
		case protoreflect.MessageKind:
			// Nested message as map value
			nestedMap, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: expected map for message value, got %T", ErrUnexpectedFieldValueType, v)
			}

			elemMsg := mapValue.NewValue().Message()
			if err := l.mergeConfig(nestedMap, elemMsg.Interface()); err != nil {
				return fmt.Errorf("%w: failed to merge map value for key %s: %w", ErrUnexpectedFieldValueType, k, err)
			}
			mapVal = protoreflect.ValueOfMessage(elemMsg)

		case protoreflect.StringKind:
			str, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: expected string, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfString(str)

		case protoreflect.Int32Kind, protoreflect.Int64Kind,
			protoreflect.Sint32Kind, protoreflect.Sint64Kind,
			protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
			var intVal int64
			switch val := v.(type) {
			case int:
				intVal = int64(val)
			case int64:
				intVal = val
			case float64:
				intVal = int64(val)
			default:
				return fmt.Errorf("%w: expected int, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfInt64(intVal)

		case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
			protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
			var uintVal uint64
			switch val := v.(type) {
			case int:
				if val < 0 {
					return fmt.Errorf("%w: cannot convert negative int %d to uint64", ErrOverflow, val)
				}
				uintVal = uint64(val)
			case uint:
				uintVal = uint64(val)
			case uint32:
				uintVal = uint64(val)
			case uint64:
				uintVal = val
			case float64:
				uintVal = uint64(val)
			default:
				return fmt.Errorf("%w: expected uint, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfUint64(uintVal)

		case protoreflect.FloatKind, protoreflect.DoubleKind:
			var floatVal float64
			switch val := v.(type) {
			case float32:
				floatVal = float64(val)
			case float64:
				floatVal = val
			case int:
				floatVal = float64(val)
			default:
				return fmt.Errorf("%w: expected float, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfFloat64(floatVal)

		case protoreflect.BoolKind:
			boolVal, ok := v.(bool)
			if !ok {
				return fmt.Errorf("%w: expected bool, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfBool(boolVal)

		case protoreflect.BytesKind:
			var bytesVal []byte
			switch val := v.(type) {
			case string:
				bytesVal = []byte(val)
			case []byte:
				bytesVal = val
			default:
				return fmt.Errorf("%w: expected string or []byte for bytes field, got %T", ErrUnexpectedFieldValueType, v)
			}
			mapVal = protoreflect.ValueOfBytes(bytesVal)

		case protoreflect.EnumKind:
			enumDesc := valueField.Enum()
			switch val := v.(type) {
			case string:
				enumVal := enumDesc.Values().ByName(protoreflect.Name(val))
				if enumVal == nil {
					return fmt.Errorf("%w: unknown enum value: %s", ErrUnknownField, val)
				}
				mapVal = protoreflect.ValueOfEnum(enumVal.Number())
			case int, int32, int64, float64:
				var num int32
				switch enumInt := val.(type) {
				case int:
					if enumInt < -2147483648 || enumInt > 2147483647 {
						return fmt.Errorf("%w: int value %d overflows int32", ErrOverflow, enumInt)
					}
					num = int32(enumInt)
				case int32:
					num = enumInt
				case int64:
					if enumInt < -2147483648 || enumInt > 2147483647 {
						return fmt.Errorf("%w: int64 value %d overflows int32", ErrOverflow, enumInt)
					}
					num = int32(enumInt)
				case float64:
					if enumInt < -2147483648 || enumInt > 2147483647 {
						return fmt.Errorf("%w: float64 value %f overflows int32", ErrOverflow, enumInt)
					}
					num = int32(enumInt)
				}
				mapVal = protoreflect.ValueOfEnum(protoreflect.EnumNumber(num))
			default:
				return fmt.Errorf("%w: expected string or int for enum, got %T", ErrUnexpectedFieldValueType, v)
			}

		case protoreflect.GroupKind:
			return fmt.Errorf("%w: GroupKind is deprecated and not supported", ErrUnexpectedFieldValueType)

		default:
			return fmt.Errorf("%w: unsupported map value type: %s", ErrUnexpectedFieldValueType, valueField.Kind())
		}

		mapValue.Set(mapKey, mapVal)
	}

	return nil
}

// setEnumField handles enum fields.
func (l *ConfigLoader) setEnumField(msg protoreflect.Message, field protoreflect.FieldDescriptor, value any) error {
	enumDesc := field.Enum()

	// Value can be string (enum name) or int (enum number)
	switch v := value.(type) {
	case string:
		// Look up enum by name
		enumVal := enumDesc.Values().ByName(protoreflect.Name(v))
		if enumVal == nil {
			return fmt.Errorf("%w: unknown enum value: %s", ErrUnknownField, v)
		}
		msg.Set(field, protoreflect.ValueOfEnum(enumVal.Number()))

	case int, int32, int64, float64:
		// Convert to enum number
		var num int32
		switch val := v.(type) {
		case int:
			if val < -2147483648 || val > 2147483647 {
				return fmt.Errorf("%w: int value %d overflows int32", ErrOverflow, val)
			}
			num = int32(val)
		case int32:
			num = val
		case int64:
			if val < -2147483648 || val > 2147483647 {
				return fmt.Errorf("%w: int64 value %d overflows int32", ErrOverflow, val)
			}
			num = int32(val)
		case float64:
			if val < -2147483648 || val > 2147483647 {
				return fmt.Errorf("%w: float64 value %f overflows int32", ErrOverflow, val)
			}
			num = int32(val)
		}
		msg.Set(field, protoreflect.ValueOfEnum(protoreflect.EnumNumber(num)))

	default:
		return fmt.Errorf("%w: expected string or int for enum, got %T", ErrUnexpectedFieldValueType, value)
	}

	return nil
}

// applyEnvVars overrides fields with environment variables.
func (l *ConfigLoader) applyEnvVars(target proto.Message) error {
	if l.envPrefix == "" {
		return nil
	}

	return l.applyEnvVarsRecursive(target.ProtoReflect(), l.envPrefix)
}

// applyEnvVarsRecursive recursively applies environment variables to nested messages.
func (l *ConfigLoader) applyEnvVarsRecursive(msg protoreflect.Message, prefix string) error {
	return l.applyEnvVarsWithPath(msg, prefix, "")
}

func (l *ConfigLoader) applyEnvVarsWithPath(msg protoreflect.Message, prefix string, fieldPath string) error {
	fields := msg.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Build environment variable name from field name
		// Convert snake_case to UPPER_CASE
		fieldName := string(field.Name())
		envName := prefix + "_" + strings.ToUpper(fieldName)

		// Build nested field path for error messages
		currentPath := fieldName
		if fieldPath != "" {
			currentPath = fieldPath + "." + fieldName
		}

		// Handle nested messages recursively
		if field.Kind() == protoreflect.MessageKind && !field.IsList() && !field.IsMap() {
			// Check if this field is already set
			if !msg.Has(field) {
				// Create new message instance
				nestedMsg := msg.NewField(field).Message()
				msg.Set(field, protoreflect.ValueOfMessage(nestedMsg))
			}

			// Recursively apply env vars to nested message
			nestedMsg := msg.Get(field).Message()
			if err := l.applyEnvVarsWithPath(nestedMsg, envName, currentPath); err != nil {
				return err
			}
			continue
		}

		// Check if env var is set
		envValue, exists := os.LookupEnv(envName)
		if !exists {
			continue
		}

		// Track debug info
		if l.debug {
			l.debugInfo.EnvVarsApplied[envName] = envValue
		}

		// Parse and set value based on type
		if err := l.setFieldFromString(msg, field, envValue); err != nil {
			return fmt.Errorf("failed to set field %s from env %s: %w", currentPath, envName, err)
		}
	}

	return nil
}

// applyFlags overrides fields with CLI flags (single-command mode only).
func (l *ConfigLoader) applyFlags(cmd *cli.Command, target proto.Message) error {
	return l.applyFlagsRecursive(cmd, target.ProtoReflect(), "", "")
}

// applyFlagsRecursive recursively applies CLI flags to nested messages.
func (l *ConfigLoader) applyFlagsRecursive(cmd *cli.Command, msg protoreflect.Message, prefix string, fieldPath string) error {
	fields := msg.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Get flag name from field
		flagName := l.getFlagName(field)
		if flagName == "" {
			continue
		}

		// Build nested field path for error messages
		fieldName := string(field.Name())
		currentPath := fieldName
		if fieldPath != "" {
			currentPath = fieldPath + "." + fieldName
		}

		// Add prefix for nested fields (e.g., "database-url" becomes "database-url")
		fullFlagName := flagName
		if prefix != "" {
			fullFlagName = prefix + "-" + flagName
		}

		// Handle nested messages recursively
		if field.Kind() == protoreflect.MessageKind && !field.IsList() && !field.IsMap() {
			// Check if this field is already set
			if !msg.Has(field) {
				// Create new message instance
				nestedMsg := msg.NewField(field).Message()
				msg.Set(field, protoreflect.ValueOfMessage(nestedMsg))
			}

			// Recursively apply flags to nested message
			nestedMsg := msg.Get(field).Message()
			if err := l.applyFlagsRecursive(cmd, nestedMsg, fullFlagName, currentPath); err != nil {
				return err
			}
			continue
		}

		// Check if flag was set by user (not just default)
		if !cmd.IsSet(fullFlagName) {
			continue
		}

		// Track debug info
		if l.debug {
			flagValue := cmd.String(fullFlagName) // Simplified - works for most types
			l.debugInfo.FlagsApplied[fullFlagName] = flagValue
		}

		// Get flag value and set field
		if err := l.setFieldFromFlag(cmd, msg, field, fullFlagName); err != nil {
			return fmt.Errorf("failed to set field %s from flag %s: %w", currentPath, fullFlagName, err)
		}
	}

	return nil
}

// getFlagName extracts flag name from field using (cli.flag) annotation.
func (l *ConfigLoader) getFlagName(field protoreflect.FieldDescriptor) string {
	// Try to read the (cli.flag) annotation
	opts := field.Options()
	if opts != nil && proto.HasExtension(opts, clipb.E_Flag) {
		ext := proto.GetExtension(opts, clipb.E_Flag)
		if flagOpts, ok := ext.(*clipb.FlagOptions); ok && flagOpts.Name != "" {
			return flagOpts.Name
		}
	}

	// Fallback: convert field name to kebab-case
	fieldName := string(field.Name())
	return strings.ReplaceAll(fieldName, "_", "-")
}

// setFieldFromString parses a string value and sets the field.
func (l *ConfigLoader) setFieldFromString(msg protoreflect.Message, field protoreflect.FieldDescriptor, value string) error {
	switch field.Kind() {
	case protoreflect.StringKind:
		msg.Set(field, protoreflect.ValueOfString(value))

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		intVal, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfInt32(int32(intVal)))

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfInt64(intVal))

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		uintVal, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfUint32(uint32(uintVal)))

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfUint64(uintVal))

	case protoreflect.BoolKind:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfBool(boolVal))

	case protoreflect.FloatKind:
		floatVal, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfFloat32(float32(floatVal)))

	case protoreflect.DoubleKind:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfFloat64(floatVal))

	case protoreflect.BytesKind:
		// Parse bytes from string (direct string-to-bytes conversion)
		msg.Set(field, protoreflect.ValueOfBytes([]byte(value)))

	case protoreflect.EnumKind:
		// Parse enum from string (name or number)
		enumDesc := field.Enum()
		// Try as enum name first
		enumVal := enumDesc.Values().ByName(protoreflect.Name(value))
		if enumVal != nil {
			msg.Set(field, protoreflect.ValueOfEnum(enumVal.Number()))
		} else {
			// Try as number
			num, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return fmt.Errorf("%w: invalid enum value: %s (not a valid name or number)", ErrUnknownField, value)
			}
			msg.Set(field, protoreflect.ValueOfEnum(protoreflect.EnumNumber(num)))
		}

	case protoreflect.MessageKind:
		return fmt.Errorf("%w: MessageKind cannot be set from environment variable string", ErrUnexpectedFieldValueType)

	case protoreflect.GroupKind:
		return fmt.Errorf("%w: GroupKind is deprecated and not supported", ErrUnexpectedFieldValueType)

	default:
		return fmt.Errorf("%w: unsupported field type: %s", ErrUnexpectedFieldValueType, field.Kind())
	}

	return nil
}

// setFieldFromFlag gets flag value from CLI command and sets field.
func (l *ConfigLoader) setFieldFromFlag(cmd *cli.Command, msg protoreflect.Message, field protoreflect.FieldDescriptor, flagName string) error {
	switch field.Kind() {
	case protoreflect.StringKind:
		value := cmd.String(flagName)
		msg.Set(field, protoreflect.ValueOfString(value))

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		value := cmd.Int(flagName)
		if value < -2147483648 || value > 2147483647 {
			return fmt.Errorf("%w: int value %d overflows int32 for flag %s", ErrOverflow, value, flagName)
		}
		msg.Set(field, protoreflect.ValueOfInt32(int32(value)))

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		value := cmd.Int(flagName)
		msg.Set(field, protoreflect.ValueOfInt64(int64(value)))

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		value := cmd.Uint(flagName)
		if value > 4294967295 {
			return fmt.Errorf("%w: uint value %d overflows uint32 for flag %s", ErrOverflow, value, flagName)
		}
		msg.Set(field, protoreflect.ValueOfUint32(uint32(value)))

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		value := cmd.Uint(flagName)
		msg.Set(field, protoreflect.ValueOfUint64(uint64(value)))

	case protoreflect.BoolKind:
		value := cmd.Bool(flagName)
		msg.Set(field, protoreflect.ValueOfBool(value))

	case protoreflect.FloatKind:
		value := cmd.Float(flagName)
		msg.Set(field, protoreflect.ValueOfFloat32(float32(value)))

	case protoreflect.DoubleKind:
		value := cmd.Float(flagName)
		msg.Set(field, protoreflect.ValueOfFloat64(value))

	case protoreflect.BytesKind:
		// Get bytes from string flag
		value := cmd.String(flagName)
		msg.Set(field, protoreflect.ValueOfBytes([]byte(value)))

	case protoreflect.EnumKind:
		// Parse enum from string flag (name or number)
		value := cmd.String(flagName)
		enumDesc := field.Enum()
		// Try as enum name first
		enumVal := enumDesc.Values().ByName(protoreflect.Name(value))
		if enumVal != nil {
			msg.Set(field, protoreflect.ValueOfEnum(enumVal.Number()))
		} else {
			// Try as number
			num, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return fmt.Errorf("%w: invalid enum value: %s (not a valid name or number)", ErrUnknownField, value)
			}
			msg.Set(field, protoreflect.ValueOfEnum(protoreflect.EnumNumber(num)))
		}

	case protoreflect.MessageKind:
		return fmt.Errorf("%w: MessageKind cannot be set from CLI flag", ErrUnexpectedFieldValueType)

	case protoreflect.GroupKind:
		return fmt.Errorf("%w: GroupKind is deprecated and not supported", ErrUnexpectedFieldValueType)

	default:
		return fmt.Errorf("%w: unsupported field type: %s", ErrUnexpectedFieldValueType, field.Kind())
	}

	return nil
}

// NewConfigMessage creates a new config message instance using the proto registry.
// The configType should be a pointer to the config message type (e.g., &UserServiceConfig{}).
func NewConfigMessage(configType proto.Message) proto.Message {
	// Use proto.Clone to create a new instance of the same type
	return proto.Clone(configType)
}

// CallFactory calls a factory function with a config message using reflection.
// Returns the service implementation.
func CallFactory(factory any, config proto.Message) (any, error) {
	// Use type assertion to call factory with proper signature
	// The factory should be func(*ConfigMsg) ServiceServer

	// We need to use reflection since we don't know the exact types at compile time
	factoryValue := reflect.ValueOf(factory)
	if factoryValue.Kind() != reflect.Func {
		return nil, fmt.Errorf("%w: factory is not a function", ErrUnexpectedFieldValueType)
	}

	// Call the factory with the config
	results := factoryValue.Call([]reflect.Value{reflect.ValueOf(config)})
	if len(results) != 1 {
		return nil, fmt.Errorf("%w: factory should return exactly one value", ErrUnexpectedFieldValueType)
	}

	return results[0].Interface(), nil
}
