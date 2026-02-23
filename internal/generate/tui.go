package generate

import (
	"fmt"
	"strings"

	"github.com/dave/jennifer/jen"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const protocliPkg = "github.com/drewfead/proto-cli"

// generateTUIDescriptor returns a jen expression initializing &protocli.TUIServiceDescriptor{...}.
// Returns nil if the service does not have tui=true.
func generateTUIDescriptor(file *protogen.File, service *protogen.Service) jen.Code {
	serviceOpts := getServiceOptions(service)
	if serviceOpts == nil || serviceOpts.GetTui() == nil {
		return nil
	}

	configOpts := getServiceConfigOptions(service)
	var configMessageType string
	if configOpts != nil && configOpts.ConfigMessage != "" {
		configMessageType = configOpts.ConfigMessage
	}

	// Collect service name/description/display name
	serviceName := toKebabCase(service.GoName)
	serviceDisplayName := service.GoName
	serviceDescription := stripServiceSuffix(service.GoName) + " commands"
	if serviceOpts != nil {
		if serviceOpts.Name != "" {
			serviceName = serviceOpts.Name
		}
		if serviceOpts.GetTui().GetName() != "" {
			serviceDisplayName = serviceOpts.GetTui().GetName()
		}
		if serviceOpts.Description != "" {
			serviceDescription = serviceOpts.Description
		}
	}

	// Generate method descriptors (skip client-streaming)
	var methodElems []jen.Code
	for _, method := range service.Methods {
		if method.Desc.IsStreamingClient() {
			continue
		}
		methodElems = append(methodElems, generateTUIMethodDescriptor(file, service, method, configMessageType))
	}

	return jen.Op("&").Qual(protocliPkg, "TUIServiceDescriptor").Values(jen.Dict{
		jen.Id("Name"):        jen.Lit(serviceName),
		jen.Id("DisplayName"): jen.Lit(serviceDisplayName),
		jen.Id("Description"): jen.Lit(serviceDescription),
		jen.Id("Methods"): jen.Index().Op("*").Qual(protocliPkg, "TUIMethodDescriptor").Values(
			methodElems...,
		),
	})
}

// generateTUIMethodDescriptor returns a jen expression initializing &protocli.TUIMethodDescriptor{...}.
func generateTUIMethodDescriptor(
	file *protogen.File,
	service *protogen.Service,
	method *protogen.Method,
	configMessageType string,
) jen.Code {
	cmdName := toKebabCase(method.GoName)
	displayName := method.GoName
	description := ""
	var hidden bool

	if cmdOpts := getMethodCommandOptions(method); cmdOpts != nil {
		if cmdOpts.Name != "" {
			cmdName = cmdOpts.Name
		}
		if cmdOpts.GetTui().GetName() != "" {
			displayName = cmdOpts.GetTui().GetName()
		}
		if cmdOpts.Description != "" {
			description = cmdOpts.Description
		}
		hidden = cmdOpts.GetTui().GetHidden()
	}

	if description == "" {
		if comment := cleanProtoComment(method.Comments.Leading); comment != "" {
			description = firstLine(comment)
		}
	}

	isStreaming := method.Desc.IsStreamingServer()
	reqQualifiedType := qualifyType(file, method.Input, false)

	// NewRequest closure
	newRequestClosure := jen.Func().Params().Qual("google.golang.org/protobuf/proto", "Message").Block(
		jen.Return(jen.Op("&").Add(reqQualifiedType).Values()),
	)

	// InputFields slice
	fieldDescs := generateTUIFieldDescriptors(file, service, method.Input, reqQualifiedType, nil)

	responseDescriptor := jen.Qual(protocliPkg, "TUIResponseDescriptor").Values(jen.Dict{
		jen.Id("MethodName"):      jen.Lit(cmdName),
		jen.Id("MessageFullName"): jen.Lit(string(method.Output.Desc.FullName())),
	})

	dict := jen.Dict{
		jen.Id("Name"):               jen.Lit(cmdName),
		jen.Id("DisplayName"):        jen.Lit(displayName),
		jen.Id("Description"):        jen.Lit(description),
		jen.Id("Hidden"):             jen.Lit(hidden),
		jen.Id("IsStreaming"):        jen.Lit(isStreaming),
		jen.Id("ResponseDescriptor"): responseDescriptor,
		jen.Id("NewRequest"):         newRequestClosure,
		jen.Id("InputFields"):        jen.Index().Qual(protocliPkg, "TUIFieldDescriptor").Values(fieldDescs...),
		jen.Id("Invoke"):             generateTUIInvokeClosure(file, service, method, configMessageType),
	}

	if isStreaming {
		dict[jen.Id("InvokeStream")] = generateTUIInvokeStreamClosure(file, service, method, configMessageType)
	}

	return jen.Op("&").Qual(protocliPkg, "TUIMethodDescriptor").Values(dict)
}

// generateTUIFieldDescriptors generates TUIFieldDescriptor literal elements for the message fields.
// reqQualifiedType is the jen statement for the concrete request type (for type assertions in setters).
// parentInit is an optional list of initialization statements (jen code) needed before accessing a parent field
// (e.g., ensuring a nested message is not nil). For top-level fields this is nil.
func generateTUIFieldDescriptors(
	file *protogen.File,
	service *protogen.Service,
	message *protogen.Message,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) []jen.Code {
	var descriptors []jen.Code

	for _, field := range message.Fields {
		desc := generateTUIFieldDescriptor(file, service, field, reqQualifiedType, parentChain)
		if desc != nil {
			descriptors = append(descriptors, desc)
		}
	}

	return descriptors
}

// fieldChainEntry tracks a parent field accessor for setter generation.
type fieldChainEntry struct {
	goName   string // Go field name on the parent struct (e.g. "Address")
	typeName string // Go type name of this message (e.g. "Address")
	typeCode *jen.Statement
}

// generateTUIFieldDescriptor generates a single TUIFieldDescriptor literal.
// Returns nil for unsupported field types (GroupKind, client-streaming, etc.).
func generateTUIFieldDescriptor(
	file *protogen.File,
	service *protogen.Service,
	field *protogen.Field,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) jen.Code {
	flagOpts := getFieldFlagOptions(field)

	// Determine flag name and label
	flagName := toKebabCase(field.GoName)
	label := flagName
	usage := ""
	description := ""
	defaultValue := ""
	required := false
	hidden := false

	if flagOpts != nil {
		if flagOpts.Name != "" {
			flagName = flagOpts.Name
			label = flagName
		}
		if flagOpts.GetTui().GetLabel() != "" {
			label = flagOpts.GetTui().GetLabel()
		}
		if flagOpts.Usage != "" {
			usage = flagOpts.Usage
		}
		if flagOpts.Description != "" {
			description = flagOpts.Description
		}
		defaultValue = flagOpts.GetDefaultValue()
		required = flagOpts.GetRequired()
		hidden = flagOpts.GetTui().GetHidden()
	}
	// Title-case auto-derived labels. Explicit tui.label annotations are kept
	// as-is so the developer's casing and phrasing are preserved.
	if flagOpts == nil || flagOpts.GetTui().GetLabel() == "" {
		label = toTitleCase(label)
	}

	if usage == "" {
		if comment := cleanProtoComment(field.Comments.Leading); comment != "" {
			usage = firstLine(comment)
		}
	}

	// For well-known types with no usage hint, provide a helpful format description.
	if field.Desc.Kind() == protoreflect.MessageKind && usage == "" {
		if hint := tuiWKTUsageHint(string(field.Message.Desc.FullName())); hint != "" {
			usage = hint
		}
	}

	// Determine kind and generate setter/appender
	kind, setter, appender := generateTUIFieldKindAndSetters(
		file, service, field, reqQualifiedType, parentChain,
	)
	if kind < 0 {
		// Unsupported field kind (GroupKind)
		return nil
	}

	dict := jen.Dict{
		jen.Id("Name"):         jen.Lit(flagName),
		jen.Id("Label"):        jen.Lit(label),
		jen.Id("Usage"):        jen.Lit(usage),
		jen.Id("Description"):  jen.Lit(description),
		jen.Id("DefaultValue"): jen.Lit(defaultValue),
		jen.Id("Required"):     jen.Lit(required),
		jen.Id("Hidden"):       jen.Lit(hidden),
		jen.Id("Kind"):         jen.Qual(protocliPkg, tuiKindConstant(kind)),
	}

	if field.Desc.Kind() == protoreflect.EnumKind {
		dict[jen.Id("EnumValues")] = generateTUIEnumValues(field.Enum)
	}

	if field.Desc.IsList() {
		dict[jen.Id("ElementKind")] = jen.Qual(protocliPkg, tuiKindConstant(elementKindForField(field)))
	}

	if field.Desc.Kind() == protoreflect.MessageKind {
		dict[jen.Id("MessageFullName")] = jen.Lit(string(field.Message.Desc.FullName()))
	}

	if setter != nil {
		dict[jen.Id("Setter")] = setter
	}
	if appender != nil {
		dict[jen.Id("Appender")] = appender
	}

	// For message fields, recurse to generate nested fields (skip well-known types — they have a setter)
	if field.Desc.Kind() == protoreflect.MessageKind && !field.Desc.IsList() {
		fullName := string(field.Message.Desc.FullName())
		if !isWellKnownType(fullName) {
			chain := append(parentChain, fieldChainEntry{ //nolint:gocritic
				goName:   field.GoName,
				typeName: field.Message.GoIdent.GoName,
				typeCode: qualifyType(file, field.Message, false),
			})
			nestedFields := generateTUIFieldDescriptors(file, service, field.Message, reqQualifiedType, chain)
			if len(nestedFields) > 0 {
				dict[jen.Id("Fields")] = jen.Index().Qual(protocliPkg, "TUIFieldDescriptor").Values(nestedFields...)
			}
		}
	}

	return jen.Qual(protocliPkg, "TUIFieldDescriptor").Values(dict)
}

// tuiFieldKind represents the TUIFieldKind constants.
// We use int to match TUIFieldKind.
const (
	kindString   = 0
	kindInt      = 1
	kindFloat    = 2
	kindBool     = 3
	kindEnum     = 4
	kindRepeated = 5
	kindMessage  = 6
	kindSkip     = -1
)

// tuiKindConstant returns the Go constant name for a kind value.
func tuiKindConstant(kind int) string {
	switch kind {
	case kindString:
		return "TUIFieldKindString"
	case kindInt:
		return "TUIFieldKindInt"
	case kindFloat:
		return "TUIFieldKindFloat"
	case kindBool:
		return "TUIFieldKindBool"
	case kindEnum:
		return "TUIFieldKindEnum"
	case kindRepeated:
		return "TUIFieldKindRepeated"
	case kindMessage:
		return "TUIFieldKindMessage"
	default:
		return "TUIFieldKindString"
	}
}

// protoKindToTUIKind maps a proto field kind to the TUI field kind constant.
func protoKindToTUIKind(k protoreflect.Kind) int {
	switch k {
	case protoreflect.StringKind, protoreflect.BytesKind:
		return kindString
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return kindInt
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return kindFloat
	case protoreflect.BoolKind:
		return kindBool
	case protoreflect.EnumKind:
		return kindEnum
	case protoreflect.MessageKind:
		return kindMessage
	default:
		return kindSkip
	}
}

// elementKindForField returns the TUI kind for the elements of a repeated field.
func elementKindForField(field *protogen.Field) int {
	return protoKindToTUIKind(field.Desc.Kind())
}

// isWellKnownType returns true for proto well-known types that have special string-input handling.
func isWellKnownType(fullName string) bool {
	switch fullName {
	case "google.protobuf.Timestamp", "google.protobuf.Duration":
		return true
	default:
		return false
	}
}

// tuiWKTUsageHint returns a format hint string for a well-known type, or empty string.
func tuiWKTUsageHint(fullName string) string {
	switch fullName {
	case "google.protobuf.Timestamp":
		return "RFC3339 timestamp (e.g. 2006-01-02T15:04:05Z)"
	case "google.protobuf.Duration":
		return "Go duration (e.g. 1h30m, 300ms)"
	default:
		return ""
	}
}

// generateWKTSetterClosure generates a setter for well-known type message fields.
// Timestamp expects RFC3339; Duration expects Go duration notation.
func generateWKTSetterClosure(
	field *protogen.Field,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) jen.Code {
	fullName := string(field.Message.Desc.FullName())
	flagName := toKebabCase(field.GoName)
	if flagOpts := getFieldFlagOptions(field); flagOpts != nil && flagOpts.Name != "" {
		flagName = flagOpts.Name
	}

	initStmts, fieldAccess := buildAccessorChain(parentChain, field.GoName)

	var body []jen.Code
	body = append(body,
		jen.Id("req").Op(":=").Id("msg").Assert(jen.Op("*").Add(reqQualifiedType.Clone())),
	)
	body = append(body, initStmts...)

	switch fullName {
	case "google.protobuf.Timestamp":
		body = append(body,
			jen.List(jen.Id("t"), jen.Err()).Op(":=").Qual("time", "Parse").Call(
				jen.Qual("time", "RFC3339"), jen.Id("s"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid RFC3339 timestamp for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Qual("google.golang.org/protobuf/types/known/timestamppb", "New").Call(jen.Id("t")),
		)

	case "google.protobuf.Duration":
		body = append(body,
			jen.List(jen.Id("d"), jen.Err()).Op(":=").Qual("time", "ParseDuration").Call(jen.Id("s")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid duration for %s (use Go format e.g. 1h30m): %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Qual("google.golang.org/protobuf/types/known/durationpb", "New").Call(jen.Id("d")),
		)

	default:
		return nil
	}

	body = append(body, jen.Return(jen.Nil()))

	return jen.Func().Params(
		jen.Id("msg").Qual("google.golang.org/protobuf/proto", "Message"),
		jen.Id("s").String(),
	).Error().Block(body...)
}

// generateTUIEnumValues returns a jen expression for the EnumValues slice.
func generateTUIEnumValues(enum *protogen.Enum) jen.Code {
	var elems []jen.Code
	for _, value := range enum.Values {
		if value.Desc.Number() == 0 {
			continue
		}
		name := getEnumValueCLIName(value)
		if name == "" {
			name = strings.ToLower(string(value.Desc.Name()))
		}
		elems = append(elems, jen.Qual(protocliPkg, "TUIEnumValue").Values(jen.Dict{
			jen.Id("Name"):  jen.Lit(name),
			jen.Id("Value"): jen.Lit(int(value.Desc.Number())),
		}))
	}
	return jen.Index().Qual(protocliPkg, "TUIEnumValue").Values(elems...)
}

// generateTUIFieldKindAndSetters returns (kind, setter, appender) for a field.
// kind=-1 means skip this field.
// For repeated fields, Appender is set instead of Setter.
// For message fields, neither Setter nor Appender is set (handled via nested Fields).
func generateTUIFieldKindAndSetters(
	file *protogen.File,
	service *protogen.Service,
	field *protogen.Field,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) (kind int, setter jen.Code, appender jen.Code) {
	k := field.Desc.Kind()

	if k == protoreflect.GroupKind {
		return kindSkip, nil, nil
	}

	isList := field.Desc.IsList()

	if isList {
		// Repeated field — generate Appender
		elemKind := protoKindToTUIKind(k)
		if elemKind == kindSkip || elemKind == kindMessage {
			// Can't auto-generate for repeated messages
			return kindRepeated, nil, nil
		}
		appender = generateTUIAppenderClosure(service, field, reqQualifiedType, parentChain)
		return kindRepeated, nil, appender
	}

	if k == protoreflect.MessageKind {
		// Well-known types get a string setter instead of nested Fields.
		fullName := string(field.Message.Desc.FullName())
		if isWellKnownType(fullName) {
			setter = generateWKTSetterClosure(field, reqQualifiedType, parentChain)
			return kindString, setter, nil
		}
		// Other message fields: no setter, nested Fields are handled separately
		return kindMessage, nil, nil
	}

	// Scalar field — generate Setter
	tKind := protoKindToTUIKind(k)
	if tKind == kindSkip {
		return kindSkip, nil, nil
	}

	setter = generateTUISetterClosure(service, field, reqQualifiedType, parentChain)
	return tKind, setter, nil
}

// buildAccessorPath returns a jen expression for accessing req.Parent1.Parent2...Field.
// It always builds a fresh statement to avoid jen's in-place mutation issues.
func buildAccessorPath(goNames ...string) *jen.Statement {
	s := jen.Id("req")
	for _, name := range goNames {
		s = s.Dot(name)
	}
	return s
}

// buildAccessorChain builds init statements for nested message fields and the final field accessor.
// It returns:
// - initStatements: statements to ensure each parent in the chain is non-nil
// - fieldAccess: the jen expression to access the final field (e.g. req.Address.Street or req.Id)
// Each returned statement is freshly constructed to avoid jen's in-place mutation issues.
func buildAccessorChain(parentChain []fieldChainEntry, fieldGoName string) (initStmts []jen.Code, fieldAccess *jen.Statement) {
	// Build init statements for each parent in the chain.
	// For each parent at index i, the init path is parentChain[0..i].
	for i, entry := range parentChain {
		// Build the accessor path up to and including this parent (fresh each time)
		names := make([]string, i+1)
		for j := 0; j <= i; j++ {
			names[j] = parentChain[j].goName
		}
		parentAccess := buildAccessorPath(names...)

		initStmts = append(initStmts,
			jen.If(buildAccessorPath(names...).Op("==").Nil()).Block(
				parentAccess.Op("=").Op("&").Add(entry.typeCode.Clone()).Values(),
			),
		)
	}

	// Build the final field accessor path (fresh)
	allNames := make([]string, len(parentChain)+1)
	for i, entry := range parentChain {
		allNames[i] = entry.goName
	}
	allNames[len(parentChain)] = fieldGoName
	fieldAccess = buildAccessorPath(allNames...)

	return initStmts, fieldAccess
}

// generateTUISetterClosure generates the Setter closure for a singular scalar/enum/bytes field.
//
//nolint:gocyclo,maintidx // Complexity is inherent in handling all proto kinds
func generateTUISetterClosure(
	service *protogen.Service,
	field *protogen.Field,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) jen.Code {
	flagName := toKebabCase(field.GoName)
	if flagOpts := getFieldFlagOptions(field); flagOpts != nil && flagOpts.Name != "" {
		flagName = flagOpts.Name
	}

	oneof := field.Desc.ContainingOneof()
	isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))

	initStmts, fieldAccess := buildAccessorChain(parentChain, field.GoName)

	// Build the body of the setter
	var body []jen.Code
	body = append(body,
		jen.Id("req").Op(":=").Id("msg").Assert(jen.Op("*").Add(reqQualifiedType.Clone())),
	)
	body = append(body, initStmts...)

	k := field.Desc.Kind()

	switch k {
	case protoreflect.StringKind:
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Id("s"),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("s"))
		}

	case protoreflect.BytesKind:
		body = append(body, fieldAccess.Clone().Op("=").Index().Byte().Call(jen.Id("s")))

	case protoreflect.BoolKind:
		body = append(body,
			jen.List(jen.Id("v"), jen.Err()).Op(":=").Qual("strconv", "ParseBool").Call(jen.Id("s")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid bool for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body, fieldAccess.Clone().Op("=").Op("&").Id("v"))
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("v"))
		}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("s"), jen.Lit(10), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid int32 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Int32().Call(jen.Id("n")),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Int32().Call(jen.Id("n")))
		}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("s"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid int64 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Id("n"),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("n"))
		}

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseUint").Call(jen.Id("s"), jen.Lit(10), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid uint32 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Uint32().Call(jen.Id("n")),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Uint32().Call(jen.Id("n")))
		}

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseUint").Call(jen.Id("s"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid uint64 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Id("n"),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("n"))
		}

	case protoreflect.FloatKind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseFloat").Call(jen.Id("s"), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid float32 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Float32().Call(jen.Id("n")),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Float32().Call(jen.Id("n")))
		}

	case protoreflect.DoubleKind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseFloat").Call(jen.Id("s"), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid float64 for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body,
				jen.Id("v").Op(":=").Id("n"),
				fieldAccess.Clone().Op("=").Op("&").Id("v"),
			)
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("n"))
		}

	case protoreflect.EnumKind:
		enumTypeName := field.Enum.GoIdent.GoName
		parserFuncName := enumParserFuncName(service, enumTypeName)
		body = append(body,
			jen.List(jen.Id("v"), jen.Err()).Op(":=").Id(parserFuncName).Call(jen.Id("s")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid enum for %s: %%w", flagName)),
					jen.Err(),
				)),
			),
		)
		if isOptional {
			body = append(body, fieldAccess.Clone().Op("=").Op("&").Id("v"))
		} else {
			body = append(body, fieldAccess.Clone().Op("=").Id("v"))
		}

	default:
		return nil
	}

	body = append(body, jen.Return(jen.Nil()))

	return jen.Func().Params(
		jen.Id("msg").Qual("google.golang.org/protobuf/proto", "Message"),
		jen.Id("s").String(),
	).Error().Block(body...)
}

// generateTUIAppenderClosure generates the Appender closure for a repeated scalar/enum field.
func generateTUIAppenderClosure(
	service *protogen.Service,
	field *protogen.Field,
	reqQualifiedType *jen.Statement,
	parentChain []fieldChainEntry,
) jen.Code {
	flagName := toKebabCase(field.GoName)
	if flagOpts := getFieldFlagOptions(field); flagOpts != nil && flagOpts.Name != "" {
		flagName = flagOpts.Name
	}

	initStmts, fieldAccess := buildAccessorChain(parentChain, field.GoName)

	var body []jen.Code
	body = append(body,
		jen.Id("req").Op(":=").Id("msg").Assert(jen.Op("*").Add(reqQualifiedType.Clone())),
	)
	body = append(body, initStmts...)

	k := field.Desc.Kind()

	switch k {
	case protoreflect.StringKind:
		body = append(body,
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("s")),
		)

	case protoreflect.BytesKind:
		body = append(body,
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Index().Byte().Call(jen.Id("s"))),
		)

	case protoreflect.BoolKind:
		body = append(body,
			jen.List(jen.Id("v"), jen.Err()).Op(":=").Qual("strconv", "ParseBool").Call(jen.Id("s")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid bool for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("v")),
		)

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("s"), jen.Lit(10), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid int32 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Int32().Call(jen.Id("n"))),
		)

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("s"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid int64 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("n")),
		)

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseUint").Call(jen.Id("s"), jen.Lit(10), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid uint32 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Uint32().Call(jen.Id("n"))),
		)

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseUint").Call(jen.Id("s"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid uint64 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("n")),
		)

	case protoreflect.FloatKind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseFloat").Call(jen.Id("s"), jen.Lit(32)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid float32 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Float32().Call(jen.Id("n"))),
		)

	case protoreflect.DoubleKind:
		body = append(body,
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "ParseFloat").Call(jen.Id("s"), jen.Lit(64)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid float64 for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("n")),
		)

	case protoreflect.EnumKind:
		enumTypeName := field.Enum.GoIdent.GoName
		parserFuncName := enumParserFuncName(service, enumTypeName)
		body = append(body,
			jen.List(jen.Id("v"), jen.Err()).Op(":=").Id(parserFuncName).Call(jen.Id("s")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("invalid enum for %s element: %%w", flagName)),
					jen.Err(),
				)),
			),
			fieldAccess.Clone().Op("=").Append(fieldAccess.Clone(), jen.Id("v")),
		)

	default:
		return nil
	}

	body = append(body, jen.Return(jen.Nil()))

	return jen.Func().Params(
		jen.Id("msg").Qual("google.golang.org/protobuf/proto", "Message"),
		jen.Id("s").String(),
	).Error().Block(body...)
}

// generateTUIInvokeClosure generates the Invoke closure for a unary (or server-streaming) method.
// For streaming methods this handles the initial call; InvokeStream handles receiving.
func generateTUIInvokeClosure(
	file *protogen.File,
	service *protogen.Service,
	method *protogen.Method,
	configMessageType string,
) jen.Code {
	reqQualifiedType := qualifyType(file, method.Input, true)
	outputTypeName := method.Output.GoIdent.GoName
	serviceName := strings.ToLower(service.GoName)

	var body []jen.Code
	body = append(body,
		jen.Id("typedReq").Op(":=").Id("req").Assert(reqQualifiedType),
	)

	if configMessageType != "" {
		body = append(body,
			jen.Id("rootCmd").Op(":=").Id("cmd").Dot("Root").Call(),
			jen.Id("configPaths").Op(":=").Id("rootCmd").Dot("StringSlice").Call(jen.Lit("config")),
			jen.Id("envPrefix").Op(":=").Id("rootCmd").Dot("String").Call(jen.Lit("env-prefix")),
			jen.Id("loader").Op(":=").Qual(protocliPkg, "NewConfigLoader").Call(
				jen.Qual(protocliPkg, "SingleCommandMode"),
				jen.Qual(protocliPkg, "FileConfig").Call(jen.Id("configPaths").Op("...")),
				jen.Qual(protocliPkg, "EnvPrefix").Call(jen.Id("envPrefix")),
			),
			jen.Id("config").Op(":=").Op("&").Id(configMessageType).Values(),
			jen.If(
				jen.Err().Op(":=").Id("loader").Dot("LoadServiceConfig").Call(
					jen.Id("cmd"),
					jen.Lit(serviceName),
					jen.Id("config"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to load config: %w"), jen.Err())),
			),
			jen.List(jen.Id("svcImpl"), jen.Err()).Op(":=").Qual(protocliPkg, "CallFactory").Call(
				jen.Id("implOrFactory"),
				jen.Id("config"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to create service: %w"), jen.Err())),
			),
			jen.List(jen.Id("resp"), jen.Err()).Op(":=").Id("svcImpl").Assert(jen.Id(service.GoName+"Server")).Dot(method.GoName).Call(
				jen.Id("ctx"),
				jen.Id("typedReq"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(jen.Lit("method failed: %w"), jen.Err())),
			),
		)
	} else {
		body = append(body,
			jen.Id("svcImpl").Op(":=").Id("implOrFactory").Assert(jen.Id(service.GoName+"Server")),
			jen.List(jen.Id("resp"), jen.Err()).Op(":=").Id("svcImpl").Dot(method.GoName).Call(
				jen.Id("ctx"),
				jen.Id("typedReq"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(jen.Lit("method failed: %w"), jen.Err())),
			),
		)
	}

	// For streaming methods, Invoke is not the primary path but we still need it to return something
	// We return nil response and nil error for streaming (InvokeStream handles the actual call)
	if method.Desc.IsStreamingServer() {
		// For server streaming, Invoke is a stub — InvokeStream should be used
		return jen.Func().Params(
			jen.Id("ctx").Qual("context", "Context"),
			jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
			jen.Id("req").Qual("google.golang.org/protobuf/proto", "Message"),
		).Params(
			jen.Qual("google.golang.org/protobuf/proto", "Message"),
			jen.Error(),
		).Block(
			jen.Return(
				jen.Nil(),
				jen.Qual("fmt", "Errorf").Call(jen.Lit("use InvokeStream for server-streaming method "+method.GoName)),
			),
		)
	}

	body = append(body,
		jen.Return(jen.Qual("google.golang.org/protobuf/proto", "Message").Call(jen.Id("resp")), jen.Nil()),
	)

	_ = outputTypeName // referenced implicitly via resp type

	return jen.Func().Params(
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
		jen.Id("req").Qual("google.golang.org/protobuf/proto", "Message"),
	).Params(
		jen.Qual("google.golang.org/protobuf/proto", "Message"),
		jen.Error(),
	).Block(body...)
}

// generateTUIInvokeStreamClosure generates the InvokeStream closure for server-streaming methods.
func generateTUIInvokeStreamClosure(
	file *protogen.File,
	service *protogen.Service,
	method *protogen.Method,
	configMessageType string,
) jen.Code {
	reqQualifiedType := qualifyType(file, method.Input, true)
	responseType := method.Output.GoIdent.GoName
	streamTypeName := streamWrapperTypeName(service, method)
	serviceName := strings.ToLower(service.GoName)

	var body []jen.Code
	body = append(body,
		jen.Id("typedReq").Op(":=").Id("req").Assert(reqQualifiedType),
	)

	if configMessageType != "" {
		body = append(body,
			jen.Id("rootCmd").Op(":=").Id("cmd").Dot("Root").Call(),
			jen.Id("configPaths").Op(":=").Id("rootCmd").Dot("StringSlice").Call(jen.Lit("config")),
			jen.Id("envPrefix").Op(":=").Id("rootCmd").Dot("String").Call(jen.Lit("env-prefix")),
			jen.Id("loader").Op(":=").Qual(protocliPkg, "NewConfigLoader").Call(
				jen.Qual(protocliPkg, "SingleCommandMode"),
				jen.Qual(protocliPkg, "FileConfig").Call(jen.Id("configPaths").Op("...")),
				jen.Qual(protocliPkg, "EnvPrefix").Call(jen.Id("envPrefix")),
			),
			jen.Id("config").Op(":=").Op("&").Id(configMessageType).Values(),
			jen.If(
				jen.Err().Op(":=").Id("loader").Dot("LoadServiceConfig").Call(
					jen.Id("cmd"),
					jen.Lit(serviceName),
					jen.Id("config"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to load config: %w"), jen.Err())),
			),
			jen.List(jen.Id("svcImpl"), jen.Id("factoryErr")).Op(":=").Qual(protocliPkg, "CallFactory").Call(
				jen.Id("implOrFactory"),
				jen.Id("config"),
			),
			jen.If(jen.Id("factoryErr").Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to create service: %w"), jen.Id("factoryErr"))),
			),
			jen.Id("typedImpl").Op(":=").Id("svcImpl").Assert(jen.Id(service.GoName+"Server")),
		)
	} else {
		body = append(body,
			jen.Id("typedImpl").Op(":=").Id("implOrFactory").Assert(jen.Id(service.GoName+"Server")),
		)
	}

	body = append(body,
		jen.Id("localStream").Op(":=").Op("&").Id(streamTypeName).Values(jen.Dict{
			jen.Id("ctx"):       jen.Id("ctx"),
			jen.Id("responses"): jen.Make(jen.Chan().Op("*").Id(responseType)),
			jen.Id("errors"):    jen.Make(jen.Chan().Error()),
		}),
		jen.Go().Func().Params().Block(
			jen.Var().Id("methodErr").Error(),
			jen.Id("methodErr").Op("=").Id("typedImpl").Dot(method.GoName).Call(
				jen.Id("typedReq"),
				jen.Id("localStream"),
			),
			jen.Close(jen.Id("localStream").Dot("responses")),
			jen.If(jen.Id("methodErr").Op("!=").Nil()).Block(
				jen.Id("localStream").Dot("errors").Op("<-").Id("methodErr"),
			),
			jen.Close(jen.Id("localStream").Dot("errors")),
		).Call(),
		jen.For().Block(
			jen.Select().Block(
				jen.Case(jen.List(jen.Id("msg"), jen.Id("ok")).Op(":=").Op("<-").Id("localStream").Dot("responses")).Block(
					jen.If(jen.Op("!").Id("ok")).Block(
						jen.If(jen.Id("streamErr").Op(":=").Op("<-").Id("localStream").Dot("errors"), jen.Id("streamErr").Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("stream error: %w"), jen.Id("streamErr"))),
						),
						jen.Return(jen.Nil()),
					),
					jen.If(jen.Id("recvErr").Op(":=").Id("recv").Call(jen.Id("msg")), jen.Id("recvErr").Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("recv callback failed: %w"), jen.Id("recvErr"))),
					),
				),
				jen.Case(jen.Op("<-").Id("ctx").Dot("Done").Call()).Block(
					jen.Return(jen.Id("ctx").Dot("Err").Call()),
				),
			),
		),
	)

	return jen.Func().Params(
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
		jen.Id("req").Qual("google.golang.org/protobuf/proto", "Message"),
		jen.Id("recv").Func().Params(jen.Qual("google.golang.org/protobuf/proto", "Message")).Error(),
	).Error().Block(body...)
}
