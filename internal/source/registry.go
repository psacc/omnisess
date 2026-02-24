package source

import "github.com/psacc/omnisess/internal/model"

var registry []Source

// Register adds a source to the global registry.
// Called from each source's init() function.
func Register(s Source) {
	registry = append(registry, s)
}

// All returns all registered sources.
func All() []Source {
	return registry
}

// ByName returns sources matching the given tool name, or all if empty.
func ByName(name model.Tool) []Source {
	if name == "" {
		return registry
	}
	var filtered []Source
	for _, s := range registry {
		if s.Name() == name {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
