package generate

import (
	"fmt"
	"os"
	"strings"

	"github.com/dave/jennifer/jen"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// generateAfterHooksDefer generates code for executing after hooks in reverse order (LIFO) using defer.
// This is extracted to avoid code duplication between unary and streaming command generation.
func generateAfterHooksDefer() jen.Code {
	return jen.Defer().Func().Params().Block(
		jen.Id("hooks").Op(":=").Id("options").Dot("AfterCommandHooks").Call(),
		jen.For(
			jen.Id("i").Op(":=").Len(jen.Id("hooks")).Op("-").Lit(1),
			jen.Id("i").Op(">=").Lit(0),
			jen.Id("i").Op("--"),
		).Block(
			jen.If(
				jen.Err().Op(":=").Id("hooks").Index(jen.Id("i")).Call(
					jen.Id("cmdCtx"),
					jen.Id("cmd"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Qual("log/slog", "Warn").Call(
					jen.Lit("after hook failed"),
					jen.Lit("error"),
					jen.Err(),
				),
			),
		),
	).Call()
}

func generateMethodCommand(service *protogen.Service, method *protogen.Method, configMessageType string, file *protogen.File) []jen.Code {
	var statements []jen.Code

	// Get command name and help fields from annotation or use defaults
	cmdName := toKebabCase(method.GoName)
	cmdUsage := method.GoName                             // Short description for Usage field
	var cmdDescription, cmdUsageText, cmdArgsUsage string // Long description, custom usage line, args

	var localOnly bool

	// Determine whether the parent service is TUI-enabled (for --interactive flag generation).
	serviceOpts := getServiceOptions(service)
	tuiEnabled := serviceOpts != nil && serviceOpts.GetTui() != nil

	// Derive the service CLI name (mirrors generateServiceCommands logic).
	serviceCLIName := toKebabCase(service.GoName)
	if serviceOpts != nil && serviceOpts.Name != "" {
		serviceCLIName = serviceOpts.Name
	}

	cmdOpts := getMethodCommandOptions(method)
	if cmdOpts != nil {
		// Use custom command name if provided
		if cmdOpts.Name != "" {
			cmdName = cmdOpts.Name
		}
		// Use description if provided (short one-liner)
		if cmdOpts.Description != "" {
			cmdUsage = cmdOpts.Description
		}
		// Use long_description for detailed explanation
		if cmdOpts.LongDescription != "" {
			cmdDescription = cmdOpts.LongDescription
		}
		// Use custom usage_text to override auto-generated USAGE line
		if cmdOpts.UsageText != "" {
			cmdUsageText = cmdOpts.UsageText
		}
		// Use args_usage for argument description
		if cmdOpts.ArgsUsage != "" {
			cmdArgsUsage = cmdOpts.ArgsUsage
		}
		// Check if command should always run locally
		localOnly = cmdOpts.GetLocalOnly()
	}

	// Fallback to proto source comment if no annotation provided a description
	if cmdUsage == method.GoName {
		if comment := cleanProtoComment(method.Comments.Leading); comment != "" {
			cmdUsage = firstLine(comment)
		}
	}

	// Create a safe variable name (replace hyphens with underscores)
	cmdVarName := strings.ReplaceAll(cmdName, "-", "_")

	// Build flags dynamically with output format support
	initialFlags := []jen.Code{
		jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(jen.Dict{
			jen.Id("Name"):  jen.Lit("format"),
			jen.Id("Value"): jen.Id("defaultFormat"),
			jen.Id("Usage"): jen.Lit("Output format (use --format to see available formats)"),
		}),
		jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(jen.Dict{
			jen.Id("Name"):  jen.Lit("output"),
			jen.Id("Value"): jen.Lit("-"),
			jen.Id("Usage"): jen.Lit("Output file (- for stdout)"),
		}),
		jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(jen.Dict{
			jen.Id("Name"):  jen.Lit("input-file"),
			jen.Id("Usage"): jen.Lit("Read request from file (JSON or YAML). CLI flags override file values"),
		}),
		jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(jen.Dict{
			jen.Id("Name"):  jen.Lit("input-format"),
			jen.Id("Usage"): jen.Lit("Input file format (auto-detected from extension if not set)"),
		}),
	}
	if !localOnly {
		initialFlags = append([]jen.Code{
			jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(jen.Dict{
				jen.Id("Name"):  jen.Lit("remote"),
				jen.Id("Usage"): jen.Lit("Remote gRPC server address (host:port). If set, uses gRPC client instead of direct call"),
			}),
		}, initialFlags...)
	}
	statements = append(statements,
		jen.Comment("Build flags for "+cmdName),
		jen.Id("flags_"+cmdVarName).Op(":=").Index().Qual("github.com/urfave/cli/v3", "Flag").Values(initialFlags...),
		jen.Line(),
	)

	// Add request field flags
	for _, field := range method.Input.Fields {
		flagCode := generateFlag(field)
		if flagCode != nil {
			statements = append(statements,
				jen.Id("flags_"+cmdVarName).Op("=").Append(jen.Id("flags_"+cmdVarName), flagCode),
			)
		}
	}

	// Add config field flags if service has config
	statements = append(statements, generateConfigFlags(file, configMessageType, cmdVarName)...)

	// Add format-specific flags from registered formats
	statements = append(statements,
		jen.Line(),
		jen.Comment("Add format-specific flags from registered formats"),
		jen.For(
			jen.List(jen.Id("_"), jen.Id("outputFmt")).Op(":=").Range().Id("options").Dot("OutputFormats").Call(),
		).Block(
			jen.Comment("Check if format implements FlagConfiguredOutputFormat"),
			jen.If(
				jen.List(jen.Id("flagConfigured"), jen.Id("ok")).Op(":=").Id("outputFmt").Assert(
					jen.Qual("github.com/drewfead/proto-cli", "FlagConfiguredOutputFormat"),
				),
				jen.Id("ok"),
			).Block(
				jen.Id("flags_"+cmdVarName).Op("=").Append(
					jen.Id("flags_"+cmdVarName),
					jen.Id("flagConfigured").Dot("Flags").Call().Op("..."),
				),
			),
		),
		jen.Line(),
	)

	// For TUI-enabled services, add an --interactive flag to every method command so
	// users can deep-link: `myapp farewell leave-note --interactive`.
	if tuiEnabled {
		statements = append(statements,
			jen.Id("flags_"+cmdVarName).Op("=").Append(
				jen.Id("flags_"+cmdVarName),
				jen.Op("&").Qual("github.com/urfave/cli/v3", "BoolFlag").Values(jen.Dict{
					jen.Id("Name"):  jen.Lit("interactive"),
					jen.Id("Usage"): jen.Lit("Open the interactive TUI at this method's form"),
				}),
			),
		)
	}

	// Build command dict with help fields
	cmdDict := jen.Dict{
		jen.Id("Name"):  jen.Lit(cmdName),
		jen.Id("Usage"): jen.Lit(cmdUsage),
		jen.Id("Flags"): jen.Id("flags_" + cmdVarName),
		jen.Id("Action"): jen.Func().Params(
			jen.Id("cmdCtx").Qual("context", "Context"),
			jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
		).Error().Block(
			generateActionBodyWithHooks(file, service, method, configMessageType, localOnly)...,
		),
	}

	// For TUI-enabled services, add a Before hook that intercepts --interactive and
	// deep-links into the TUI at this method's request form.
	if tuiEnabled {
		cmdDict[jen.Id("Before")] = generateTUIBeforeHook(method, serviceCLIName, cmdName)
	}

	// Add optional help fields if provided
	if cmdDescription != "" {
		cmdDict[jen.Id("Description")] = jen.Lit(cmdDescription)
	}
	if cmdUsageText != "" {
		cmdDict[jen.Id("UsageText")] = jen.Lit(cmdUsageText)
	}
	if cmdArgsUsage != "" {
		cmdDict[jen.Id("ArgsUsage")] = jen.Lit(cmdArgsUsage)
	}

	// Generate the command with lifecycle hooks
	statements = append(statements,
		jen.Id("commands").Op("=").Append(
			jen.Id("commands"),
			jen.Op("&").Qual("github.com/urfave/cli/v3", "Command").Values(cmdDict),
		),
		jen.Line(),
	)

	return statements
}

func generateFlag(field *protogen.Field) jen.Code {
	flagName := toKebabCase(field.GoName)
	usage := field.GoName
	var shorthand string

	// Check for flag annotation
	flagOpts := getFieldFlagOptions(field)
	if flagOpts != nil {
		if flagOpts.Name != "" {
			flagName = flagOpts.Name
		}
		if flagOpts.Usage != "" {
			usage = flagOpts.Usage
		}
	}

	// Fallback to proto source comment if no annotation provided usage text
	if usage == field.GoName {
		if comment := cleanProtoComment(field.Comments.Leading); comment != "" {
			usage = firstLine(comment)
		}
	}

	if flagOpts != nil {
		if flagOpts.Shorthand != "" {
			shorthand = flagOpts.Shorthand
		}
	}

	// Helper function to build flag dict with optional Aliases, Required, DefaultText
	buildFlagDict := func() jen.Dict {
		dict := jen.Dict{
			jen.Id("Name"):  jen.Lit(flagName),
			jen.Id("Usage"): jen.Lit(usage),
		}
		if shorthand != "" {
			dict[jen.Id("Aliases")] = jen.Index().String().Values(jen.Lit(shorthand))
		}
		if flagOpts != nil && flagOpts.GetRequired() {
			dict[jen.Id("Required")] = jen.True()
		}
		if flagOpts != nil && flagOpts.GetPlaceholder() != "" {
			dict[jen.Id("DefaultText")] = jen.Lit(flagOpts.GetPlaceholder())
		}
		return dict
	}

	// defaultStr is the annotation default_value, used to set the flag's Value field.
	var defaultStr string
	if flagOpts != nil {
		defaultStr = flagOpts.GetDefaultValue()
	}

	// Handle repeated (list) fields — use slice flag types (no default Value for slices)
	if field.Desc.IsList() {
		if field.Desc.Kind() == protoreflect.EnumKind {
			usage = usage + " [" + getEnumValuesPiped(field.Enum) + "]"
		}
		if ft, ok := scalarFlagTypes[field.Desc.Kind()]; ok {
			return cliFlagRef(ft.SliceFlag, buildFlagDict())
		}
		if field.Desc.Kind() == protoreflect.MessageKind {
			messageType := field.Message
			fullyQualifiedName := string(messageType.Desc.FullName())
			if flagOpts == nil || flagOpts.Usage == "" {
				usage = fmt.Sprintf("%s (%s)", field.GoName, fullyQualifiedName)
			}
			return cliFlagRef("StringSliceFlag", buildFlagDict())
		}
		return nil
	}

	// Append valid enum values to usage text for singular enums
	if field.Desc.Kind() == protoreflect.EnumKind {
		usage = usage + " [" + getEnumValuesPiped(field.Enum) + "]"
	}
	if ft, ok := scalarFlagTypes[field.Desc.Kind()]; ok {
		dict := buildFlagDict()
		if dv := defaultValueCode(field.Desc.Kind(), defaultStr); dv != nil {
			dict[jen.Id("Value")] = dv
		}
		return cliFlagRef(ft.SingularFlag, dict)
	}

	switch field.Desc.Kind() {
	case protoreflect.MessageKind:
		// For message fields (e.g., google.protobuf.Timestamp, nested messages),
		// generate a StringFlag that custom deserializers can parse
		messageType := field.Message
		fullyQualifiedName := string(messageType.Desc.FullName())

		// Override usage for message types if not already customized
		if flagOpts == nil || flagOpts.Usage == "" {
			usage = fmt.Sprintf("%s (%s)", field.GoName, fullyQualifiedName)
		}

		dict := buildFlagDict()
		if defaultStr != "" {
			dict[jen.Id("Value")] = jen.Lit(defaultStr)
		}
		return jen.Op("&").Qual("github.com/urfave/cli/v3", "StringFlag").Values(dict)
	case protoreflect.GroupKind:
		// GroupKind is deprecated in proto3 and not supported
		fmt.Fprintf(os.Stderr, "WARNING: Field %s uses deprecated GroupKind and will not generate a CLI flag\n", field.Desc.FullName())
		return nil
	default:
		return nil
	}
}

// generateConfigFlags generates CLI flags for config message fields.
// cmdVarName is the sanitized command name for use in variable names (with hyphens replaced by underscores).
func generateConfigFlags(file *protogen.File, configMessageType string, cmdVarName string) []jen.Code {
	var statements []jen.Code

	if configMessageType == "" {
		return statements
	}

	// Find the config message in the file's messages
	var configMessage *protogen.Message
	for _, msg := range file.Messages {
		if msg.GoIdent.GoName == configMessageType {
			configMessage = msg
			break
		}
	}

	if configMessage == nil {
		return statements
	}

	statements = append(statements,
		jen.Line(),
		jen.Comment("Add config field flags for single-command mode"),
	)

	// Generate flags for each config field
	for _, field := range configMessage.Fields {
		// Get the cli.flag annotation if present
		flagOpts := getFieldFlagOptions(field)
		if flagOpts == nil {
			// No flag annotation, skip this field
			continue
		}

		flagName := flagOpts.Name
		if flagName == "" {
			flagName = toKebabCase(field.GoName)
		}

		usage := flagOpts.Usage
		if usage == "" {
			usage = field.GoName
		}

		// Fallback to proto source comment if no annotation provided usage text
		if usage == field.GoName {
			if comment := cleanProtoComment(field.Comments.Leading); comment != "" {
				usage = firstLine(comment)
			}
		}

		shorthand := flagOpts.Shorthand

		// Helper to build flag dict with optional Aliases, Required, DefaultText
		buildFlagDict := func() jen.Dict {
			dict := jen.Dict{
				jen.Id("Name"):  jen.Lit(flagName),
				jen.Id("Usage"): jen.Lit(usage),
			}
			if shorthand != "" {
				dict[jen.Id("Aliases")] = jen.Index().String().Values(jen.Lit(shorthand))
			}
			if flagOpts != nil && flagOpts.GetRequired() {
				dict[jen.Id("Required")] = jen.True()
			}
			if flagOpts != nil && flagOpts.GetPlaceholder() != "" {
				dict[jen.Id("DefaultText")] = jen.Lit(flagOpts.GetPlaceholder())
			}
			return dict
		}

		// Generate flag based on field type
		var flagCode jen.Code

		switch {
		case field.Desc.Kind() == protoreflect.MessageKind:
			// MessageKind requires custom deserializers, skip auto-generation
			continue
		case field.Desc.Kind() == protoreflect.GroupKind:
			// GroupKind is deprecated in proto3 and not supported
			fmt.Fprintf(os.Stderr, "WARNING: Field %s uses deprecated GroupKind and will not generate a CLI flag\n", field.Desc.FullName())
			continue
		default:
			if field.Desc.Kind() == protoreflect.EnumKind {
				usage = usage + " [" + getEnumValuesPiped(field.Enum) + "]"
			}
			if ft, ok := scalarFlagTypes[field.Desc.Kind()]; ok {
				if field.Desc.IsList() {
					flagCode = cliFlagRef(ft.SliceFlag, buildFlagDict())
				} else {
					flagCode = cliFlagRef(ft.SingularFlag, buildFlagDict())
				}
			}
		}

		if flagCode != nil {
			statements = append(statements,
				jen.Id("flags_"+cmdVarName).Op("=").Append(jen.Id("flags_"+cmdVarName), flagCode),
			)
		}
	}

	return statements
}

// generateRequestFieldAssignments generates code to assign flag values to request fields
// Handles both primitive types and nested messages (checking for custom deserializers)
//
//nolint:gocyclo,dupl,maintidx // Complexity comes from handling all proto kinds with optional field support
func generateRequestFieldAssignments(file *protogen.File, service *protogen.Service, method *protogen.Method) []jen.Code {
	var statements []jen.Code

	for _, field := range method.Input.Fields {
		flagName := toKebabCase(field.GoName)

		// Handle repeated (list) fields
		if field.Desc.IsList() {
			switch field.Desc.Kind() {
			case protoreflect.BoolKind:
				// No BoolSliceFlag — parse each string element with strconv.ParseBool
				statements = append(statements,
					jen.For(
						jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
					).Block(
						jen.List(jen.Id("v"), jen.Err()).Op(":=").Qual("strconv", "ParseBool").Call(jen.Id("s")),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid bool value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Append(jen.Id("req").Dot(field.GoName), jen.Id("v")),
					),
				)
			case protoreflect.BytesKind:
				// Convert each string element to []byte
				statements = append(statements,
					jen.For(
						jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
					).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Append(
							jen.Id("req").Dot(field.GoName),
							jen.Index().Byte().Call(jen.Id("s")),
						),
					),
				)
			case protoreflect.EnumKind:
				// Parse each string element via the generated enum parser
				enumTypeName := field.Enum.GoIdent.GoName
				parserFuncName := enumParserFuncName(service, enumTypeName)
				statements = append(statements,
					jen.For(
						jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
					).Block(
						jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(jen.Id("s")),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Append(jen.Id("req").Dot(field.GoName), jen.Id("val")),
					),
				)
			case protoreflect.MessageKind:
				// Repeated messages: iterate over each StringSlice element and deserialize
				messageType := field.Message
				fullyQualifiedName := string(messageType.Desc.FullName())
				goTypeName := messageType.GoIdent.GoName
				qualifiedType := qualifyType(file, messageType, true)

				statements = append(statements,
					jen.If(
						jen.List(jen.Id("fieldDeserializer"), jen.Id("hasFieldDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
							jen.Lit(fullyQualifiedName),
						),
						jen.Id("hasFieldDeserializer"),
					).Block(
						jen.For(
							jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
						).Block(
							jen.Id("elemFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewStringValueFlagContainer").Call(
								jen.Id("s"),
								jen.Id("cmd"),
							),
							jen.List(jen.Id("elemMsg"), jen.Id("elemErr")).Op(":=").Id("fieldDeserializer").Call(
								jen.Id("cmdCtx"),
								jen.Id("elemFlags"),
							),
							jen.If(jen.Id("elemErr").Op("!=").Nil()).Block(
								jen.Return(jen.Qual("fmt", "Errorf").Call(
									jen.Lit(fmt.Sprintf("failed to deserialize element for --%s: %%w", flagName)),
									jen.Id("elemErr"),
								)),
							),
							jen.If(jen.Id("elemMsg").Op("!=").Nil()).Block(
								jen.List(jen.Id("typedElem"), jen.Id("elemOk")).Op(":=").Id("elemMsg").Assert(qualifiedType),
								jen.If(jen.Op("!").Id("elemOk")).Block(
									jen.Return(jen.Qual("fmt", "Errorf").Call(
										jen.Lit(fmt.Sprintf("custom deserializer for %s returned wrong type: expected *%s, got %%T", fullyQualifiedName, goTypeName)),
										jen.Id("elemMsg"),
									)),
								),
								jen.Id("req").Dot(field.GoName).Op("=").Append(
									jen.Id("req").Dot(field.GoName),
									jen.Id("typedElem"),
								),
							),
						),
					).Else().Block(
						jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("flag --%s requires a custom deserializer for %s (register with protocli.WithFlagDeserializer)", flagName, fullyQualifiedName)),
							)),
						),
					),
				)
				continue
			default:
				// Direct slice assignment for numeric and string types
				if ft, ok := scalarFlagTypes[field.Desc.Kind()]; ok {
					statements = append(statements,
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot(ft.SliceAccessor).Call(jen.Lit(flagName)),
					)
				}
			}
			continue
		}

		switch field.Desc.Kind() {
		case protoreflect.MessageKind:
			// For message fields, check if there's a custom deserializer
			// Use fully qualified proto name
			messageType := field.Message
			fullyQualifiedName := string(messageType.Desc.FullName())
			goTypeName := messageType.GoIdent.GoName
			qualifiedType := qualifyType(file, messageType, true)

			statements = append(statements,
				jen.Comment(fmt.Sprintf("Field %s: check for custom deserializer for %s", field.GoName, fullyQualifiedName)),
				jen.If(
					jen.List(jen.Id("fieldDeserializer"), jen.Id("hasFieldDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
						jen.Lit(fullyQualifiedName),
					),
					jen.Id("hasFieldDeserializer"),
				).Block(
					jen.Comment("Use custom deserializer for nested message"),
					jen.Comment(fmt.Sprintf("Create FlagContainer for field flag: %s", flagName)),
					jen.Id("fieldFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewFlagContainer").Call(
						jen.Id("cmd"),
						jen.Lit(flagName),
					),
					jen.List(jen.Id("fieldMsg"), jen.Id("fieldErr")).Op(":=").Id("fieldDeserializer").Call(
						jen.Id("cmdCtx"),
						jen.Id("fieldFlags"),
					),
					jen.If(jen.Id("fieldErr").Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("failed to deserialize field %s: %%w", field.GoName)),
							jen.Id("fieldErr"),
						)),
					),
					jen.Comment("Handle nil return from deserializer (means skip/use default)"),
					jen.If(jen.Id("fieldMsg").Op("!=").Nil()).Block(
						jen.List(jen.Id("typedField"), jen.Id("fieldOk")).Op(":=").Id("fieldMsg").Assert(qualifiedType),
						jen.If(jen.Op("!").Id("fieldOk")).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("custom deserializer for %s returned wrong type: expected *%s, got %%T", fullyQualifiedName, goTypeName)),
								jen.Id("fieldMsg"),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Id("typedField"),
					),
				).Else().Block(
					jen.Comment("No custom deserializer - check if user provided a value"),
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("flag --%s requires a custom deserializer for %s (register with protocli.WithFlagDeserializer)", flagName, fullyQualifiedName)),
						)),
					),
					jen.Comment("No value provided - leave field as nil"),
				),
			)

		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
			// Check if field is optional (has synthetic oneof for presence tracking)
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				// Optional field - only set if flag was provided
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Int32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				// Regular field - always set
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Int32").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Int64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Int64").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Uint32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Uint32").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Uint64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Uint64").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.FloatKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Float32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Float32").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.DoubleKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Float64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Float64").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.StringKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("String").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.BoolKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Bool").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Bool").Call(jen.Lit(flagName)),
				)
			}
		case protoreflect.BytesKind:
			// Bytes fields don't have explicit presence in proto3, always set
			statements = append(statements,
				jen.Id("req").Dot(field.GoName).Op("=").Index().Byte().Call(
					jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
				),
			)
		case protoreflect.EnumKind:
			// Parse enum from string using generated parser
			enumTypeName := field.Enum.GoIdent.GoName
			parserFuncName := enumParserFuncName(service, enumTypeName)
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))

			if isOptional {
				// Optional enum field - only set if flag was provided
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(
							jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				// Regular enum field - always parse if provided, otherwise use zero value
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(
							jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Id("val"),
					),
				)
			}
		case protoreflect.GroupKind:
			// GroupKind is deprecated and not supported - generate runtime error
			fmt.Fprintf(os.Stderr, "WARNING: Field %s uses deprecated GroupKind - generating code that will return a runtime error\n", field.Desc.FullName())
			statements = append(statements,
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("field %s uses deprecated proto2 GroupKind which is not supported - please update your proto definition to use a message type instead", field.GoName)),
				)),
			)
		}
	}

	return statements
}

// generateRequestFieldOverrides generates code to override request fields from CLI flags,
// but ONLY when the flag was explicitly set (cmd.IsSet). This is used when a request
// was loaded from an input file and flags should selectively override fields.
//
//nolint:gocyclo,dupl,maintidx // Complexity comes from handling all proto kinds with IsSet checks
func generateRequestFieldOverrides(file *protogen.File, service *protogen.Service, method *protogen.Method) []jen.Code {
	var statements []jen.Code

	for _, field := range method.Input.Fields {
		flagName := toKebabCase(field.GoName)

		// Handle repeated (list) fields — only override if flag was explicitly set
		if field.Desc.IsList() {
			switch field.Desc.Kind() {
			case protoreflect.BoolKind:
				// No BoolSliceFlag — parse each string element with strconv.ParseBool
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Nil(),
						jen.For(
							jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
						).Block(
							jen.List(jen.Id("v"), jen.Err()).Op(":=").Qual("strconv", "ParseBool").Call(jen.Id("s")),
							jen.If(jen.Err().Op("!=").Nil()).Block(
								jen.Return(jen.Qual("fmt", "Errorf").Call(
									jen.Lit(fmt.Sprintf("invalid bool value for --%s: %%w", flagName)),
									jen.Err(),
								)),
							),
							jen.Id("req").Dot(field.GoName).Op("=").Append(jen.Id("req").Dot(field.GoName), jen.Id("v")),
						),
					),
				)
			case protoreflect.BytesKind:
				// Convert each string element to []byte
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Nil(),
						jen.For(
							jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
						).Block(
							jen.Id("req").Dot(field.GoName).Op("=").Append(
								jen.Id("req").Dot(field.GoName),
								jen.Index().Byte().Call(jen.Id("s")),
							),
						),
					),
				)
			case protoreflect.EnumKind:
				// Parse each string element via the generated enum parser
				enumTypeName := field.Enum.GoIdent.GoName
				parserFuncName := enumParserFuncName(service, enumTypeName)
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Nil(),
						jen.For(
							jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
						).Block(
							jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(jen.Id("s")),
							jen.If(jen.Err().Op("!=").Nil()).Block(
								jen.Return(jen.Qual("fmt", "Errorf").Call(
									jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
									jen.Err(),
								)),
							),
							jen.Id("req").Dot(field.GoName).Op("=").Append(jen.Id("req").Dot(field.GoName), jen.Id("val")),
						),
					),
				)
			case protoreflect.MessageKind:
				// Repeated messages: iterate over each StringSlice element and deserialize
				messageType := field.Message
				fullyQualifiedName := string(messageType.Desc.FullName())
				goTypeName := messageType.GoIdent.GoName
				qualifiedType := qualifyType(file, messageType, true)

				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.If(
							jen.List(jen.Id("fieldDeserializer"), jen.Id("hasFieldDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
								jen.Lit(fullyQualifiedName),
							),
							jen.Id("hasFieldDeserializer"),
						).Block(
							jen.Id("req").Dot(field.GoName).Op("=").Nil(),
							jen.For(
								jen.List(jen.Id("_"), jen.Id("s")).Op(":=").Range().Id("cmd").Dot("StringSlice").Call(jen.Lit(flagName)),
							).Block(
								jen.Id("elemFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewStringValueFlagContainer").Call(
									jen.Id("s"),
									jen.Id("cmd"),
								),
								jen.List(jen.Id("elemMsg"), jen.Id("elemErr")).Op(":=").Id("fieldDeserializer").Call(
									jen.Id("cmdCtx"),
									jen.Id("elemFlags"),
								),
								jen.If(jen.Id("elemErr").Op("!=").Nil()).Block(
									jen.Return(jen.Qual("fmt", "Errorf").Call(
										jen.Lit(fmt.Sprintf("failed to deserialize element for --%s: %%w", flagName)),
										jen.Id("elemErr"),
									)),
								),
								jen.If(jen.Id("elemMsg").Op("!=").Nil()).Block(
									jen.List(jen.Id("typedElem"), jen.Id("elemOk")).Op(":=").Id("elemMsg").Assert(qualifiedType),
									jen.If(jen.Op("!").Id("elemOk")).Block(
										jen.Return(jen.Qual("fmt", "Errorf").Call(
											jen.Lit(fmt.Sprintf("custom deserializer for %s returned wrong type: expected *%s, got %%T", fullyQualifiedName, goTypeName)),
											jen.Id("elemMsg"),
										)),
									),
									jen.Id("req").Dot(field.GoName).Op("=").Append(
										jen.Id("req").Dot(field.GoName),
										jen.Id("typedElem"),
									),
								),
							),
						).Else().Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("flag --%s requires a custom deserializer for %s (register with protocli.WithFlagDeserializer)", flagName, fullyQualifiedName)),
							)),
						),
					),
				)
				continue
			default:
				// Direct slice assignment for numeric and string types
				if ft, ok := scalarFlagTypes[field.Desc.Kind()]; ok {
					statements = append(statements,
						jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
							jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot(ft.SliceAccessor).Call(jen.Lit(flagName)),
						),
					)
				}
			}
			continue
		}

		switch field.Desc.Kind() {
		case protoreflect.MessageKind:
			// For message fields, check if there's a custom deserializer
			messageType := field.Message
			fullyQualifiedName := string(messageType.Desc.FullName())
			goTypeName := messageType.GoIdent.GoName
			qualifiedType := qualifyType(file, messageType, true)

			statements = append(statements,
				jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
					jen.If(
						jen.List(jen.Id("fieldDeserializer"), jen.Id("hasFieldDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
							jen.Lit(fullyQualifiedName),
						),
						jen.Id("hasFieldDeserializer"),
					).Block(
						jen.Id("fieldFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewFlagContainer").Call(
							jen.Id("cmd"),
							jen.Lit(flagName),
						),
						jen.List(jen.Id("fieldMsg"), jen.Id("fieldErr")).Op(":=").Id("fieldDeserializer").Call(
							jen.Id("cmdCtx"),
							jen.Id("fieldFlags"),
						),
						jen.If(jen.Id("fieldErr").Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("failed to deserialize field %s: %%w", field.GoName)),
								jen.Id("fieldErr"),
							)),
						),
						jen.If(jen.Id("fieldMsg").Op("!=").Nil()).Block(
							jen.List(jen.Id("typedField"), jen.Id("fieldOk")).Op(":=").Id("fieldMsg").Assert(qualifiedType),
							jen.If(jen.Op("!").Id("fieldOk")).Block(
								jen.Return(jen.Qual("fmt", "Errorf").Call(
									jen.Lit(fmt.Sprintf("custom deserializer for %s returned wrong type: expected *%s, got %%T", fullyQualifiedName, goTypeName)),
									jen.Id("fieldMsg"),
								)),
							),
							jen.Id("req").Dot(field.GoName).Op("=").Id("typedField"),
						),
					).Else().Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("flag --%s requires a custom deserializer for %s (register with protocli.WithFlagDeserializer)", flagName, fullyQualifiedName)),
						)),
					),
				),
			)

		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Int32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Int32").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Int64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Int64").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Uint32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Uint32").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Uint64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Uint64").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.FloatKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Float32").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Float32").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.DoubleKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Float64").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Float64").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.StringKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("String").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.BoolKind:
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))
			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("val").Op(":=").Id("cmd").Dot("Bool").Call(jen.Lit(flagName)),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.Id("req").Dot(field.GoName).Op("=").Id("cmd").Dot("Bool").Call(jen.Lit(flagName)),
					),
				)
			}
		case protoreflect.BytesKind:
			statements = append(statements,
				jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
					jen.Id("req").Dot(field.GoName).Op("=").Index().Byte().Call(
						jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
					),
				),
			)
		case protoreflect.EnumKind:
			enumTypeName := field.Enum.GoIdent.GoName
			parserFuncName := enumParserFuncName(service, enumTypeName)
			oneof := field.Desc.ContainingOneof()
			isOptional := field.Desc.HasPresence() && (oneof == nil || (oneof != nil && oneof.IsSynthetic()))

			if isOptional {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(
							jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Op("&").Id("val"),
					),
				)
			} else {
				statements = append(statements,
					jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
						jen.List(jen.Id("val"), jen.Err()).Op(":=").Id(parserFuncName).Call(
							jen.Id("cmd").Dot("String").Call(jen.Lit(flagName)),
						),
						jen.If(jen.Err().Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("invalid value for --%s: %%w", flagName)),
								jen.Err(),
							)),
						),
						jen.Id("req").Dot(field.GoName).Op("=").Id("val"),
					),
				)
			}
		case protoreflect.GroupKind:
			// GroupKind is deprecated and not supported
		}
	}

	return statements
}

// generateOutputWriterOpening generates code to open the output writer and set up cleanup
func generateOutputWriterOpening(service *protogen.Service) []jen.Code {
	return []jen.Code{
		jen.Comment("Open output writer"),
		jen.List(jen.Id("outputWriter"), jen.Err()).Op(":=").Id(outputWriterFuncName(service)).Call(
			jen.Id("cmd"),
			jen.Id("cmd").Dot("String").Call(jen.Lit("output")),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to open output: %w"), jen.Err())),
		),
		jen.If(jen.Id("closer").Op(",").Id("ok").Op(":=").Id("outputWriter").Assert(jen.Qual("io", "Closer")), jen.Id("ok")).Block(
			jen.Defer().Id("closer").Dot("Close").Call(),
		),
		jen.Line(),
	}
}

func generateActionBodyWithHooks(file *protogen.File, service *protogen.Service, method *protogen.Method, configMessageType string, localOnly bool) []jen.Code {
	var statements []jen.Code

	// Reject extra positional arguments.
	statements = append(statements,
		jen.If(
			jen.Id("cmd").Dot("Args").Call().Dot("Len").Call().Op(">").Lit(0),
		).Block(
			jen.Return(
				jen.Qual("github.com/urfave/cli/v3", "Exit").Call(
					jen.Qual("fmt", "Sprintf").Call(
						jen.Lit("unsupported argument: %q"),
						jen.Id("cmd").Dot("Args").Call().Dot("Get").Call(jen.Lit(0)),
					),
					jen.Lit(3),
				),
			),
		),
		jen.Line(),
	)

	// Defer after hooks in reverse order (LIFO)
	// IMPORTANT: Register defer FIRST so it runs even if before hooks fail
	statements = append(statements,
		generateAfterHooksDefer(),
		jen.Line(),
	)

	// Call before hooks in order (FIFO)
	statements = append(statements,
		jen.For(
			jen.List(jen.Id("_"), jen.Id("hook")).Op(":=").Range().Id("options").Dot("BeforeCommandHooks").Call(),
		).Block(
			jen.If(
				jen.Err().Op(":=").Id("hook").Call(
					jen.Id("cmdCtx"),
					jen.Id("cmd"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit("before hook failed: %w"),
					jen.Err(),
				)),
			),
		),
		jen.Line(),
	)

	// Build request - check for input file, custom deserializer, or auto-generated flags
	requestFullyQualifiedName := string(method.Input.Desc.FullName())
	requestTypeName := method.Input.GoIdent.GoName
	requestQualifiedType := qualifyType(file, method.Input, true)

	statements = append(statements,
		jen.Comment("Build request message"),
		jen.Var().Id("req").Add(requestQualifiedType),
		jen.Line(),
	)

	// Generate the if-else block: input-file → custom deserializer → auto-generated
	requestBuildBlock := []jen.Code{
		jen.Comment("Check for file-based input"),
		jen.Id("inputFile").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("input-file")),
		jen.If(jen.Id("inputFile").Op("!=").Lit("")).Block(
			append([]jen.Code{
				jen.Comment("Read request from file"),
				jen.Id("req").Op("=").Op("&").Add(qualifyType(file, method.Input, false)).Values(),
				jen.If(
					jen.Err().Op(":=").Qual("github.com/drewfead/proto-cli", "ReadInputFile").Call(
						jen.Id("inputFile"),
						jen.Id("cmd").Dot("String").Call(jen.Lit("input-format")),
						jen.Id("options").Dot("InputFormats").Call(),
						jen.Id("req"),
					),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Err()),
				),
				jen.Comment("Apply flag overrides (only explicitly-set flags)"),
			}, generateRequestFieldOverrides(file, service, method)...)...,
		).Else().Block(
			// Inner if-else: custom deserializer vs auto-generated
			jen.Comment(fmt.Sprintf("Check for custom flag deserializer for %s", requestFullyQualifiedName)),
			jen.List(jen.Id("deserializer"), jen.Id("hasDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
				jen.Lit(requestFullyQualifiedName),
			),
			jen.If(jen.Id("hasDeserializer")).Block(
				jen.Comment("Use custom deserializer for top-level request"),
				jen.Comment("Create FlagContainer (deserializer can access multiple flags via Command())"),
				jen.Id("requestFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewFlagContainer").Call(
					jen.Id("cmd"),
					jen.Lit(""), // Empty flag name for top-level requests
				),
				jen.List(jen.Id("msg"), jen.Err()).Op(":=").Id("deserializer").Call(
					jen.Id("cmdCtx"),
					jen.Id("requestFlags"),
				),
				jen.If(jen.Err().Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit("custom deserializer failed: %w"),
						jen.Err(),
					)),
				),
				jen.Comment("Handle nil return from deserializer"),
				jen.If(jen.Id("msg").Op("==").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit("custom deserializer returned nil message"),
					)),
				),
				jen.Var().Id("ok").Bool(),
				jen.List(jen.Id("req"), jen.Id("ok")).Op("=").Id("msg").Assert(requestQualifiedType),
				jen.If(jen.Op("!").Id("ok")).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit("custom deserializer returned wrong type: expected *%s, got %T"),
						jen.Lit(requestTypeName),
						jen.Id("msg"),
					)),
				),
			).Else().Block(
				append([]jen.Code{
					jen.Comment("Use auto-generated flag parsing"),
					jen.Id("req").Op("=").Op("&").Add(qualifyType(file, method.Input, false)).Values(),
				}, generateRequestFieldAssignments(file, service, method)...)...,
			),
		),
		jen.Line(),
	}

	statements = append(statements, requestBuildBlock...)

	// Generate remote/local call logic
	if localOnly {
		// Local-only command: always use direct implementation call
		statements = append(statements,
			jen.Comment("Local-only command: always use direct implementation call"),
			jen.Var().Id("resp").Op("*").Id(method.Output.GoIdent.GoName),
			jen.Var().Err().Error(),
			jen.Line(),
		)
		statements = append(statements, generateLocalCallLogic(service, method, configMessageType)...)
		statements = append(statements, jen.Line())
	} else {
		// Check if remote flag is set and call either remote or direct
		clientType := "New" + service.GoName + "Client"
		statements = append(statements,
			jen.Comment("Check if using remote gRPC call or direct implementation call"),
			jen.Id("remoteAddr").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("remote")),
			jen.Var().Id("resp").Op("*").Id(method.Output.GoIdent.GoName),
			jen.Var().Err().Error(),
			jen.Line(),
			jen.If(jen.Id("remoteAddr").Op("!=").Lit("")).Block(
				jen.Comment("Remote gRPC call"),
				jen.List(jen.Id("conn"), jen.Id("connErr")).Op(":=").Qual("google.golang.org/grpc", "NewClient").Call(
					jen.Id("remoteAddr"),
					jen.Qual("google.golang.org/grpc", "WithTransportCredentials").Call(
						jen.Qual("google.golang.org/grpc/credentials/insecure", "NewCredentials").Call(),
					),
				),
				jen.If(jen.Id("connErr").Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit("failed to connect to remote %s: %w"),
						jen.Id("remoteAddr"),
						jen.Id("connErr"),
					)),
				),
				jen.Defer().Id("conn").Dot("Close").Call(),
				jen.Line(),
				jen.Id("client").Op(":=").Id(clientType).Call(jen.Id("conn")),
				jen.List(jen.Id("resp"), jen.Err()).Op("=").Id("client").Dot(method.GoName).Call(
					jen.Id("cmdCtx"),
					jen.Id("req"),
				),
				jen.If(jen.Err().Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("remote call failed: %w"), jen.Err())),
				),
			).Else().Block(
				generateLocalCallLogic(service, method, configMessageType)...,
			),
			jen.Line(),
		)
	}

	// Handle output formatting
	statements = append(statements, generateOutputWriterOpening(service)...)

	statements = append(statements,
		jen.Comment("Find and use the appropriate output format"),
		jen.Id("formatName").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("format")),
		jen.Line(),
		jen.Comment("Try registered formats"),
		jen.For(
			jen.List(jen.Id("_"), jen.Id("outputFmt")).Op(":=").Range().Id("options").Dot("OutputFormats").Call(),
		).Block(
			jen.If(jen.Id("outputFmt").Dot("Name").Call().Op("==").Id("formatName")).Block(
				jen.If(
					jen.Err().Op(":=").Id("outputFmt").Dot("Format").Call(
						jen.Id("cmdCtx"),
						jen.Id("cmd"),
						jen.Id("outputWriter"),
						jen.Id("resp"),
					),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("format failed: %w"), jen.Err())),
				),
				jen.Comment("Write final newline to keep terminal clean"),
				jen.If(
					jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("outputWriter").Dot("Write").Call(
						jen.Index().Byte().Call(jen.Lit("\n")),
					),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to write final newline: %w"), jen.Err())),
				),
				jen.Return(jen.Nil()),
			),
		),
		jen.Line(),
		jen.Comment("Format not found - build list of available formats"),
		jen.Var().Id("availableFormats").Index().String(),
		jen.For(
			jen.List(jen.Id("_"), jen.Id("f")).Op(":=").Range().Id("options").Dot("OutputFormats").Call(),
		).Block(
			jen.Id("availableFormats").Op("=").Append(
				jen.Id("availableFormats"),
				jen.Id("f").Dot("Name").Call(),
			),
		),
		jen.If(jen.Len(jen.Id("availableFormats")).Op("==").Lit(0)).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(
				jen.Lit("no output formats registered (use WithOutputFormats to register formats)"),
			)),
		),
		jen.Return(jen.Qual("fmt", "Errorf").Call(
			jen.Lit("unknown format %q (available: %v)"),
			jen.Id("formatName"),
			jen.Id("availableFormats"),
		)),
	)

	return statements
}

// generateLocalCallLogic generates the logic for calling the service implementation locally
func generateLocalCallLogic(service *protogen.Service, method *protogen.Method, configMessageType string) []jen.Code {
	var statements []jen.Code

	if configMessageType != "" {
		// Service has config - need to load it and call factory
		statements = append(statements,
			jen.Comment("Load config and create service implementation"),
			jen.Comment("Get config paths and env prefix from root command"),
			jen.Id("rootCmd").Op(":=").Id("cmd").Dot("Root").Call(),
			jen.Id("configPaths").Op(":=").Id("rootCmd").Dot("StringSlice").Call(jen.Lit("config")),
			jen.Id("envPrefix").Op(":=").Id("rootCmd").Dot("String").Call(jen.Lit("env-prefix")),
			jen.Line(),
			jen.Comment("Create config loader (single-command mode = uses files + env + flags)"),
			jen.Id("loader").Op(":=").Qual("github.com/drewfead/proto-cli", "NewConfigLoader").Call(
				jen.Qual("github.com/drewfead/proto-cli", "SingleCommandMode"),
				jen.Qual("github.com/drewfead/proto-cli", "FileConfig").Call(jen.Id("configPaths").Op("...")),
				jen.Qual("github.com/drewfead/proto-cli", "EnvPrefix").Call(jen.Id("envPrefix")),
			),
			jen.Line(),
			jen.Comment("Create config instance and load configuration"),
			jen.Id("config").Op(":=").Op("&").Id(configMessageType).Values(),
			jen.If(
				jen.Err().Op(":=").Id("loader").Dot("LoadServiceConfig").Call(
					jen.Id("cmd"),
					jen.Lit(strings.ToLower(service.GoName)),
					jen.Id("config"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to load config: %w"), jen.Err())),
			),
			jen.Line(),
			jen.Comment("Call factory to create service implementation"),
			jen.List(jen.Id("svcImpl"), jen.Err()).Op(":=").Qual("github.com/drewfead/proto-cli", "CallFactory").Call(
				jen.Id("implOrFactory"),
				jen.Id("config"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to create service: %w"), jen.Err())),
			),
			jen.Line(),
			jen.Comment("Call the RPC method"),
			jen.List(jen.Id("resp"), jen.Err()).Op("=").Id("svcImpl").Assert(jen.Id(service.GoName+"Server")).Dot(method.GoName).Call(
				jen.Id("cmdCtx"),
				jen.Id("req"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("method failed: %w"), jen.Err())),
			),
		)
	} else {
		// No config - direct implementation call
		statements = append(statements,
			jen.Comment("Direct implementation call (no config)"),
			jen.Id("svcImpl").Op(":=").Id("implOrFactory").Assert(jen.Id(service.GoName+"Server")),
			jen.List(jen.Id("resp"), jen.Err()).Op("=").Id("svcImpl").Dot(method.GoName).Call(
				jen.Id("cmdCtx"),
				jen.Id("req"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("method failed: %w"), jen.Err())),
			),
		)
	}

	return statements
}

// generateTUIBeforeHook returns a jen func literal for the Before hook that
// intercepts --interactive, collects explicitly-set flag values into a prefill
// map, and calls InvokeTUI with StartAtMethod + WithPrefillFields.
func generateTUIBeforeHook(method *protogen.Method, serviceCLIName, cmdName string) jen.Code {
	// Build the statements that populate the prefill map.
	var prefillStmts []jen.Code
	prefillStmts = append(prefillStmts,
		jen.Id("prefill").Op(":=").Map(jen.String()).String().Values(),
	)
	for _, field := range method.Input.Fields {
		if field.Desc.IsList() {
			continue // skip repeated fields; Appender handles multi-value inputs
		}
		flagName := toKebabCase(field.GoName)
		flagOpts := getFieldFlagOptions(field)
		if flagOpts != nil && flagOpts.Name != "" {
			flagName = flagOpts.Name
		}
		valExpr := flagToStringExpr(field)
		if valExpr == nil {
			continue
		}
		prefillStmts = append(prefillStmts,
			jen.If(jen.Id("cmd").Dot("IsSet").Call(jen.Lit(flagName))).Block(
				jen.Id("prefill").Index(jen.Lit(flagName)).Op("=").Add(valExpr),
			),
		)
	}

	// Build the InvokeTUI call arguments.
	invokeArgs := []jen.Code{
		jen.Id("ctx"),
		jen.Id("cmd"),
		jen.Qual("github.com/drewfead/proto-cli", "StartAtMethod").Call(
			jen.Lit(serviceCLIName),
			jen.Lit(cmdName),
		),
		jen.Qual("github.com/drewfead/proto-cli", "WithPrefillFields").Call(jen.Id("prefill")),
	}

	interactiveBlock := append(
		prefillStmts,
		jen.If(
			jen.Err().Op(":=").Qual("github.com/drewfead/proto-cli", "InvokeTUI").Call(invokeArgs...),
			jen.Err().Op("!=").Nil(),
		).Block(jen.Return(jen.Id("ctx"), jen.Err())),
		jen.Return(jen.Id("ctx"), jen.Qual("github.com/urfave/cli/v3", "Exit").Call(jen.Lit(""), jen.Lit(0))),
	)

	argsCheck := jen.If(
		jen.Id("cmd").Dot("Args").Call().Dot("Len").Call().Op(">").Lit(0),
	).Block(
		jen.Return(
			jen.Id("ctx"),
			jen.Qual("github.com/urfave/cli/v3", "Exit").Call(
				jen.Qual("fmt", "Sprintf").Call(
					jen.Lit("unsupported argument: %q"),
					jen.Id("cmd").Dot("Args").Call().Dot("Get").Call(jen.Lit(0)),
				),
				jen.Lit(3),
			),
		),
	)

	return jen.Func().Params(
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
	).Params(jen.Qual("context", "Context"), jen.Error()).Block(
		argsCheck,
		jen.If(jen.Id("cmd").Dot("Bool").Call(jen.Lit("interactive"))).Block(interactiveBlock...),
		jen.Return(jen.Id("ctx"), jen.Nil()),
	)
}

// flagToStringExpr returns a jen expression that converts a (non-list) field's CLI
// flag value to a string suitable for TUI form prefill. Returns nil for unsupported kinds.
// Message, string, enum, and bytes fields are read directly as strings from their StringFlag.
func flagToStringExpr(field *protogen.Field) jen.Code {
	flagName := toKebabCase(field.GoName)
	flagOpts := getFieldFlagOptions(field)
	if flagOpts != nil && flagOpts.Name != "" {
		flagName = flagOpts.Name
	}

	switch field.Desc.Kind() {
	case protoreflect.StringKind, protoreflect.EnumKind, protoreflect.MessageKind, protoreflect.BytesKind:
		return jen.Id("cmd").Dot("String").Call(jen.Lit(flagName))
	case protoreflect.BoolKind:
		return jen.Qual("strconv", "FormatBool").Call(jen.Id("cmd").Dot("Bool").Call(jen.Lit(flagName)))
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return jen.Qual("strconv", "FormatInt").Call(
			jen.Int64().Call(jen.Id("cmd").Dot("Int32").Call(jen.Lit(flagName))),
			jen.Lit(10),
		)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return jen.Qual("strconv", "FormatInt").Call(
			jen.Id("cmd").Dot("Int64").Call(jen.Lit(flagName)),
			jen.Lit(10),
		)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return jen.Qual("strconv", "FormatUint").Call(
			jen.Uint64().Call(jen.Id("cmd").Dot("Uint32").Call(jen.Lit(flagName))),
			jen.Lit(10),
		)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return jen.Qual("strconv", "FormatUint").Call(
			jen.Id("cmd").Dot("Uint64").Call(jen.Lit(flagName)),
			jen.Lit(10),
		)
	case protoreflect.FloatKind:
		return jen.Qual("strconv", "FormatFloat").Call(
			jen.Float64().Call(jen.Id("cmd").Dot("Float32").Call(jen.Lit(flagName))),
			jen.LitRune('g'),
			jen.Lit(-1),
			jen.Lit(32),
		)
	case protoreflect.DoubleKind:
		return jen.Qual("strconv", "FormatFloat").Call(
			jen.Id("cmd").Dot("Float64").Call(jen.Lit(flagName)),
			jen.LitRune('g'),
			jen.Lit(-1),
			jen.Lit(64),
		)
	default:
		return nil
	}
}
