package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
)

var serviceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var ErrProfileLocked = errors.New("profile is already in use")

type Store struct {
	configDir string
	dataDir   string
	cacheDir  string
}

func NewStore(configDir, dataDir, cacheDir string) *Store {
	return &Store{configDir: configDir, dataDir: dataDir, cacheDir: cacheDir}
}

// AcquireLock takes an advisory, process-wide lock for this profile. The
// returned function releases it and is safe to call more than once. The lock
// file is deliberately retained because removing it can allow two processes
// to lock different inodes for the same profile.
func (s *Store) AcquireLock() (release func(), err error) {
	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(s.configDir, 0700); err != nil {
		return nil, fmt.Errorf("secure config directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(s.configDir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open profile lock: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure profile lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrProfileLocked
		}
		return nil, fmt.Errorf("lock profile: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
		})
	}, nil
}

func (s *Store) Load() (State, error) {
	path := filepath.Join(s.configDir, "state.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var state State
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("decode state %q: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return State{}, fmt.Errorf("decode state %q: trailing content: %w", path, err)
	}
	if err := validateState(state); err != nil {
		return State{}, fmt.Errorf("invalid state %q: %w", path, err)
	}
	return state, nil
}

func (s *Store) Save(state State) error {
	if err := validateState(state); err != nil {
		return fmt.Errorf("save invalid state: %w", err)
	}
	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(s.configDir, 0700); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.configDir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return fmt.Errorf("secure temporary state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(s.configDir, "state.json")); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	ok = true
	return nil
}

func (s *Store) DataProfileDir(serviceID string) (string, error) {
	if !serviceIDPattern.MatchString(serviceID) {
		return "", errors.New("invalid service ID")
	}
	return filepath.Join(s.dataDir, "profiles", serviceID), nil
}

func (s *Store) CacheProfileDir(serviceID string) (string, error) {
	if !serviceIDPattern.MatchString(serviceID) {
		return "", errors.New("invalid service ID")
	}
	return filepath.Join(s.cacheDir, "profiles", serviceID), nil
}

func validateState(state State) error {
	const (
		minX11Coordinate = -1 << 31
		maxX11Coordinate = 1<<31 - 1
	)
	if state.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %d", state.Version)
	}
	if (state.Window.Width == 0) != (state.Window.Height == 0) {
		return errors.New("window width and height must be stored together")
	}
	if state.Window.Width != 0 && (state.Window.Width < 800 || state.Window.Height < 600) {
		return errors.New("window size is below the application minimum")
	}
	if state.Window.Width > maxX11Coordinate || state.Window.Height > maxX11Coordinate {
		return errors.New("window size is outside the supported range")
	}
	if state.Window.Positioned && (state.Window.X < minX11Coordinate || state.Window.X > maxX11Coordinate || state.Window.Y < minX11Coordinate || state.Window.Y > maxX11Coordinate) {
		return errors.New("window position is outside the supported coordinate range")
	}
	seen := make(map[string]struct{}, len(state.Services))
	for _, service := range state.Services {
		if err := validateService(service); err != nil {
			return fmt.Errorf("service %q: %w", service.ID, err)
		}
		if _, exists := seen[service.ID]; exists {
			return fmt.Errorf("duplicate service ID %q", service.ID)
		}
		seen[service.ID] = struct{}{}
	}
	if state.ActiveID != "" {
		i := state.index(state.ActiveID)
		if i < 0 {
			return errors.New("active service does not exist")
		}
		if !state.Services[i].Enabled {
			return errors.New("active service is disabled")
		}
	}
	return nil
}

func validateService(service Service) error {
	if !serviceIDPattern.MatchString(service.ID) {
		return errors.New("invalid service ID")
	}
	if strings.TrimSpace(service.Name) == "" {
		return errors.New("service name is required")
	}
	if strings.TrimSpace(service.Kind) == "" {
		return errors.New("service kind is required")
	}
	normalized, err := NormalizeURL(service.URL)
	if err != nil {
		return err
	}
	if normalized != service.URL {
		return errors.New("service URL is not normalized")
	}
	return nil
}
