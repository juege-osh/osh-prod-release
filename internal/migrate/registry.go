package migrate

import (
	"fmt"
	"os"
	"path/filepath"
)

// Registry loads component manifests and migration units (P2 skeleton).
type Registry struct {
	Root string
}

func New(root string) *Registry {
	return &Registry{Root: root}
}

type ComponentManifest struct {
	Name           string `yaml:"name"`
	Kind           string `yaml:"kind"`
	DataDir        string `yaml:"data_dir"`
	ConfigDir      string `yaml:"config_dir"`
	ComposeFragment string `yaml:"compose_fragment"`
}

func (r *Registry) ListComponents() ([]string, error) {
	entries, err := os.ReadDir(r.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

func (r *Registry) ValidateComponent(name string) error {
	path := filepath.Join(r.Root, name)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("component manifest missing: %s", name)
	}
	return nil
}
