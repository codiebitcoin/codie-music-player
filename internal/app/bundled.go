package app

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// bundledRoot is a shipped plugin's path inside the embedded files. It stays
// slash-separated: on Windows filepath.Join would make plugins\netease, and
// fs.WalkDir would not find it, so the first launch would install none of the
// sources the app ships.
func bundledRoot(id string) string {
	return path.Join("plugins", id)
}

// ensureBundledPlugins copies the plugins the app ships with into the plugins
// folder, each one that isn't there yet. They are ordinary plugin folders
// after that: the listener can update or remove them like any other.
func ensureBundledPlugins(bundled fs.FS) {
	installBundledPlugins(bundled, pluginsDir())
}

// installBundledPlugins copies each shipped plugin that destRoot doesn't
// already have. One already there is left alone, so an update of that plugin
// isn't replaced by the copy the app shipped with.
func installBundledPlugins(bundled fs.FS, destRoot string) {
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
		if _, err := os.Stat(filepath.Join(destRoot, id, "plugin.json")); err == nil {
			continue // already there
		}
		if err := fs.WalkDir(bundled, bundledRoot(id), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, ok := strings.CutPrefix(p, "plugins/")
			if !ok || rel == "" || strings.Contains(rel, "..") {
				return fmt.Errorf("bundled path %q is outside plugins", p)
			}
			dest := filepath.Join(destRoot, filepath.FromSlash(rel))
			if d.IsDir() {
				return os.MkdirAll(dest, 0o700)
			}
			data, rerr := fs.ReadFile(bundled, p)
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
