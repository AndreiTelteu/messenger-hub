package model

import (
	"errors"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"example.com/chat", "https://example.com/chat", true},
		{" http://localhost:8080/a ", "http://localhost:8080/a", true},
		{"HTTPS://Example.com/inbox", "https://Example.com/inbox", true},
		{"ftp://example.com", "", false},
		{"https:///missing-host", "", false},
		{"javascript:alert(1)", "", false},
		{"data:text/plain,hello", "", false},
		{"https://example..com", "", false},
		{"https://.example.com", "", false},
		{"https://example.com/a b", "", false},
		{"https://example.com/a\tb", "", false},
		{"https://user@example.com", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if (err == nil) != tt.ok || got != tt.want {
				t.Fatalf("NormalizeURL() = %q, %v; want %q, ok=%v", got, err, tt.want, tt.ok)
			}
		})
	}
}

func TestStateTransitionsPreserveSidebarPositions(t *testing.T) {
	state := NewState()
	first := mustService(t, "First", "one.example")
	second := mustService(t, "Second", "two.example")
	third := mustService(t, "Third", "three.example")
	for _, service := range []Service{first, second, third} {
		if err := state.Add(service); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.SetEnabled(second.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := state.SelectPosition(2); !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("select disabled position: %v", err)
	}
	if err := state.SelectPosition(3); err != nil {
		t.Fatal(err)
	}
	if state.ActiveID != third.ID {
		t.Fatalf("active ID = %q, want %q", state.ActiveID, third.ID)
	}
	if err := state.Move(third.ID, 1); err != nil {
		t.Fatal(err)
	}
	if got := []string{state.Services[0].ID, state.Services[1].ID, state.Services[2].ID}; got[0] != third.ID || got[1] != first.ID || got[2] != second.ID {
		t.Fatalf("unexpected order: %v", got)
	}
	if state.Services[2].Enabled {
		t.Fatal("move changed disabled state")
	}
	if err := state.Remove(third.ID); err != nil {
		t.Fatal(err)
	}
	if state.ActiveID != first.ID {
		t.Fatalf("active after removal = %q, want %q", state.ActiveID, first.ID)
	}
}

func TestStateBoundaryErrorsDoNotMutate(t *testing.T) {
	state := NewState()
	service := mustService(t, "Chat", "chat.example")
	if err := state.Add(service); err != nil {
		t.Fatal(err)
	}
	before := state
	before.Services = append([]Service(nil), state.Services...)
	if err := state.Move(service.ID, 0); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("move position zero: %v", err)
	}
	if err := state.SelectPosition(2); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("select beyond end: %v", err)
	}
	if err := state.Remove("00000000000000000000000000000000"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("remove missing: %v", err)
	}
	if state.ActiveID != before.ActiveID || len(state.Services) != 1 || state.Services[0] != before.Services[0] {
		t.Fatalf("boundary errors mutated state: %#v", state)
	}
}

func mustService(t *testing.T, name, rawURL string) Service {
	t.Helper()
	service, err := NewService(name, "custom", rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
