package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// GreeterServiceImpl is a simple in-process implementation of GreeterService.
type GreeterServiceImpl struct {
	UnimplementedGreeterServiceServer
}

// Greet returns a greeting for the given name.
func (s *GreeterServiceImpl) Greet(_ context.Context, req *GreetRequest) (*GreetResponse, error) {
	msg := "Hello, " + req.Name + "!"
	if req.Loud {
		msg = strings.ToUpper(msg)
	}

	var sb strings.Builder
	repeat := int(req.Repeat)
	if repeat <= 0 {
		repeat = 1
	}
	for i := 0; i < repeat; i++ {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(msg)
	}

	return &GreetResponse{Message: sb.String()}, nil
}

// ListGreetings returns greetings for multiple names.
func (s *GreeterServiceImpl) ListGreetings(_ context.Context, req *ListGreetingsRequest) (*ListGreetingsResponse, error) {
	msgs := make([]string, 0, len(req.Names))
	for _, name := range req.Names {
		msgs = append(msgs, "Hello, "+strings.TrimSpace(name)+"!")
	}
	return &ListGreetingsResponse{Messages: msgs}, nil
}

// HiddenMethod is a method that is hidden from the TUI but accessible via CLI.
func (s *GreeterServiceImpl) HiddenMethod(_ context.Context, req *GreetRequest) (*GreetResponse, error) {
	return &GreetResponse{Message: "secret greeting for " + req.Name}, nil
}

// ScheduleCall confirms the call and echoes the time in both local and UTC.
func (s *GreeterServiceImpl) ScheduleCall(_ context.Context, req *ScheduleCallRequest) (*ScheduleCallResponse, error) {
	if req.When == nil {
		return nil, fmt.Errorf("when is required")
	}
	t := req.When.AsTime()
	return &ScheduleCallResponse{
		Message:   fmt.Sprintf("Call with %s confirmed!", req.With),
		WhenLocal: t.Local().Format(time.RFC3339), // server's local timezone
		WhenUtc:   t.UTC().Format(time.RFC3339),   // same instant in UTC
	}, nil
}

// ColoredGreet returns a greeting with a hex color code derived from the requested RgbColor.
func (s *GreeterServiceImpl) ColoredGreet(_ context.Context, req *ColoredGreetRequest) (*ColoredGreetResponse, error) {
	colorHex := "#000000"
	if req.Color != nil {
		r := clamp(int(req.Color.R), 0, 255)
		g := clamp(int(req.Color.G), 0, 255)
		b := clamp(int(req.Color.B), 0, 255)
		colorHex = fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	return &ColoredGreetResponse{
		Message:  fmt.Sprintf("Hello, %s!", req.Name),
		ColorHex: colorHex,
	}, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DirectoryServiceImpl is a simple in-process implementation of DirectoryService.
type DirectoryServiceImpl struct {
	UnimplementedDirectoryServiceServer
}

// people is the hardcoded contact directory.
var people = []*PersonCard{
	// A — two entries so filter "a" returns multiple results
	{Name: "Alice Nakamura", Bio: "Principal engineer · distributed systems · she/her"},
	{Name: "Amir Osei", Bio: "Backend engineer · Rust enthusiast · he/him"},
	// B
	{Name: "Bob Tremblay", Bio: "Product designer · motion & interaction · he/him"},
	{Name: "Beatriz Santos", Bio: "iOS engineer · accessibility advocate · she/her"},
	// C
	{Name: "Charlie Okonkwo", Bio: "Engineering manager · team of 12 · he/him"},
	{Name: "Chloe Andersson", Bio: "ML engineer · recommendation systems · she/her"},
	// D
	{Name: "Diana Ferreira", Bio: "Staff SRE · guardian of uptime · she/her"},
	// E
	{Name: "Eve Larsson", Bio: "Security researcher · trust no one · she/her"},
	// F
	{Name: "Frank Dubois", Bio: "Data scientist · charts and dashboards · he/him"},
	// G — tests multi-word bio wrapping in narrow cards
	{Name: "Grace Kim", Bio: "Platform engineer · internal tooling · she/her"},
	// H
	{Name: "Hiroshi Patel", Bio: "DevRel engineer · conference speaker · he/him"},
	// I — single entry to test a lone result
	{Name: "Imani Reyes", Bio: "UX researcher · mixed-methods · she/her"},
	// J — tests long name in card header
	{Name: "Jean-Baptiste Müller", Bio: "Infra lead · multi-cloud · he/him"},
	// K
	{Name: "Keiko Andrade", Bio: "QA engineer · chaos testing · she/her"},
	// L
	{Name: "Luca Obi", Bio: "Full-stack engineer · GraphQL · he/him"},
	// M — two entries
	{Name: "Maya Johansson", Bio: "Staff engineer · observability · she/her"},
	{Name: "Marcus Webb", Bio: "Solutions architect · financial services · he/him"},
	// N
	{Name: "Naledi Eriksson", Bio: "Product manager · growth · she/her"},
	// O
	{Name: "Omar Petrov", Bio: "Embedded engineer · IoT · he/him"},
	// P
	{Name: "Priya Nguyen", Bio: "Data engineer · streaming pipelines · she/her"},
}

// ListPeople streams the contact directory, applying an optional name-prefix filter.
func (s *DirectoryServiceImpl) ListPeople(req *ListPeopleRequest, stream grpc.ServerStreamingServer[PersonCard]) error {
	filter := strings.ToLower(strings.TrimSpace(req.Filter))
	for _, p := range people {
		if filter == "" || strings.HasPrefix(strings.ToLower(p.Name), filter) {
			if err := stream.Send(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// FarewellServiceImpl is a simple in-process implementation of FarewellService.
type FarewellServiceImpl struct {
	UnimplementedFarewellServiceServer
}

// Farewell bids goodbye to a person.
func (s *FarewellServiceImpl) Farewell(_ context.Context, req *FarewellRequest) (*FarewellResponse, error) {
	var msg string
	if req.Formal {
		msg = "Farewell, " + req.Name + ". Until we meet again."
	} else {
		msg = "Bye, " + req.Name + "!"
	}
	return &FarewellResponse{Message: msg}, nil
}

// FarewellMany bids goodbye to multiple people.
func (s *FarewellServiceImpl) FarewellMany(_ context.Context, req *FarewellManyRequest) (*FarewellManyResponse, error) {
	msgs := make([]string, 0, len(req.Names))
	for _, name := range req.Names {
		msgs = append(msgs, "Bye, "+strings.TrimSpace(name)+"!")
	}
	return &FarewellManyResponse{Messages: msgs}, nil
}

// LeaveNote echoes the note back with the recipient's name.
func (s *FarewellServiceImpl) LeaveNote(_ context.Context, req *NoteRequest) (*NoteResponse, error) {
	return &NoteResponse{
		Message:  fmt.Sprintf("Note left for %s.", req.Name),
		Metadata: req.Metadata,
	}, nil
}

// CountdownFarewell streams a countdown then a final farewell message.
func (s *FarewellServiceImpl) CountdownFarewell(req *CountdownFarewellRequest, stream grpc.ServerStreamingServer[CountdownFarewellResponse]) error {
	from := int(req.From)
	if from <= 0 {
		from = 5
	}
	delayMs := int(req.DelayMs)
	if delayMs <= 0 {
		delayMs = 500
	}
	for i := from; i > 0; i-- {
		if err := stream.Send(&CountdownFarewellResponse{
			Message: fmt.Sprintf("%d…", i),
		}); err != nil {
			return err
		}
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}
	return stream.Send(&CountdownFarewellResponse{
		Message: fmt.Sprintf("Goodbye, %s! 👋", req.Name),
	})
}

// ScheduledFarewell schedules a farewell, echoing back the time and address.
func (s *FarewellServiceImpl) ScheduledFarewell(_ context.Context, req *ScheduledFarewellRequest) (*ScheduledFarewellResponse, error) {
	scheduledFor := "(no time specified)"
	if req.SendAt != nil {
		scheduledFor = req.SendAt.AsTime().UTC().Format(time.RFC3339)
	}

	deliveryAddr := "(no address specified)"
	if req.Address != nil && (req.Address.Street != "" || req.Address.City != "") {
		parts := []string{}
		if req.Address.Street != "" {
			parts = append(parts, req.Address.Street)
		}
		if req.Address.City != "" {
			parts = append(parts, req.Address.City)
		}
		deliveryAddr = strings.Join(parts, ", ")
	}

	return &ScheduledFarewellResponse{
		Message:         fmt.Sprintf("Farewell, %s!", req.Name),
		ScheduledFor:    scheduledFor,
		DeliveryAddress: deliveryAddr,
	}, nil
}
