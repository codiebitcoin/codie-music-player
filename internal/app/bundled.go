package app

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// ensureBundledPlugins copies the plugins the app ships with into the plugins
// folder, each one that isn't there yet. They are ordinary plugin folders
// after that: the listener can update or remove them like any other.
func ensureBundledPlugins(bundled fs.FS) {
	if bundled == nil {
		return
	}
	names, err := fs.ReadDir(bundled, "plugins")
	if err != nil {
		return
	}
	for _, e := range names {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if _, err := os.Stat(filepath.Join(pluginsDir(), id, "plugin.json")); err == nil {
			continue // already there
		}
		if err := fs.WalkDir(bundled, filepath.Join("plugins", id), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, rerr := filepath.Rel("plugins", path)
			if rerr != nil {
				return rerr
			}
			dest := filepath.Join(pluginsDir(), rel)
			if d.IsDir() {
				return os.MkdirAll(dest, 0o700)
			}
			data, rerr := fs.ReadFile(bundled, path)
			if rerr != nil {
				return rerr
			}
			if werr := os.MkdirAll(filepath.Dir(dest), 0o700); werr != nil {
				return werr
			}
			return os.WriteFile(dest, data, 0o600)
		}); err != nil {
			log.Printf("bundled plugin %s: %v", id, err)
			continue
		}
		log.Printf("installed bundled plugin %s", id)
	}
}
