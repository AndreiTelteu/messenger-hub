// Messenger Hub is a native GTK4 application backed by the system WebKitGTK.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"messenger-hub/internal/model"
	"messenger-hub/internal/ui"
)

func main() { os.Exit(run()) }

func run() int {
	runtime.LockOSThread()
	profile := flag.String("profile", "", "use isolated configuration, data and cache under this directory")
	flag.Parse()
	store, err := openStore(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Messenger Hub:", err)
		return 1
	}
	release, err := store.AcquireLock()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Messenger Hub:", err)
		return 1
	}
	defer release()
	state, err := store.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Messenger Hub: cannot load settings:", err)
		return 1
	}
	return ui.Run(store, state, *profile == "")
}

func openStore(profile string) (*model.Store, error) {
	if profile != "" {
		root, err := filepath.Abs(profile)
		if err != nil {
			return nil, err
		}
		return model.NewStore(filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "cache")), nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		data = filepath.Join(home, ".local", "share")
	}
	for _, path := range []string{config, data, cache} {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("XDG directory must be absolute: %s", path)
		}
	}
	return model.NewStore(filepath.Join(config, "messenger-hub"), filepath.Join(data, "messenger-hub"), filepath.Join(cache, "messenger-hub")), nil
}
