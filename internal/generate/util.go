package generate

import (
	"strings"
	"unicode"

	"github.com/dave/jennifer/jen"
	annotations "github.com/drewfead/proto-cli/proto/cli/v1"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

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
