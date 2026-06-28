package plugin

import "fmt"

// Resolved is one manifest in the resolved install order. Name is the catalog
// key (the plugin's identity for lookup, requires, and diagnostics); TOML is its
// raw manifest body.
type Resolved struct {
	Name string
	TOML string
}

// ResolveOrder takes a set of root plugin names and a loader that fetches
// manifest TOML by name (from the DB). It walks the Requires graph, pulls
// in all transitive dependencies, and returns manifests in topological
// order (dependencies before dependents).
func ResolveOrder(roots []string, load func(string) (string, error)) ([]Resolved, error) {
	type node struct {
		requires []string
		toml     string
	}

	nodes := map[string]node{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, ok := nodes[name]; ok {
			continue
		}
		raw, err := load(name)
		if err != nil {
			return nil, fmt.Errorf("load plugin %q: %w", name, err)
		}
		m, err := ParseManifest(raw)
		if err != nil {
			return nil, err
		}
		nodes[name] = node{requires: m.Requires, toml: raw}
		queue = append(queue, m.Requires...)
	}

	inDegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string)
	for name, n := range nodes {
		for _, dep := range n.requires {
			dependents[dep] = append(dependents[dep], name)
			inDegree[name]++
		}
	}

	var ready []string
	for name := range nodes {
		if inDegree[name] == 0 {
			ready = append(ready, name)
		}
	}

	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		order = append(order, name)
		for _, dep := range dependents[name] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}

	if len(order) != len(nodes) {
		return nil, fmt.Errorf("cycle in plugin dependencies")
	}

	result := make([]Resolved, len(order))
	for i, name := range order {
		result[i] = Resolved{Name: name, TOML: nodes[name].toml}
	}
	return result, nil
}
