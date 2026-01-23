package protocli

import (
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

// ConfigMode determines which config sources are used
type ConfigMode int

const (
	// SingleCommandMode uses files + env + flags (all sources)
	SingleCommandMode ConfigMode = iota
	// DaemonMode uses files + env only (no CLI flag overrides)
	DaemonMode
)

// ConfigLoader loads configuration with precedence: CLI flags > env vars > files
type ConfigLoader struct {
	configPaths   []string
	configReaders []io.Reader
	envPrefix     string
	mode          ConfigMode
}

// ConfigLoaderOption is a functional option for configuring a ConfigLoader
type ConfigLoaderOption func(*ConfigLoader)

// FileConfig adds config file paths to load
func FileConfig(paths ...string) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.configPaths = append(l.configPaths, paths...)
	}
}

// ReaderConfig adds io.Readers to load config from (for testing)
func ReaderConfig(readers ...io.Reader) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.configReaders = append(l.configReaders, readers...)
	}
}

// EnvPrefix sets the environment variable prefix for config overrides
func EnvPrefix(prefix string) ConfigLoaderOption {
	return func(l *ConfigLoader) {
		l.envPrefix = prefix
	}
}

// NewConfigLoader creates a new config loader with options
func NewConfigLoader(mode ConfigMode, opts ...ConfigLoaderOption) *ConfigLoader {
	loader := &ConfigLoader{
		mode: mode,
	}
	for _, opt := range opts {
		opt(loader)
	}
	return loader
}

// DefaultConfigPaths returns default paths for config files
func DefaultConfigPaths(rootCommandName string) []string {
	home, _ := os.UserHomeDir()
	return []string{
		fmt.Sprintf("./%s.yaml", rootCommandName),
		filepath.Join(home, ".config", rootCommandName, "config.yaml"),
	}
}

// LoadServiceConfig loads config for a specific service
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

	return nil
}

// loadFromFiles loads and deep merges config from multiple YAML files and readers
func (l *ConfigLoader) loadFromFiles(serviceName string, target proto.Message) error {
	// Load from file paths
	for _, path := range l.configPaths {
		// Skip if file doesn't exist (silent ignore for default paths)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		if err := l.loadYAMLServiceFromData(data, serviceName, target); err != nil {
			return fmt.Errorf("failed to load %s: %w", path, err)
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

// loadYAMLServiceFromData loads YAML from bytes and extracts service section
func (l *ConfigLoader) loadYAMLServiceFromData(data []byte, serviceName string, target proto.Message) error {
	// Parse YAML into map
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Extract services section
	services, ok := root["services"].(map[string]interface{})
	if !ok {
		// No services section, skip this file
		return nil
	}

	// Extract service-specific section
	serviceConfig, ok := services[serviceName].(map[string]interface{})
	if !ok {
		// No config for this service, skip
		return nil
	}

	// Merge config into target
	return l.mergeConfig(serviceConfig, target)
}

// mergeConfig deep merges YAML data into proto message using reflection
func (l *ConfigLoader) mergeConfig(data map[string]interface{}, target proto.Message) error {
	msg := target.ProtoReflect()
	fields := msg.Descriptor().Fields()

	for key, value := range data {
		// Convert kebab-case to snake_case for field lookup
		fieldName := strings.ReplaceAll(key, "-", "_")

		// Find field by name
		field := fields.ByName(protoreflect.Name(fieldName))
		if field == nil {
			// Also try JSON name
			field = fields.ByJSONName(key)
		}
		if field == nil {
			return fmt.Errorf("unknown field: %s", key)
		}

		// Set field value
		if err := l.setFieldValue(msg, field, value); err != nil {
			return fmt.Errorf("failed to set field %s: %w", key, err)
		}
	}

	return nil
}

// setFieldValue sets a proto field value based on type
func (l *ConfigLoader) setFieldValue(msg protoreflect.Message, field protoreflect.FieldDescriptor, value interface{}) error {
	switch field.Kind() {
	case protoreflect.StringKind:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
		msg.Set(field, protoreflect.ValueOfString(str))

	case protoreflect.Int32Kind, protoreflect.Int64Kind:
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
			return fmt.Errorf("expected int, got %T", value)
		}
		msg.Set(field, protoreflect.ValueOfInt64(intVal))

	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		var uintVal uint64
		switch v := value.(type) {
		case int:
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
			return fmt.Errorf("expected uint, got %T", value)
		}
		msg.Set(field, protoreflect.ValueOfUint64(uintVal))

	case protoreflect.BoolKind:
		boolVal, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", value)
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
			return fmt.Errorf("expected float, got %T", value)
		}
		msg.Set(field, protoreflect.ValueOfFloat64(floatVal))

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// applyEnvVars overrides fields with environment variables
func (l *ConfigLoader) applyEnvVars(target proto.Message) error {
	if l.envPrefix == "" {
		return nil
	}

	msg := target.ProtoReflect()
	fields := msg.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Build environment variable name from field name
		// Convert snake_case to UPPER_CASE
		fieldName := string(field.Name())
		envName := l.envPrefix + "_" + strings.ToUpper(fieldName)

		// Check if env var is set
		envValue, exists := os.LookupEnv(envName)
		if !exists {
			continue
		}

		// Parse and set value based on type
		if err := l.setFieldFromString(msg, field, envValue); err != nil {
			return fmt.Errorf("failed to set field %s from env %s: %w", fieldName, envName, err)
		}
	}

	return nil
}

// applyFlags overrides fields with CLI flags (single-command mode only)
func (l *ConfigLoader) applyFlags(cmd *cli.Command, target proto.Message) error {
	msg := target.ProtoReflect()
	fields := msg.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Get flag name from field
		flagName := l.getFlagName(field)
		if flagName == "" {
			continue
		}

		// Check if flag was set by user (not just default)
		if !cmd.IsSet(flagName) {
			continue
		}

		// Get flag value and set field
		if err := l.setFieldFromFlag(cmd, msg, field, flagName); err != nil {
			return fmt.Errorf("failed to set field from flag %s: %w", flagName, err)
		}
	}

	return nil
}

// getFlagName extracts flag name from field using (cli.flag) annotation
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

// setFieldFromString parses a string value and sets the field
func (l *ConfigLoader) setFieldFromString(msg protoreflect.Message, field protoreflect.FieldDescriptor, value string) error {
	switch field.Kind() {
	case protoreflect.StringKind:
		msg.Set(field, protoreflect.ValueOfString(value))

	case protoreflect.Int32Kind:
		intVal, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfInt32(int32(intVal)))

	case protoreflect.Int64Kind:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfInt64(intVal))

	case protoreflect.Uint32Kind:
		uintVal, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return err
		}
		msg.Set(field, protoreflect.ValueOfUint32(uint32(uintVal)))

	case protoreflect.Uint64Kind:
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

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// setFieldFromFlag gets flag value from CLI command and sets field
func (l *ConfigLoader) setFieldFromFlag(cmd *cli.Command, msg protoreflect.Message, field protoreflect.FieldDescriptor, flagName string) error {
	switch field.Kind() {
	case protoreflect.StringKind:
		value := cmd.String(flagName)
		msg.Set(field, protoreflect.ValueOfString(value))

	case protoreflect.Int32Kind:
		value := cmd.Int(flagName)
		msg.Set(field, protoreflect.ValueOfInt32(int32(value)))

	case protoreflect.Int64Kind:
		value := cmd.Int(flagName)
		msg.Set(field, protoreflect.ValueOfInt64(int64(value)))

	case protoreflect.Uint32Kind:
		value := cmd.Uint(flagName)
		msg.Set(field, protoreflect.ValueOfUint32(uint32(value)))

	case protoreflect.Uint64Kind:
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

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// Helper function to get flag value generically
func getFlagValue(cmd *cli.Command, flagName string) (interface{}, error) {
	// This is a simplified version - in practice you'd need to determine flag type
	return cmd.Value(flagName), nil
}

// NewConfigMessage creates a new config message instance using the proto registry
// The configType should be a pointer to the config message type (e.g., &UserServiceConfig{})
func NewConfigMessage(configType proto.Message) proto.Message {
	// Use proto.Clone to create a new instance of the same type
	return proto.Clone(configType)
}

// CallFactory calls a factory function with a config message using reflection
// Returns the service implementation
func CallFactory(factory interface{}, config proto.Message) (interface{}, error) {
	// Use type assertion to call factory with proper signature
	// The factory should be func(*ConfigMsg) ServiceServer

	// We need to use reflection since we don't know the exact types at compile time
	factoryValue := reflect.ValueOf(factory)
	if factoryValue.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory is not a function")
	}

	// Call the factory with the config
	results := factoryValue.Call([]reflect.Value{reflect.ValueOf(config)})
	if len(results) != 1 {
		return nil, fmt.Errorf("factory should return exactly one value")
	}

	return results[0].Interface(), nil
}
