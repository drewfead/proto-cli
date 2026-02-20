package generate

import (
	"fmt"
	"strings"

	"github.com/dave/jennifer/jen"
	"google.golang.org/protobuf/compiler/protogen"
)

// generateServerStreamingCommand generates a CLI command for server streaming RPC methods
func generateServerStreamingCommand(service *protogen.Service, method *protogen.Method, configMessageType string, file *protogen.File) []jen.Code {
	var statements []jen.Code

	// Get command name and help fields from annotation or use defaults
	cmdName := toKebabCase(method.GoName)
	cmdUsage := method.GoName + " (streaming)"            // Short description for Usage field
	var cmdDescription, cmdUsageText, cmdArgsUsage string // Long description, custom usage line, args

	var localOnly bool

	cmdOpts := getMethodCommandOptions(method)
	if cmdOpts != nil {
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
	defaultStreamingUsage := method.GoName + " (streaming)"
	if cmdUsage == defaultStreamingUsage {
		if comment := cleanProtoComment(method.Comments.Leading); comment != "" {
			cmdUsage = firstLine(comment)
		}
	}

	// Create a safe variable name (replace hyphens with underscores)
	cmdVarName := strings.ReplaceAll(cmdName, "-", "_")

	// Build flags dynamically with output format support and delimiter
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
			jen.Id("Name"):  jen.Lit("delimiter"),
			jen.Id("Value"): jen.Lit("\n"),
			jen.Id("Usage"): jen.Lit("Delimiter between streamed messages"),
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

	// Build command dict with help fields
	cmdDict := jen.Dict{
		jen.Id("Name"):  jen.Lit(cmdName),
		jen.Id("Usage"): jen.Lit(cmdUsage),
		jen.Id("Flags"): jen.Id("flags_" + cmdVarName),
		jen.Id("Action"): jen.Func().Params(
			jen.Id("cmdCtx").Qual("context", "Context"),
			jen.Id("cmd").Op("*").Qual("github.com/urfave/cli/v3", "Command"),
		).Error().Block(
			generateServerStreamingActionBody(file, service, method, configMessageType, localOnly)...,
		),
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

	// Generate the command with streaming action
	statements = append(statements,
		jen.Id("commands").Op("=").Append(
			jen.Id("commands"),
			jen.Op("&").Qual("github.com/urfave/cli/v3", "Command").Values(cmdDict),
		),
		jen.Line(),
	)

	return statements
}

// generateServerStreamingActionBody generates the action body for server streaming commands
func generateServerStreamingActionBody(file *protogen.File, service *protogen.Service, method *protogen.Method, configMessageType string, localOnly bool) []jen.Code {
	var statements []jen.Code

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

	// Build request (same as unary)
	requestFullyQualifiedName := string(method.Input.Desc.FullName())
	requestTypeName := method.Input.GoIdent.GoName
	requestQualifiedType := qualifyType(file, method.Input, true)

	statements = append(statements,
		jen.Comment("Build request message"),
		jen.Var().Id("req").Add(requestQualifiedType),
		jen.Line(),
	)

	// Generate the if-else block for custom deserializer vs auto-generated
	deserializerCheck := []jen.Code{
		jen.Comment(fmt.Sprintf("Check for custom flag deserializer for %s", requestFullyQualifiedName)),
		jen.List(jen.Id("deserializer"), jen.Id("hasDeserializer")).Op(":=").Id("options").Dot("FlagDeserializer").Call(
			jen.Lit(requestFullyQualifiedName),
		),
		jen.If(jen.Id("hasDeserializer")).Block(
			jen.Comment("Use custom deserializer for top-level request"),
			jen.Id("requestFlags").Op(":=").Qual("github.com/drewfead/proto-cli", "NewFlagContainer").Call(
				jen.Id("cmd"),
				jen.Lit(""),
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
		jen.Line(),
	}

	statements = append(statements, deserializerCheck...)

	// Open output writer
	statements = append(statements, generateOutputWriterOpening(service)...)

	// Find output format
	statements = append(statements,
		jen.Comment("Find the appropriate output format"),
		jen.Id("formatName").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("format")),
		jen.Var().Id("outputFmt").Qual("github.com/drewfead/proto-cli", "OutputFormat"),
		jen.For(
			jen.List(jen.Id("_"), jen.Id("f")).Op(":=").Range().Id("options").Dot("OutputFormats").Call(),
		).Block(
			jen.If(jen.Id("f").Dot("Name").Call().Op("==").Id("formatName")).Block(
				jen.Id("outputFmt").Op("=").Id("f"),
				jen.Break(),
			),
		),
		jen.If(jen.Id("outputFmt").Op("==").Nil()).Block(
			jen.Var().Id("availableFormats").Index().String(),
			jen.For(
				jen.List(jen.Id("_"), jen.Id("f")).Op(":=").Range().Id("options").Dot("OutputFormats").Call(),
			).Block(
				jen.Id("availableFormats").Op("=").Append(
					jen.Id("availableFormats"),
					jen.Id("f").Dot("Name").Call(),
				),
			),
			jen.Return(jen.Qual("fmt", "Errorf").Call(
				jen.Lit("unknown format %q (available: %v)"),
				jen.Id("formatName"),
				jen.Id("availableFormats"),
			)),
		),
		jen.Line(),
	)

	// Get delimiter
	statements = append(statements,
		jen.Comment("Get delimiter for separating streamed messages"),
		jen.Id("delimiter").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("delimiter")),
		jen.Line(),
	)

	// Generate remote/local streaming call logic
	if localOnly {
		// Local-only command: always use direct implementation call
		statements = append(statements,
			jen.Comment("Local-only command: always use direct implementation call"),
		)
		statements = append(statements, generateLocalStreamingCall(service, method, configMessageType)...)
		statements = append(statements,
			jen.Line(),
			jen.Return(jen.Nil()),
		)
	} else {
		// Check if remote or local call
		clientType := "New" + service.GoName + "Client"
		statements = append(statements,
			jen.Comment("Check if using remote gRPC call or direct implementation call"),
			jen.Id("remoteAddr").Op(":=").Id("cmd").Dot("String").Call(jen.Lit("remote")),
			jen.Line(),
			jen.If(jen.Id("remoteAddr").Op("!=").Lit("")).Block(
				generateRemoteStreamingCall(service, method, clientType)...,
			).Else().Block(
				generateLocalStreamingCall(service, method, configMessageType)...,
			),
			jen.Line(),
			jen.Return(jen.Nil()),
		)
	}

	return statements
}

// generateRemoteStreamingCall generates code for remote streaming gRPC calls
func generateRemoteStreamingCall(_ *protogen.Service, method *protogen.Method, clientType string) []jen.Code {
	return []jen.Code{
		jen.Comment("Remote gRPC streaming call"),
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
		jen.List(jen.Id("stream"), jen.Err()).Op(":=").Id("client").Dot(method.GoName).Call(
			jen.Id("cmdCtx"),
			jen.Id("req"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to start stream: %w"), jen.Err())),
		),
		jen.Line(),
		jen.Comment("Receive and format each message in the stream"),
		jen.Var().Id("messageCount").Int(),
		jen.For().Block(
			jen.List(jen.Id("msg"), jen.Id("recvErr")).Op(":=").Id("stream").Dot("Recv").Call(),
			jen.If(jen.Id("recvErr").Op("==").Qual("io", "EOF")).Block(
				jen.Break(),
			),
			jen.If(jen.Id("recvErr").Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("stream receive error: %w"), jen.Id("recvErr"))),
			),
			jen.Line(),
			jen.Comment("Format and write the message"),
			jen.If(
				jen.Err().Op(":=").Id("outputFmt").Dot("Format").Call(
					jen.Id("cmdCtx"),
					jen.Id("cmd"),
					jen.Id("outputWriter"),
					jen.Id("msg"),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("format failed: %w"), jen.Err())),
			),
			jen.Line(),
			jen.Comment("Write delimiter"),
			jen.If(
				jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("outputWriter").Dot("Write").Call(
					jen.Index().Byte().Call(jen.Id("delimiter")),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to write delimiter: %w"), jen.Err())),
			),
			jen.Id("messageCount").Op("++"),
		),
		jen.Line(),
		jen.Comment("Write final newline to keep terminal clean (only if delimiter doesn't already end with newline)"),
		jen.If(
			jen.Id("messageCount").Op(">").Lit(0).Op("&&").Op("!").Qual("strings", "HasSuffix").Call(
				jen.Id("delimiter"),
				jen.Lit("\n"),
			),
		).Block(
			jen.If(
				jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("outputWriter").Dot("Write").Call(
					jen.Index().Byte().Call(jen.Lit("\n")),
				),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to write final newline: %w"), jen.Err())),
			),
		),
	}
}

// generateLocalStreamingCall generates code for local streaming calls
func generateLocalStreamingCall(service *protogen.Service, method *protogen.Method, configMessageType string) []jen.Code {
	var statements []jen.Code

	responseType := method.Output.GoIdent.GoName
	streamTypeName := streamWrapperTypeName(service, method)

	if configMessageType != "" {
		// Service has config - need to load it and call factory
		statements = append(statements,
			jen.Comment("Load config and create service implementation"),
			jen.Id("rootCmd").Op(":=").Id("cmd").Dot("Root").Call(),
			jen.Id("configPaths").Op(":=").Id("rootCmd").Dot("StringSlice").Call(jen.Lit("config")),
			jen.Id("envPrefix").Op(":=").Id("rootCmd").Dot("String").Call(jen.Lit("env-prefix")),
			jen.Line(),
			jen.Id("loader").Op(":=").Qual("github.com/drewfead/proto-cli", "NewConfigLoader").Call(
				jen.Qual("github.com/drewfead/proto-cli", "SingleCommandMode"),
				jen.Qual("github.com/drewfead/proto-cli", "FileConfig").Call(jen.Id("configPaths").Op("...")),
				jen.Qual("github.com/drewfead/proto-cli", "EnvPrefix").Call(jen.Id("envPrefix")),
			),
			jen.Line(),
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
			jen.List(jen.Id("svcImpl"), jen.Err()).Op(":=").Qual("github.com/drewfead/proto-cli", "CallFactory").Call(
				jen.Id("implOrFactory"),
				jen.Id("config"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to create service: %w"), jen.Err())),
			),
			jen.Line(),
		)
	} else {
		// No config - direct implementation call
		statements = append(statements,
			jen.Comment("Direct implementation call (no config)"),
			jen.Id("svcImpl").Op(":=").Id("implOrFactory").Assert(jen.Id(service.GoName+"Server")),
			jen.Line(),
		)
	}

	// Create local stream wrapper and call method
	statements = append(statements,
		jen.Comment("Create local stream wrapper for direct call"),
		jen.Id("localStream").Op(":=").Op("&").Id(streamTypeName).Values(jen.Dict{
			jen.Id("ctx"):       jen.Id("cmdCtx"),
			jen.Id("responses"): jen.Make(jen.Chan().Op("*").Id(responseType)),
			jen.Id("errors"):    jen.Make(jen.Chan().Error()),
		}),
		jen.Line(),
	)

	// Generate the method call inside goroutine (different based on config)
	var methodCallStmt jen.Code
	if configMessageType != "" {
		methodCallStmt = jen.Id("methodErr").Op("=").Id("svcImpl").Assert(jen.Id(service.GoName+"Server")).Dot(method.GoName).Call(
			jen.Id("req"),
			jen.Id("localStream"),
		)
	} else {
		methodCallStmt = jen.Id("methodErr").Op("=").Id("svcImpl").Dot(method.GoName).Call(
			jen.Id("req"),
			jen.Id("localStream"),
		)
	}

	statements = append(statements,
		jen.Comment("Call streaming method in goroutine"),
		jen.Go().Func().Params().Block(
			jen.Var().Id("methodErr").Error(),
			methodCallStmt,
			jen.Close(jen.Id("localStream").Dot("responses")),
			jen.If(jen.Id("methodErr").Op("!=").Nil()).Block(
				jen.Id("localStream").Dot("errors").Op("<-").Id("methodErr"),
			),
			jen.Close(jen.Id("localStream").Dot("errors")),
		).Call(),
		jen.Line(),
		jen.Comment("Receive and format each message in the stream"),
		jen.Var().Id("messageCount").Int(),
		jen.For().Block(
			jen.Select().Block(
				jen.Case(jen.List(jen.Id("msg"), jen.Id("ok")).Op(":=").Op("<-").Id("localStream").Dot("responses")).Block(
					jen.If(jen.Op("!").Id("ok")).Block(
						jen.Comment("Stream closed, check for errors"),
						jen.If(jen.Id("streamErr").Op(":=").Op("<-").Id("localStream").Dot("errors"), jen.Id("streamErr").Op("!=").Nil()).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("stream error: %w"), jen.Id("streamErr"))),
						),
						jen.Comment("Write final newline to keep terminal clean (only if delimiter doesn't already end with newline)"),
						jen.If(
							jen.Id("messageCount").Op(">").Lit(0).Op("&&").Op("!").Qual("strings", "HasSuffix").Call(
								jen.Id("delimiter"),
								jen.Lit("\n"),
							),
						).Block(
							jen.If(
								jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("outputWriter").Dot("Write").Call(
									jen.Index().Byte().Call(jen.Lit("\n")),
								),
								jen.Err().Op("!=").Nil(),
							).Block(
								jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to write final newline: %w"), jen.Err())),
							),
						),
						jen.Return(jen.Nil()),
					),
					jen.Line(),
					jen.Comment("Format and write the message"),
					jen.If(
						jen.Err().Op(":=").Id("outputFmt").Dot("Format").Call(
							jen.Id("cmdCtx"),
							jen.Id("cmd"),
							jen.Id("outputWriter"),
							jen.Id("msg"),
						),
						jen.Err().Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("format failed: %w"), jen.Err())),
					),
					jen.Line(),
					jen.Comment("Write delimiter"),
					jen.If(
						jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("outputWriter").Dot("Write").Call(
							jen.Index().Byte().Call(jen.Id("delimiter")),
						),
						jen.Err().Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to write delimiter: %w"), jen.Err())),
					),
					jen.Id("messageCount").Op("++"),
				),
				jen.Case(jen.Op("<-").Id("cmdCtx").Dot("Done").Call()).Block(
					jen.Return(jen.Id("cmdCtx").Dot("Err").Call()),
				),
			),
		),
	)

	return statements
}

// generateLocalStreamWrapper generates a helper type for local streaming calls
// This is generated at the package level (not inside a function)
func generateLocalStreamWrapper(f *jen.File, service *protogen.Service, method *protogen.Method) {
	streamTypeName := streamWrapperTypeName(service, method)
	responseType := method.Output.GoIdent.GoName

	f.Comment(fmt.Sprintf("%s is a helper type for local server streaming calls to %s", streamTypeName, method.GoName))
	f.Type().Id(streamTypeName).Struct(
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("responses").Chan().Op("*").Id(responseType),
		jen.Id("errors").Chan().Error(),
	)
	f.Line()

	// Send method
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("Send").Params(
		jen.Id("resp").Op("*").Id(responseType),
	).Error().Block(
		jen.Select().Block(
			jen.Case(jen.Id("s").Dot("responses").Op("<-").Id("resp")).Block(
				jen.Return(jen.Nil()),
			),
			jen.Case(jen.Op("<-").Id("s").Dot("ctx").Dot("Done").Call()).Block(
				jen.Return(jen.Id("s").Dot("ctx").Dot("Err").Call()),
			),
		),
	)
	f.Line()

	// Context method
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("Context").Params().Qual("context", "Context").Block(
		jen.Return(jen.Id("s").Dot("ctx")),
	)
	f.Line()

	// SetHeader method (no-op for local calls)
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("SetHeader").Params(
		jen.Qual("google.golang.org/grpc/metadata", "MD"),
	).Error().Block(
		jen.Return(jen.Nil()),
	)
	f.Line()

	// SendHeader method (no-op for local calls)
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("SendHeader").Params(
		jen.Qual("google.golang.org/grpc/metadata", "MD"),
	).Error().Block(
		jen.Return(jen.Nil()),
	)
	f.Line()

	// SetTrailer method (no-op for local calls)
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("SetTrailer").Params(
		jen.Qual("google.golang.org/grpc/metadata", "MD"),
	).Block()
	f.Line()

	// SendMsg method (delegates to Send)
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("SendMsg").Params(
		jen.Id("m").Any(),
	).Error().Block(
		jen.List(jen.Id("msg"), jen.Id("ok")).Op(":=").Id("m").Assert(jen.Op("*").Id(responseType)),
		jen.If(jen.Op("!").Id("ok")).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(
				jen.Lit("invalid message type: expected *%s, got %T"),
				jen.Lit(responseType),
				jen.Id("m"),
			)),
		),
		jen.Return(jen.Id("s").Dot("Send").Call(jen.Id("msg"))),
	)
	f.Line()

	// RecvMsg method (not used for server streaming, but required by interface)
	f.Func().Params(
		jen.Id("s").Op("*").Id(streamTypeName),
	).Id("RecvMsg").Params(
		jen.Id("m").Any(),
	).Error().Block(
		jen.Return(jen.Qual("fmt", "Errorf").Call(
			jen.Lit("RecvMsg not supported on server streaming"),
		)),
	)
	f.Line()
}
