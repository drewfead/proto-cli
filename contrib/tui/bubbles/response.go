package bubbles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	protocli "github.com/drewfead/proto-cli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ResponseView is a bubbletea sub-model for the response screen.
// Implement this interface to replace the default JSON viewport with any
// interactive presentation — syntax-highlighted output, tables, diffs, etc.
//
// The contract mirrors a mutable bubbletea component: Update modifies the
// receiver in place and returns only a Cmd, avoiding the type-assertion
// overhead of the standard tea.Model interface.
type ResponseView interface {
	// Init is called once before the response screen is shown. ctx is the
	// request context (cancellable for streaming RPCs). msg is the initial
	// response proto, or nil for streaming views. It may return an initial Cmd.
	Init(ctx context.Context, msg proto.Message, width, height int) tea.Cmd
	// SetSize is called whenever the terminal is resized.
	SetSize(width, height int)
	// Update handles bubbletea messages and returns any follow-up Cmd.
	Update(msg tea.Msg) tea.Cmd
	// View renders the response content area.
	View() string
}

// StreamingResponseView is an optional extension of ResponseView for
// server-streaming RPC methods. ResponseViews that implement this interface
// receive messages incrementally as they arrive from the server.
// Views that do not implement this interface receive only the final message
// via Init when the stream completes (or nothing on error).
type StreamingResponseView interface {
	ResponseView
	// Append is called for each message received from the stream.
	Append(msg proto.Message)
	// Done is called once when the stream ends. err is nil on clean completion.
	Done(err error)
}

// ResponseViewFactory creates a ResponseView for a single RPC invocation.
// The desc parameter describes the response type so factories can dispatch to
// different presentations based on the response message full name or method
// name. The styles parameter carries the active palette so custom views can
// follow the application's colour scheme.
type ResponseViewFactory func(desc protocli.TUIResponseDescriptor, styles Styles) ResponseView

// MarshalResponse serialises a proto.Message to a human-readable indented JSON
// string. Custom ResponseView implementations can call this to obtain the same
// baseline representation the default view uses.
func MarshalResponse(resp proto.Message) string { return marshalResponse(resp) }

func marshalResponse(resp proto.Message) string {
	if resp == nil {
		return "(no response)"
	}
	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}
	if b, err := marshaler.Marshal(resp); err == nil {
		return string(b)
	}
	if b, err := json.MarshalIndent(resp, "", "  "); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", resp)
}

// ─── Viewport response view ───────────────────────────────────────────────────

// viewportResponseView is the built-in ResponseView: marshals the proto to
// indented JSON and presents it in a scrollable viewport.
// It also implements StreamingResponseView for server-streaming methods.
type viewportResponseView struct {
	vp         viewport.Model
	styles     Styles
	chunks     []string // accumulated rendered messages for streaming
	autoScroll bool     // if true, GotoBottom after each Append
}

// ViewportOption configures a viewportResponseView.
type ViewportOption func(*viewportResponseView)

// WithAutoScroll makes the streaming viewport automatically follow the latest
// message by scrolling to the bottom after each Append. By default the
// viewport position is user-controlled and new messages arrive below the fold.
func WithAutoScroll() ViewportOption {
	return func(v *viewportResponseView) { v.autoScroll = true }
}

// NewViewportResponseView returns a ResponseViewFactory that renders responses
// as indented JSON in a scrollable viewport. This is the default used by
// tui.New when no WithResponseView option is supplied.
// Pass ViewportOption values to configure behaviour; for example:
//
//	bubbles.NewViewportResponseView(bubbles.WithAutoScroll())
func NewViewportResponseView(opts ...ViewportOption) ResponseViewFactory {
	return func(_ protocli.TUIResponseDescriptor, styles Styles) ResponseView {
		rv := &viewportResponseView{styles: styles}
		for _, o := range opts {
			o(rv)
		}
		return rv
	}
}

func (v *viewportResponseView) Init(_ context.Context, msg proto.Message, width, height int) tea.Cmd {
	v.vp = viewport.New(width, height)
	v.chunks = nil
	if msg != nil {
		v.vp.SetContent(v.styles.Response.Render(marshalResponse(msg)))
	}
	return nil
}

// Append implements StreamingResponseView. Each call appends the rendered
// message to the viewport content. When autoScroll is enabled the viewport
// follows the latest message; otherwise the user controls scroll position.
func (v *viewportResponseView) Append(msg proto.Message) {
	v.chunks = append(v.chunks, marshalResponse(msg))
	v.vp.SetContent(v.styles.Response.Render(strings.Join(v.chunks, "\n\n")))
	if v.autoScroll {
		v.vp.GotoBottom()
	}
}

// Done implements StreamingResponseView. Called when the stream ends.
// The viewport content is already set by Append calls; no additional
// rendering is needed here.
func (v *viewportResponseView) Done(_ error) {}

func (v *viewportResponseView) SetSize(width, height int) {
	v.vp.Width = width
	v.vp.Height = height
}

func (v *viewportResponseView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

func (v *viewportResponseView) View() string { return v.vp.View() }

// ─── Table response view ──────────────────────────────────────────────────────

// tableRow holds one rendered (field-name, value) pair for the table.
type tableRow struct {
	key string
	val string
}

// tableResponseView renders RPC responses as a two-column lipgloss table.
// The left column lists field names; the right column lists their values.
// Nested objects and arrays are compacted to a single JSON line.
// The table uses the active palette: primary for the header, accent for field
// names, and muted for the border.
// It also implements StreamingResponseView: each Append call adds the new
// message's fields as rows, growing the table incrementally.
type tableResponseView struct {
	vp         viewport.Model
	styles     Styles
	width      int
	height     int
	streamRows []tableRow // accumulated rows from streaming Append calls
}

// NewTableResponseView returns a ResponseViewFactory that renders responses as
// a styled two-column lipgloss table. Register it via tui.WithResponseView:
//
//	tui.New(tui.WithResponseView(bubbles.NewTableResponseView()))
func NewTableResponseView() ResponseViewFactory {
	return func(_ protocli.TUIResponseDescriptor, styles Styles) ResponseView {
		return &tableResponseView{styles: styles}
	}
}

func (v *tableResponseView) Init(_ context.Context, msg proto.Message, width, height int) tea.Cmd {
	v.width = width
	v.height = height
	v.vp = viewport.New(width, height)
	v.streamRows = nil
	if msg != nil {
		v.vp.SetContent(v.render(msg))
	}
	return nil
}

// Append implements StreamingResponseView. Each call extracts the new message's
// fields as (key, value) pairs and appends them to the growing table.
func (v *tableResponseView) Append(msg proto.Message) {
	raw := marshalResponse(msg)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err == nil {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := fields[k]
			var s string
			if err := json.Unmarshal(val, &s); err == nil {
				v.streamRows = append(v.streamRows, tableRow{k, s})
			} else {
				var buf bytes.Buffer
				_ = json.Compact(&buf, val)
				v.streamRows = append(v.streamRows, tableRow{k, buf.String()})
			}
		}
	}
	v.vp.SetContent(v.renderPairs(v.streamRows))
}

// Done implements StreamingResponseView. No additional rendering needed;
// the table is already up to date from Append calls.
func (v *tableResponseView) Done(_ error) {}

func (v *tableResponseView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.vp.Width = width
	v.vp.Height = height
}

func (v *tableResponseView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

func (v *tableResponseView) View() string { return v.vp.View() }

func (v *tableResponseView) render(msg proto.Message) string {
	raw := marshalResponse(msg)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return v.styles.Response.Render(raw)
	}
	if len(fields) == 0 {
		return v.styles.Subtitle.Render("(empty response)")
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]tableRow, 0, len(fields))
	for _, k := range keys {
		val := fields[k]
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			rows = append(rows, tableRow{k, s})
		} else {
			var buf bytes.Buffer
			_ = json.Compact(&buf, val)
			rows = append(rows, tableRow{k, buf.String()})
		}
	}
	return v.renderPairs(rows)
}

// renderPairs builds a two-column lipgloss table from an ordered slice of
// (key, value) pairs. Used by both render (single message) and Append (streaming).
func (v *tableResponseView) renderPairs(rows []tableRow) string {
	if len(rows) == 0 {
		return v.styles.Subtitle.Render("(no messages yet)")
	}

	// Measure the longest field name so the Field column can shrink to fit.
	// Width includes 1-char padding on each side, so add 2. Minimum is the
	// header "Field" (5) + padding = 7.
	maxFieldLen := len("Field")
	for _, r := range rows {
		if len(r.key) > maxFieldLen {
			maxFieldLen = len(r.key)
		}
	}
	fieldColW := maxFieldLen + 2 // content + horizontal padding

	// Derive colors from the active styles so the table follows the theme.
	primaryColor := v.styles.Colors.Primary
	accentColor := v.styles.Colors.Accent
	dimColor := v.styles.Colors.Secondary

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Padding(0, 1)
	fieldStyle := lipgloss.NewStyle().Foreground(accentColor).Padding(0, 1).Width(fieldColW)
	altFieldStyle := lipgloss.NewStyle().Foreground(dimColor).Padding(0, 1).Width(fieldColW)
	valueStyle := lipgloss.NewStyle().Padding(0, 1)
	altValueStyle := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	borderStyle := lipgloss.NewStyle().Foreground(v.styles.Colors.SecondaryBorder)

	t := table.New().
		Border(v.styles.CardBox.GetBorderStyle()).
		BorderRow(true).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			alt := row%2 == 1
			if col == 0 {
				if alt {
					return altFieldStyle
				}
				return fieldStyle
			}
			if alt {
				return altValueStyle
			}
			return valueStyle
		}).
		Headers("Field", "Value").
		Width(v.width)

	for _, r := range rows {
		t.Row(r.key, r.val)
	}

	return t.String()
}

// ─── Card grid response view ──────────────────────────────────────────────────

// CardContentFactory renders the interior content of a single card.
// msg is the proto message the card represents; innerWidth is the available
// content width in characters; styles carries the active colour palette.
// The returned string is placed inside the card's border box.
type CardContentFactory func(msg proto.Message, innerWidth int, styles Styles) string

// CardActionErrorMsg is dispatched when a CardActionHandler's action function
// returns a non-nil error. The root model surfaces it via the error text area.
type CardActionErrorMsg struct{ Err error }

// CardSelectResult is the return value of a CardActionHandler. Construct it
// with CardSelectCmd (to dispatch a bubbletea command) or CardSelectAction
// (to run an arbitrary Go function in a goroutine).
type CardSelectResult struct {
	cmd    tea.Cmd
	action func(ctx context.Context) error
}

// CardSelectCmd wraps a bubbletea command as a CardSelectResult. The command
// is dispatched into the bubbletea runtime by the root model, so it can
// trigger anything that a normal tea.Cmd can — ShowModal, state mutations, etc.
func CardSelectCmd(cmd tea.Cmd) CardSelectResult {
	return CardSelectResult{cmd: cmd}
}

// CardSelectAction wraps an arbitrary Go function as a CardSelectResult. The
// function is run in a goroutine with the context that was passed to Init (the
// request context, which is cancellable for streaming RPCs). A non-nil error is
// sent back to the TUI as a CardActionErrorMsg.
func CardSelectAction(fn func(ctx context.Context) error) CardSelectResult {
	return CardSelectResult{action: fn}
}

// CardKeyBinding describes one key binding that a CardActionHandler handles.
// Key is matched against tea.KeyMsg.String() and may be a multi-character
// sequence such as ":g" — the view buffers characters until a complete match
// (or no-match) is reached.
type CardKeyBinding struct {
	Key         string // e.g. "enter", ":g", "d"
	Description string // shown in the help line, e.g. "greet"
}

// CardActionHandler handles keyboard actions on focused cards in a grid.
// Register it with WithCardAction. The view calls KeyBindings once to learn
// which keys (or key sequences) to capture, and calls Handle whenever one of
// those sequences is completed.
type CardActionHandler interface {
	// KeyBindings returns all key bindings this handler responds to.
	KeyBindings() []CardKeyBinding
	// Handle is called when a registered key or sequence is pressed on the
	// focused card. key matches one of the Key values from KeyBindings.
	Handle(ctx context.Context, key string, msg proto.Message) CardSelectResult
}

// CardActionHandlerFunc is a convenience adapter that satisfies CardActionHandler
// with a bindings slice and a plain function, avoiding a named type for simple cases.
type CardActionHandlerFunc struct {
	Bindings []CardKeyBinding
	Fn       func(ctx context.Context, key string, msg proto.Message) CardSelectResult
}

func (h CardActionHandlerFunc) KeyBindings() []CardKeyBinding { return h.Bindings }
func (h CardActionHandlerFunc) Handle(ctx context.Context, key string, msg proto.Message) CardSelectResult {
	return h.Fn(ctx, key, msg)
}

// CardGridOption configures a cardGridResponseView.
type CardGridOption func(*cardGridResponseView)

// WithCardWidth sets the inner content width of each card (excluding border and
// padding). Defaults to 24 when not set.
func WithCardWidth(w int) CardGridOption {
	return func(v *cardGridResponseView) { v.cardWidth = w }
}

// WithColumns forces the number of cards per row in the grid. By default the
// column count is derived from the terminal width and card width.
func WithColumns(n int) CardGridOption {
	return func(v *cardGridResponseView) { v.columns = n }
}

// WithCardContent registers a custom CardContentFactory that replaces the
// default field-value table rendered inside each card. Use this to produce any
// interior layout — compact single-line summaries, sparklines, custom tables,
// etc. — while keeping the grid layout, height-normalisation, and border
// rendering provided by NewCardGridResponseView.
func WithCardContent(factory CardContentFactory) CardGridOption {
	return func(v *cardGridResponseView) { v.cardContent = factory }
}

// WithFillWidth makes each card's inner content width fill the available
// viewport rather than using a fixed card width. Combine with WithColumns(1)
// for a single-column layout where every card spans the full terminal width.
func WithFillWidth() CardGridOption {
	return func(v *cardGridResponseView) { v.fillWidth = true }
}

// WithCardAction registers a CardActionHandler that makes cards focusable with
// arrow-key navigation. The handler declares its key bindings (single keys or
// multi-char sequences such as ":g") via KeyBindings, and Handle is called
// whenever a complete sequence is recognised on the focused card.
// The focused card is highlighted with the primary border colour.
// Arrow keys (←→↑↓) always navigate; Esc cancels any in-progress sequence or
// returns to the form.
func WithCardAction(handler CardActionHandler) CardGridOption {
	return func(v *cardGridResponseView) { v.actionHandler = handler }
}

// cardGridResponseView renders each RPC response message as a card in a flowing
// grid. Each card is a rounded-border lipgloss box; the card interior uses a
// borderless lipgloss table for field-value column alignment.
// It implements StreamingResponseView: Append adds one new card per message.
// When actionHandler is set it also implements KeyCapturer and HelpTextProvider,
// making cards individually focusable with arrow-key navigation and actions.
type cardGridResponseView struct {
	vp            viewport.Model
	styles        Styles
	ctx           context.Context    // request context threaded from Init
	width         int
	height        int
	cards         []cardEntry        // one entry per accumulated message
	cardWidth     int                // inner content width; 0 = default (24)
	fillWidth     bool               // when true, innerCardWidth fills the viewport
	columns       int                // forced column count; 0 = auto
	cardContent   CardContentFactory // nil = default field-value table
	actionHandler CardActionHandler  // nil = cards not focusable
	commandBuf    string             // in-progress multi-char key sequence; empty when idle
	focusedCard   int                // index of the focused card
}

// cardEntry holds both the parsed rows (for the default renderer) and the
// original proto message (for custom CardContentFactory calls).
type cardEntry struct {
	msg  proto.Message
	rows []tableRow
}

// NewCardGridResponseView returns a ResponseViewFactory that renders each
// response message as a card in a flowing grid. Cards accumulate as stream
// messages arrive when used with a server-streaming RPC.
//
// Register it via tui.WithResponseView:
//
//	tui.New(tui.WithResponseView(bubbles.NewCardGridResponseView()))
//
// Customise card dimensions:
//
//	bubbles.NewCardGridResponseView(
//	    bubbles.WithCardWidth(32),
//	    bubbles.WithColumns(3),
//	)
func NewCardGridResponseView(opts ...CardGridOption) ResponseViewFactory {
	return func(_ protocli.TUIResponseDescriptor, styles Styles) ResponseView {
		rv := &cardGridResponseView{styles: styles}
		for _, o := range opts {
			o(rv)
		}
		return rv
	}
}

func (v *cardGridResponseView) Init(ctx context.Context, msg proto.Message, width, height int) tea.Cmd {
	v.ctx = ctx
	v.width = width
	v.height = height
	v.vp = viewport.New(width, height)
	v.cards = nil
	if msg != nil {
		v.cards = append(v.cards, cardEntry{msg: msg, rows: v.parseRows(msg)})
		v.vp.SetContent(v.renderGrid())
	}
	return nil
}

func (v *cardGridResponseView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.vp.Width = width
	v.vp.Height = height
	if len(v.cards) > 0 {
		// Re-render because column count may change with the new width.
		v.vp.SetContent(v.renderGrid())
	}
}

func (v *cardGridResponseView) Update(msg tea.Msg) tea.Cmd {
	if v.actionHandler != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			n := len(v.cards)
			cols := v.columnCount()
			switch key.String() {
			case "left":
				if v.focusedCard > 0 {
					v.focusedCard--
				}
				v.vp.SetContent(v.renderGrid())
				return nil
			case "right":
				if v.focusedCard < n-1 {
					v.focusedCard++
				}
				v.vp.SetContent(v.renderGrid())
				return nil
			case "up":
				if v.focusedCard-cols >= 0 {
					v.focusedCard -= cols
				}
				v.vp.SetContent(v.renderGrid())
				return nil
			case "down":
				if v.focusedCard+cols < n {
					v.focusedCard += cols
				}
				v.vp.SetContent(v.renderGrid())
				return nil
			default:
				return v.handleActionKey(key.String())
			}
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

// handleActionKey processes a key press against the registered action handler.
// It accumulates multi-char sequences in commandBuf and dispatches once a
// complete binding is matched. Esc cancels any in-progress sequence.
func (v *cardGridResponseView) handleActionKey(key string) tea.Cmd {
	// Esc cancels an in-progress sequence; otherwise the root model handles it.
	if key == "esc" {
		if v.commandBuf != "" {
			v.commandBuf = ""
			// No grid re-render needed; HelpText updates on the next View() call.
		}
		return nil
	}

	candidate := v.commandBuf + key

	// Check for an exact match.
	for _, b := range v.actionHandler.KeyBindings() {
		if b.Key == candidate {
			v.commandBuf = ""
			return v.dispatchAction(candidate)
		}
	}

	// Check whether candidate is a strict prefix of any binding (more chars expected).
	for _, b := range v.actionHandler.KeyBindings() {
		if strings.HasPrefix(b.Key, candidate) && b.Key != candidate {
			v.commandBuf = candidate
			return nil // HelpText will reflect the pending sequence on next View()
		}
	}

	// No match and no prefix: discard the buffer.
	v.commandBuf = ""
	return nil
}

// dispatchAction invokes the handler for a completed key sequence on the
// currently focused card, returning the appropriate bubbletea command.
func (v *cardGridResponseView) dispatchAction(key string) tea.Cmd {
	n := len(v.cards)
	if v.focusedCard < 0 || v.focusedCard >= n {
		return nil
	}
	ctx := v.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	result := v.actionHandler.Handle(ctx, key, v.cards[v.focusedCard].msg)
	if result.cmd != nil {
		return result.cmd
	}
	if result.action != nil {
		fn := result.action
		return func() tea.Msg {
			if err := fn(ctx); err != nil {
				return CardActionErrorMsg{Err: err}
			}
			return nil
		}
	}
	return nil
}

// CapturesKey implements KeyCapturer. When an actionHandler is registered the
// grid claims arrow keys, any key that starts a registered binding, and (when
// mid-sequence) any single character or Esc.
func (v *cardGridResponseView) CapturesKey(key string) bool {
	if v.actionHandler == nil {
		return false
	}
	switch key {
	case "left", "right", "up", "down":
		return true
	}
	// Mid-sequence: capture all single chars and Esc so the sequence can
	// be completed or cancelled without the root model intercepting.
	if v.commandBuf != "" {
		return key == "esc" || len(key) == 1
	}
	// Capture any key that is a prefix of (or exactly matches) a binding.
	for _, b := range v.actionHandler.KeyBindings() {
		if strings.HasPrefix(b.Key, key) {
			return true
		}
	}
	return false
}

// HelpText implements HelpTextProvider. Lists all registered bindings.
// While a multi-char sequence is in progress the pending prefix is shown
// instead so the user can see what they have typed so far.
func (v *cardGridResponseView) HelpText() string {
	if v.actionHandler == nil {
		return ""
	}
	if v.commandBuf != "" {
		return v.commandBuf + "… • esc: cancel"
	}
	parts := []string{"↑↓←→: navigate"}
	for _, b := range v.actionHandler.KeyBindings() {
		parts = append(parts, b.Key+": "+b.Description)
	}
	parts = append(parts, "esc: back")
	return strings.Join(parts, " • ")
}

func (v *cardGridResponseView) View() string { return v.vp.View() }

// Append implements StreamingResponseView. Each call adds one new card to the
// grid and re-renders the viewport.
func (v *cardGridResponseView) Append(msg proto.Message) {
	v.cards = append(v.cards, cardEntry{msg: msg, rows: v.parseRows(msg)})
	v.vp.SetContent(v.renderGrid())
}

// Done implements StreamingResponseView. No additional rendering needed.
func (v *cardGridResponseView) Done(_ error) {}

// parseRows extracts sorted (field, value) pairs from a proto message's JSON.
func (v *cardGridResponseView) parseRows(msg proto.Message) []tableRow {
	raw := marshalResponse(msg)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		// Non-JSON fallback: treat the whole response as a single value row.
		return []tableRow{{"response", raw}}
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]tableRow, 0, len(keys))
	for _, k := range keys {
		val := fields[k]
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			rows = append(rows, tableRow{k, s})
		} else {
			var buf bytes.Buffer
			_ = json.Compact(&buf, val)
			rows = append(rows, tableRow{k, buf.String()})
		}
	}
	return rows
}

func (v *cardGridResponseView) innerCardWidth() int {
	if v.fillWidth {
		// border (2) + padding left+right (2) = 4 overhead; leave at least 1.
		if w := v.width - 4; w > 0 {
			return w
		}
		return 1
	}
	if v.cardWidth > 0 {
		return v.cardWidth
	}
	return 24
}

// fullCardWidth returns the total rendered width of a card including border (2)
// and horizontal padding (2).
func (v *cardGridResponseView) fullCardWidth() int {
	return v.innerCardWidth() + 4
}

func (v *cardGridResponseView) columnCount() int {
	if v.columns > 0 {
		return v.columns
	}
	// Leave a 1-char gap between cards.
	cols := v.width / (v.fullCardWidth() + 1)
	if cols < 1 {
		cols = 1
	}
	return cols
}

// renderCardContent renders the interior of a card without its border box.
// If a CardContentFactory is registered it is used; otherwise the default
// field-value table is rendered. The returned string is measured for height
// before all cards in a grid row are normalised to the same height.
func (v *cardGridResponseView) renderCardContent(entry cardEntry) string {
	innerW := v.innerCardWidth()

	if v.cardContent != nil {
		return v.cardContent(entry.msg, innerW, v.styles)
	}

	return v.defaultCardContent(entry.rows, innerW)
}

// defaultCardContent renders a field-value table — the built-in card interior.
func (v *cardGridResponseView) defaultCardContent(rows []tableRow, innerW int) string {
	accentColor := v.styles.Colors.Accent
	mutedColor := v.styles.Colors.Secondary

	// Field column: width = longest key + 1 space gap, minimum 5.
	maxKeyLen := 5
	for _, r := range rows {
		if len(r.key) > maxKeyLen {
			maxKeyLen = len(r.key)
		}
	}
	fieldColW := maxKeyLen + 1

	// Inner table uses HiddenBorder (single-space chars) so the columns align
	// without adding any visible chrome. The enclosing card box supplies the border.
	innerTable := table.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == 0 {
				return lipgloss.NewStyle().Foreground(accentColor).Width(fieldColW)
			}
			return lipgloss.NewStyle().Foreground(mutedColor)
		}).
		Width(innerW)

	for _, r := range rows {
		innerTable.Row(r.key, r.val)
	}

	return innerTable.String()
}

// renderCardBox wraps pre-rendered content in the card's rounded-border box.
// contentHeight is the minimum content area height in lines; pass 0 for natural
// (content-driven) height. When focused is true the border is drawn in the
// primary colour to highlight the selected card.
func (v *cardGridResponseView) renderCardBox(content string, focused bool, contentHeight int) string {
	borderColor := v.styles.Colors.SecondaryBorder
	if focused {
		borderColor = v.styles.Colors.Primary
	}
	style := v.styles.CardBox.
		BorderForeground(borderColor).
		Width(v.innerCardWidth())
	if contentHeight > 0 {
		style = style.Height(contentHeight)
	}
	return style.Render(content)
}

// renderGrid arranges all cards into rows of columnCount cards each.
// Within each row cards are height-normalised using a two-pass approach:
//  1. Render every card at its natural height and measure with lipgloss.Height
//     so that any lines wrapped by the Width constraint are counted correctly.
//  2. Re-render cards shorter than the row maximum with an explicit Height set,
//     which causes lipgloss to pad the content area with blank lines.
func (v *cardGridResponseView) renderGrid() string {
	if len(v.cards) == 0 {
		return v.styles.Subtitle.Render("(no messages yet)")
	}

	cols := v.columnCount()
	gap := lipgloss.NewStyle().PaddingRight(1)

	var gridRows []string
	for i := 0; i < len(v.cards); i += cols {
		end := i + cols
		if end > len(v.cards) {
			end = len(v.cards)
		}
		rowSlice := v.cards[i:end]

		// First pass: render at natural height and measure actual box height.
		contents := make([]string, len(rowSlice))
		rendered := make([]string, len(rowSlice))
		heights := make([]int, len(rowSlice))
		maxBoxHeight := 0
		for j, entry := range rowSlice {
			focused := v.actionHandler != nil && (i+j) == v.focusedCard
			contents[j] = v.renderCardContent(entry)
			rendered[j] = v.renderCardBox(contents[j], focused, 0)
			h := lipgloss.Height(rendered[j])
			heights[j] = h
			if h > maxBoxHeight {
				maxBoxHeight = h
			}
		}

		// Second pass: re-render cards shorter than the row maximum.
		// maxBoxHeight includes the top and bottom border lines (2), so the
		// content height to request is maxBoxHeight - 2.
		contentHeight := maxBoxHeight - 2
		for j := range rowSlice {
			if heights[j] < maxBoxHeight {
				focused := v.actionHandler != nil && (i+j) == v.focusedCard
				rendered[j] = v.renderCardBox(contents[j], focused, contentHeight)
			}
		}

		// Add a right gap to every card except the last in the row.
		gapped := make([]string, len(rendered))
		for j, c := range rendered {
			if j < len(rendered)-1 {
				gapped[j] = gap.Render(c)
			} else {
				gapped[j] = c
			}
		}
		gridRows = append(gridRows, lipgloss.JoinHorizontal(lipgloss.Top, gapped...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, gridRows...)
}
