package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var explicitSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

const CurrentVersion = 1

var (
	ErrInvalidPosition = errors.New("invalid service position")
	ErrServiceNotFound = errors.New("service not found")
	ErrServiceDisabled = errors.New("service is disabled")
)

type Service struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type WindowGeometry struct {
	Width      int  `json:"width,omitempty"`
	Height     int  `json:"height,omitempty"`
	X          int  `json:"x,omitempty"`
	Y          int  `json:"y,omitempty"`
	Positioned bool `json:"positioned,omitempty"`
	Maximized  bool `json:"maximized,omitempty"`
}

type State struct {
	Version  int            `json:"version"`
	Services []Service      `json:"services"`
	ActiveID string         `json:"active_id,omitempty"`
	Window   WindowGeometry `json:"window,omitempty"`
}

func NewState() State { return State{Version: CurrentVersion, Services: []Service{}} }

func NewService(name, kind, rawURL string) (Service, error) {
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	if name == "" {
		return Service{}, errors.New("service name is required")
	}
	if kind == "" {
		return Service{}, errors.New("service kind is required")
	}
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return Service{}, err
	}
	id, err := newID()
	if err != nil {
		return Service{}, fmt.Errorf("generate service ID: %w", err)
	}
	return Service{ID: id, Name: name, Kind: kind, URL: normalized, Enabled: true}, nil
}

func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("service URL is required")
	}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", errors.New("service URL must not contain spaces or control characters")
	}
	if match := explicitSchemePattern.FindString(raw); match != "" {
		scheme := strings.TrimSuffix(match, ":")
		if !strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https") {
			return "", errors.New("service URL must use http or https")
		}
		raw = strings.ToLower(scheme) + raw[len(scheme):]
	} else {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid service URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("service URL must use http or https")
	}
	if u.Host == "" || u.User != nil || u.Hostname() == "" {
		return "", errors.New("service URL must contain a valid host")
	}
	host := u.Hostname()
	if host == "." || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return "", errors.New("service URL must contain a valid host")
	}
	if _, err := url.ParseRequestURI(u.String()); err != nil {
		return "", fmt.Errorf("invalid service URL: %w", err)
	}
	return u.String(), nil
}

func (s *State) Add(service Service) error {
	if err := validateService(service); err != nil {
		return err
	}
	if s.index(service.ID) >= 0 {
		return fmt.Errorf("duplicate service ID %q", service.ID)
	}
	s.Services = append(s.Services, service)
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	if s.ActiveID == "" && service.Enabled {
		s.ActiveID = service.ID
	}
	return nil
}

func (s *State) Remove(id string) error {
	i := s.index(id)
	if i < 0 {
		return ErrServiceNotFound
	}
	s.Services = append(s.Services[:i], s.Services[i+1:]...)
	if s.ActiveID == id {
		s.ActiveID = s.firstEnabledID()
	}
	return nil
}

func (s *State) Move(id string, position int) error {
	from := s.index(id)
	if from < 0 {
		return ErrServiceNotFound
	}
	if position < 1 || position > len(s.Services) {
		return ErrInvalidPosition
	}
	to := position - 1
	if from == to {
		return nil
	}
	item := s.Services[from]
	copy(s.Services[from:], s.Services[from+1:])
	s.Services = s.Services[:len(s.Services)-1]
	s.Services = append(s.Services, Service{})
	copy(s.Services[to+1:], s.Services[to:])
	s.Services[to] = item
	return nil
}

func (s *State) SetEnabled(id string, enabled bool) error {
	i := s.index(id)
	if i < 0 {
		return ErrServiceNotFound
	}
	s.Services[i].Enabled = enabled
	if !enabled && s.ActiveID == id {
		s.ActiveID = s.firstEnabledID()
	} else if enabled && s.ActiveID == "" {
		s.ActiveID = id
	}
	return nil
}

// SelectPosition activates the service at the one-based sidebar position.
// Disabled entries still occupy their position but cannot be selected.
func (s *State) SelectPosition(position int) error {
	if position < 1 || position > len(s.Services) {
		return ErrInvalidPosition
	}
	service := s.Services[position-1]
	if !service.Enabled {
		return ErrServiceDisabled
	}
	s.ActiveID = service.ID
	return nil
}

func (s State) Active() (Service, bool) {
	i := s.index(s.ActiveID)
	if i < 0 || !s.Services[i].Enabled {
		return Service{}, false
	}
	return s.Services[i], true
}

func (s State) index(id string) int {
	for i := range s.Services {
		if s.Services[i].ID == id {
			return i
		}
	}
	return -1
}

func (s State) firstEnabledID() string {
	for _, service := range s.Services {
		if service.Enabled {
			return service.ID
		}
	}
	return ""
}

func newID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}
