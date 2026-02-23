package protocli

import (
	"context"

	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"
)

// TUIFieldKind describes the input semantics of a TUI form field.
type TUIFieldKind int

const (
	TUIFieldKindString   TUIFieldKind = iota
	TUIFieldKindInt
	TUIFieldKindFloat
	TUIFieldKindBool
	TUIFieldKindEnum
	TUIFieldKindRepeated
	TUIFieldKindMessage
)

// TUIEnumValue is one valid value for an enum field.
type TUIEnumValue struct {
	Name  string // display/CLI name
	Value int32  // proto numeric value
}

// TUIFieldDescriptor describes one input field in a TUI form.
type TUIFieldDescriptor struct {
	Name            string
	Label           string // from tui_label annotation or flag name
	Usage           string // from flag usage
	Description     string // from flag description
	DefaultValue    string // from default_value annotation; pre-populates TUI form fields
	Required        bool
	Hidden          bool         // from tui_hidden annotation
	Kind            TUIFieldKind
	MessageFullName string           // proto full name for message fields (e.g. "google.protobuf.Timestamp")
	EnumValues      []TUIEnumValue   // for enum fields
	ElementKind     TUIFieldKind     // for repeated fields
	Fields          []TUIFieldDescriptor // for message fields (nested)
	// Setter parses a string value and sets it on the request message.
	Setter func(proto.Message, string) error
	// Appender appends a parsed string element to a repeated field.
	Appender func(proto.Message, string) error
}

// TUIResponseDescriptor describes the response type returned by an RPC method.
// It is passed to ResponseViewFactory so factories can dispatch to different
// presentations based on the response message type or the originating method.
type TUIResponseDescriptor struct {
	// MethodName is the CLI command name of the method that produced the response.
	MethodName string
	// MessageFullName is the proto full name of the response message
	// (e.g. "mypackage.MyResponse"). Populated by the generator.
	MessageFullName string
}

// TUIMethodDescriptor describes one RPC method for TUI interaction.
type TUIMethodDescriptor struct {
	Name        string // CLI command name
	DisplayName string
	Description string
	Hidden      bool // from tui_hidden annotation
	IsStreaming bool
	// ResponseDescriptor describes the response type for this method.
	// Passed to ResponseViewFactory so factories can dispatch by type.
	ResponseDescriptor TUIResponseDescriptor
	// NewRequest returns a fresh zero-value request message.
	NewRequest func() proto.Message
	// InputFields describes all form inputs for building the request.
	InputFields []TUIFieldDescriptor
	// Invoke calls the unary method with the given request.
	// cmd is the root command; the closure uses it to load config (same as
	// the generated action code: reads --config and --env-prefix flags,
	// creates a ConfigLoader, calls CallFactory if the service has a config).
	Invoke func(ctx context.Context, cmd *cli.Command, req proto.Message) (proto.Message, error)
	// InvokeStream calls a server-streaming method.
	InvokeStream func(ctx context.Context, cmd *cli.Command, req proto.Message, recv func(proto.Message) error) error
}

// TUIServiceDescriptor describes a service's TUI-navigable structure.
type TUIServiceDescriptor struct {
	Name        string
	DisplayName string
	Description string
	Methods     []*TUIMethodDescriptor
}

// TUIMethod is the interface for RPC methods visible in the interactive TUI.
// Use this type in code that consumes descriptors (e.g. contrib/tui) so that
// the concrete *TUIMethodDescriptor can evolve independently.
type TUIMethod interface {
	TUIName() string
	TUIDisplayName() string
	TUIDescription() string
	TUIHidden() bool
	TUIIsStreaming() bool
	TUIResponseDescriptor() TUIResponseDescriptor
	TUINewRequest() proto.Message
	TUIInputFields() []TUIFieldDescriptor
	TUIInvoke(ctx context.Context, cmd *cli.Command, req proto.Message) (proto.Message, error)
	TUIInvokeStream(ctx context.Context, cmd *cli.Command, req proto.Message, recv func(proto.Message) error) error
}

// tuiMethodWrapper adapts *TUIMethodDescriptor to the TUIMethod interface.
type tuiMethodWrapper struct{ desc *TUIMethodDescriptor }

func (w tuiMethodWrapper) TUIName() string        { return w.desc.Name }
func (w tuiMethodWrapper) TUIDisplayName() string { return w.desc.DisplayName }
func (w tuiMethodWrapper) TUIDescription() string { return w.desc.Description }
func (w tuiMethodWrapper) TUIHidden() bool        { return w.desc.Hidden }
func (w tuiMethodWrapper) TUIIsStreaming() bool   { return w.desc.IsStreaming }
func (w tuiMethodWrapper) TUIResponseDescriptor() TUIResponseDescriptor {
	return w.desc.ResponseDescriptor
}
func (w tuiMethodWrapper) TUINewRequest() proto.Message { return w.desc.NewRequest() }
func (w tuiMethodWrapper) TUIInputFields() []TUIFieldDescriptor {
	return w.desc.InputFields
}
func (w tuiMethodWrapper) TUIInvoke(ctx context.Context, cmd *cli.Command, req proto.Message) (proto.Message, error) {
	return w.desc.Invoke(ctx, cmd, req)
}
func (w tuiMethodWrapper) TUIInvokeStream(ctx context.Context, cmd *cli.Command, req proto.Message, recv func(proto.Message) error) error {
	return w.desc.InvokeStream(ctx, cmd, req, recv)
}

// TUIService is the interface for services visible in the interactive TUI.
// Use this type in code that consumes descriptors (e.g. contrib/tui) so that
// the concrete *TUIServiceDescriptor can evolve independently.
type TUIService interface {
	TUIName() string
	TUIDisplayName() string
	TUIDescription() string
	TUIMethods() []TUIMethod
}

// tuiServiceWrapper adapts *TUIServiceDescriptor to the TUIService interface.
// A plain method cannot share a name with a struct field in Go, so we wrap.
type tuiServiceWrapper struct{ desc *TUIServiceDescriptor }

func (w tuiServiceWrapper) TUIName() string        { return w.desc.Name }
func (w tuiServiceWrapper) TUIDisplayName() string { return w.desc.DisplayName }
func (w tuiServiceWrapper) TUIDescription() string { return w.desc.Description }
func (w tuiServiceWrapper) TUIMethods() []TUIMethod {
	methods := make([]TUIMethod, len(w.desc.Methods))
	for i, m := range w.desc.Methods {
		methods[i] = tuiMethodWrapper{desc: m}
	}
	return methods
}

// TUIRunConfig holds the parsed options for a single TUIProvider.Run call.
// Providers read this to determine which service/method to open on launch.
type TUIRunConfig struct {
	// StartServiceName is the CLI name of the service to pre-select (e.g. "farewell").
	// Empty means start at the first service.
	StartServiceName string
	// StartMethodName is the CLI name of the method to open the form for (e.g. "leave-note").
	// Only meaningful when StartServiceName is also set. Empty means show the method list.
	StartMethodName string
	// FieldValues pre-populates form fields keyed by field flag-name (e.g. "name").
	// Only meaningful when StartMethodName is also set. Values override the proto-defined
	// default for the matching field.
	FieldValues map[string]string
}

// TUIRunOption customises a single TUIProvider.Run invocation.
type TUIRunOption func(*TUIRunConfig)

// StartAtService opens the TUI with the named service's method list pre-selected.
// cliName is the CLI command name of the service (e.g. "farewell").
func StartAtService(cliName string) TUIRunOption {
	return func(c *TUIRunConfig) { c.StartServiceName = cliName }
}

// StartAtMethod opens the TUI directly on the request form for the named method.
// serviceCLIName is the service command name; methodCLIName is the method command name.
func StartAtMethod(serviceCLIName, methodCLIName string) TUIRunOption {
	return func(c *TUIRunConfig) {
		c.StartServiceName = serviceCLIName
		c.StartMethodName = methodCLIName
	}
}

// WithPrefillFields pre-populates form fields when opening the TUI at a specific method.
// fields is a map from field flag-name (e.g. "name") to its string representation.
// Only meaningful when StartAtMethod is also used.
func WithPrefillFields(fields map[string]string) TUIRunOption {
	return func(c *TUIRunConfig) { c.FieldValues = fields }
}

// tuiLaunchKey is the Metadata key used to store the TUI launch function on the root command.
const tuiLaunchKey = "protocli:tuiLaunchFn"

// TUILaunchFn is the function stored in the root command's Metadata by RootCommand
// when a TUI provider is configured. Generated Before hooks call InvokeTUI which
// retrieves and calls this function.
type TUILaunchFn func(ctx context.Context, rootCmd *cli.Command, opts ...TUIRunOption) error

// InvokeTUI triggers the interactive TUI from a generated service or method command's
// Before hook. It retrieves the launch function registered by RootCommand and calls it
// with the given options. Returns nil without error if no TUI provider is registered.
func InvokeTUI(ctx context.Context, cmd *cli.Command, opts ...TUIRunOption) error {
	fn, ok := cmd.Root().Metadata[tuiLaunchKey].(TUILaunchFn)
	if !ok {
		return nil
	}
	return fn(ctx, cmd.Root(), opts...)
}

// TUIProvider is implemented by contrib/tui (or user code) to provide
// an interactive TUI. Receives descriptors for all tui-enabled services.
// cmd is the root command, forwarded to each Invoke call for config loading.
type TUIProvider interface {
	Run(ctx context.Context, cmd *cli.Command, services []TUIService, opts ...TUIRunOption) error
}
