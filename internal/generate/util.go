package generate

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dave/jennifer/jen"
	annotations "github.com/drewfead/proto-cli/proto/cli/v1"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// flagTypeInfo maps a proto field kind to its urfave/cli v3 flag type name
// and the corresponding cmd accessor method name, for both singular and slice variants.
type flagTypeInfo struct {
	SingularFlag    string // e.g. "Int32Flag"
	SliceFlag       string // e.g. "Int32SliceFlag"
	SingularAccessor string // e.g. "Int32"
	SliceAccessor   string // e.g. "Int32Slice"
}

// scalarFlagTypes maps proto kinds to their urfave/cli flag type info.
// Special cases (message, group) are not included — they require custom handling.
// Bool, bytes, and enum slice variants use StringSliceFlag since urfave/cli v3
// has no BoolSliceFlag, and bytes/enums need string-based parsing.
var scalarFlagTypes = map[protoreflect.Kind]flagTypeInfo{
	protoreflect.Int32Kind:    {SingularFlag: "Int32Flag", SliceFlag: "Int32SliceFlag", SingularAccessor: "Int32", SliceAccessor: "Int32Slice"},
	protoreflect.Sint32Kind:   {SingularFlag: "Int32Flag", SliceFlag: "Int32SliceFlag", SingularAccessor: "Int32", SliceAccessor: "Int32Slice"},
	protoreflect.Sfixed32Kind: {SingularFlag: "Int32Flag", SliceFlag: "Int32SliceFlag", SingularAccessor: "Int32", SliceAccessor: "Int32Slice"},
	protoreflect.Int64Kind:    {SingularFlag: "Int64Flag", SliceFlag: "Int64SliceFlag", SingularAccessor: "Int64", SliceAccessor: "Int64Slice"},
	protoreflect.Sint64Kind:   {SingularFlag: "Int64Flag", SliceFlag: "Int64SliceFlag", SingularAccessor: "Int64", SliceAccessor: "Int64Slice"},
	protoreflect.Sfixed64Kind: {SingularFlag: "Int64Flag", SliceFlag: "Int64SliceFlag", SingularAccessor: "Int64", SliceAccessor: "Int64Slice"},
	protoreflect.Uint32Kind:   {SingularFlag: "Uint32Flag", SliceFlag: "Uint32SliceFlag", SingularAccessor: "Uint32", SliceAccessor: "Uint32Slice"},
	protoreflect.Fixed32Kind:  {SingularFlag: "Uint32Flag", SliceFlag: "Uint32SliceFlag", SingularAccessor: "Uint32", SliceAccessor: "Uint32Slice"},
	protoreflect.Uint64Kind:   {SingularFlag: "Uint64Flag", SliceFlag: "Uint64SliceFlag", SingularAccessor: "Uint64", SliceAccessor: "Uint64Slice"},
	protoreflect.Fixed64Kind:  {SingularFlag: "Uint64Flag", SliceFlag: "Uint64SliceFlag", SingularAccessor: "Uint64", SliceAccessor: "Uint64Slice"},
	protoreflect.FloatKind:    {SingularFlag: "Float32Flag", SliceFlag: "Float32SliceFlag", SingularAccessor: "Float32", SliceAccessor: "Float32Slice"},
	protoreflect.DoubleKind:   {SingularFlag: "Float64Flag", SliceFlag: "Float64SliceFlag", SingularAccessor: "Float64", SliceAccessor: "Float64Slice"},
	protoreflect.StringKind:   {SingularFlag: "StringFlag", SliceFlag: "StringSliceFlag", SingularAccessor: "String", SliceAccessor: "StringSlice"},
	protoreflect.BoolKind:     {SingularFlag: "BoolFlag", SliceFlag: "StringSliceFlag", SingularAccessor: "Bool", SliceAccessor: "StringSlice"},
	protoreflect.BytesKind:    {SingularFlag: "StringFlag", SliceFlag: "StringSliceFlag", SingularAccessor: "String", SliceAccessor: "StringSlice"},
	protoreflect.EnumKind:     {SingularFlag: "StringFlag", SliceFlag: "StringSliceFlag", SingularAccessor: "String", SliceAccessor: "StringSlice"},
}

// cliFlagRef returns a jen expression for &cli.FlagType{dict} given a flag type name.
func cliFlagRef(flagTypeName string, dict jen.Dict) *jen.Statement {
	return jen.Op("&").Qual("github.com/urfave/cli/v3", flagTypeName).Values(dict)
}

// toKebabCase converts Go field names to kebab-case for CLI flags.
// Inserts a hyphen before each uppercase letter (except the first).
// Examples: StartTime -> start-time, CalendarId -> calendar-id.
func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('-')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// toTitleCase converts a kebab-case or snake_case identifier to Title Case.
// Each word separated by '-' or '_' is capitalised and words are joined with spaces.
// Examples: send-at → "Send At", name → "Name", color_hex → "Color Hex".
func toTitleCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// stripServiceSuffix removes "Service" suffix from service names to avoid redundancy.
// Examples: UserService -> User, StreamingService -> Streaming, Admin -> Admin.
func stripServiceSuffix(s string) string {
	return strings.TrimSuffix(s, "Service")
}

// cleanProtoComment trims whitespace from a protogen comment string.
func cleanProtoComment(comment protogen.Comments) string {
	return strings.TrimSpace(string(comment))
}

// firstLine returns the first line of a multiline string.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// qualifyType returns a jen.Code that properly references a Go type
// If the type is in the same package as the file being generated, use jen.Id()
// Otherwise, use jen.Qual() to import from the correct package.
func qualifyType(file *protogen.File, message *protogen.Message, pointer bool) *jen.Statement {
	var stmt *jen.Statement

	// Check if the message is in the same package as the file being generated
	if message.GoIdent.GoImportPath == file.GoImportPath {
		// Same package - use simple identifier
		stmt = jen.Id(message.GoIdent.GoName)
	} else {
		// Different package - use qualified import
		stmt = jen.Qual(string(message.GoIdent.GoImportPath), message.GoIdent.GoName)
	}

	if pointer {
		return jen.Op("*").Add(stmt)
	}
	return stmt
}

// getServiceConfigOptions extracts the service_config extension from service options.
func getServiceConfigOptions(service *protogen.Service) *annotations.ServiceConfigOptions {
	opts := service.Desc.Options()
	if opts == nil {
		return nil
	}

	if !proto.HasExtension(opts, annotations.E_ServiceConfig) {
		return nil
	}

	ext := proto.GetExtension(opts, annotations.E_ServiceConfig)
	if ext == nil {
		return nil
	}

	configOpts, ok := ext.(*annotations.ServiceConfigOptions)
	if !ok {
		return nil
	}

	return configOpts
}

// getServiceOptions extracts (cli.service) annotation from a service.
func getServiceOptions(service *protogen.Service) *annotations.ServiceOptions {
	opts := service.Desc.Options()
	if opts == nil {
		return nil
	}

	ext := proto.GetExtension(opts, annotations.E_Service)
	if ext == nil {
		return nil
	}

	serviceOpts, ok := ext.(*annotations.ServiceOptions)
	if !ok {
		return nil
	}

	return serviceOpts
}

// getMethodCommandOptions extracts (cli.command) annotation from a method.
func getMethodCommandOptions(method *protogen.Method) *annotations.CommandOptions {
	opts := method.Desc.Options()
	if opts == nil {
		return nil
	}

	ext := proto.GetExtension(opts, annotations.E_Command)
	if ext == nil {
		return nil
	}

	cmdOpts, ok := ext.(*annotations.CommandOptions)
	if !ok {
		return nil
	}

	return cmdOpts
}

// outputWriterFuncName returns the service-prefixed output writer function name.
// Example: UserService → "getUserServiceOutputWriter"
func outputWriterFuncName(service *protogen.Service) string {
	return "get" + service.GoName + "OutputWriter"
}

// enumParserFuncName returns the service-prefixed enum parser function name.
// Example: UserService, "LogLevel" → "parseUserServiceLogLevel"
func enumParserFuncName(service *protogen.Service, enumTypeName string) string {
	return "parse" + service.GoName + enumTypeName
}

// streamWrapperTypeName returns the service-prefixed stream wrapper type name.
// Example: UserService, GetUser → "localServerStream_UserService_GetUser"
func streamWrapperTypeName(service *protogen.Service, method *protogen.Method) string {
	return "localServerStream_" + service.GoName + "_" + method.GoName
}

// defaultValueCode returns a jen.Code literal for the given default string in
// the appropriate Go type for the flag kind, or nil if the string is empty or
// cannot be parsed. Used when emitting the Value field of urfave/cli flag structs.
func defaultValueCode(kind protoreflect.Kind, defaultStr string) jen.Code {
	if defaultStr == "" {
		return nil
	}
	switch kind {
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.EnumKind:
		return jen.Lit(defaultStr)
	case protoreflect.BoolKind:
		if b, err := strconv.ParseBool(defaultStr); err == nil {
			return jen.Lit(b)
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if n, err := strconv.ParseInt(defaultStr, 10, 32); err == nil {
			return jen.Lit(int32(n))
		}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if n, err := strconv.ParseInt(defaultStr, 10, 64); err == nil {
			return jen.Lit(n)
		}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if n, err := strconv.ParseUint(defaultStr, 10, 32); err == nil {
			return jen.Lit(uint32(n))
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if n, err := strconv.ParseUint(defaultStr, 10, 64); err == nil {
			return jen.Lit(n)
		}
	case protoreflect.FloatKind:
		if f, err := strconv.ParseFloat(defaultStr, 32); err == nil {
			return jen.Lit(float32(f))
		}
	case protoreflect.DoubleKind:
		if f, err := strconv.ParseFloat(defaultStr, 64); err == nil {
			return jen.Lit(f)
		}
	}
	return nil
}

// getFieldFlagOptions extracts the (cli.flag) annotation from a field
func getFieldFlagOptions(field *protogen.Field) *annotations.FlagOptions {
	opts := field.Desc.Options()
	if opts == nil {
		return nil
	}

	if !proto.HasExtension(opts, annotations.E_Flag) {
		return nil
	}

	ext := proto.GetExtension(opts, annotations.E_Flag)
	if ext == nil {
		return nil
	}

	flagOpts, ok := ext.(*annotations.FlagOptions)
	if !ok {
		return nil
	}

	return flagOpts
}
