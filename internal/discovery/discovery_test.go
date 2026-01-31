package discovery

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

type stubDiscoverer struct {
	name        string
	description string
	available   bool
	servers     []Server
	err         error
}

func (s *stubDiscoverer) Name() string {
	return s.name
}

func (s *stubDiscoverer) Description() string {
	return s.description
}

func (s *stubDiscoverer) Discover() ([]Server, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.servers, nil
}

func (s *stubDiscoverer) IsAvailable() bool {
	return s.available
}

func TestGroupDiscoveryResults(t *testing.T) {
	// Given: discovery results with multiple source paths and a duplicate
	servers := []Server{
		{
			Name:       "alpha",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/path/one",
		},
		{
			Name:       "beta",
			URL:        "https://example.com",
			Transport:  "http",
			SourcePath: "/path/one",
		},
		{
			Name:       "gamma",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/path/two",
		},
		{
			Name:       "delta",
			Command:    "other",
			Transport:  "stdio",
			SourcePath: "/path/two",
		},
	}
	result := &Result{Servers: servers, Errors: []string{"warn"}}

	// When: grouping discovery results
	grouped := GroupDiscoveryResults(result)

	// Then: groups and counts are correct
	assert.Equal(t, len(grouped.Groups), 2)
	first := findGroup(grouped.Groups, "/path/one")
	second := findGroup(grouped.Groups, "/path/two")
	assert.Equal(t, first.TotalCount, 2)
	assert.Equal(t, first.StdioCount, 1)
	assert.Equal(t, first.HTTPCount, 1)
	assert.Equal(t, first.DuplicatesFound, 1)
	assert.Equal(t, second.TotalCount, 2)
	assert.Equal(t, second.StdioCount, 2)
	assert.Equal(t, second.HTTPCount, 0)
	assert.Equal(t, second.DuplicatesFound, 1)
	assert.Equal(t, len(grouped.Errors), 1)
}

func TestCountDuplicatesPerFile(t *testing.T) {
	// Given: servers with duplicate configs across files
	servers := []Server{
		{
			Name:       "one",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/path/one",
		},
		{
			Name:       "two",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/path/two",
		},
		{
			Name:       "three",
			Command:    "other",
			Transport:  "stdio",
			SourcePath: "/path/two",
		},
	}

	// When: counting duplicates per file
	counts := countDuplicatesPerFile(servers)

	// Then: each file with duplicates is counted
	assert.Equal(t, counts["/path/one"], 1)
	assert.Equal(t, counts["/path/two"], 1)
}

func TestDeduplicateServers(t *testing.T) {
	// Given: duplicate servers by name with identical and unique configs
	servers := []Server{
		{
			Name:       "alpha",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/short",
		},
		{
			Name:       "alpha",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: filepath.Join("/longer", "path"),
		},
		{
			Name:       "beta",
			Command:    "cmd",
			Transport:  "stdio",
			SourcePath: "/path/a",
		},
		{
			Name:       "beta",
			URL:        "https://example.com",
			Transport:  "http",
			SourcePath: "/path/b",
		},
	}

	// When: deduplicating servers
	deduped := deduplicateServers(servers)

	// Then: alpha keeps one entry with duplicates count
	alpha := findServerByName(deduped, "alpha")
	assert.Assert(t, alpha != nil)
	assert.Equal(t, alpha.DuplicatesFound, 1)
	assert.Equal(t, alpha.SourcePath, filepath.Join("/longer", "path"))

	// Then: beta splits into two unique configs with suffixes
	betaNames := findNamesByPrefix(deduped, "beta-")
	assert.Equal(t, len(betaNames), 2)
}

func TestManagerDiscoverAll(t *testing.T) {
	// Given: a manager with available and unavailable discoverers
	manager := &Manager{}
	manager.RegisterDiscoverer(&stubDiscoverer{
		name:        "available",
		description: "ok",
		available:   true,
		servers: []Server{{
			Name:      "alpha",
			Command:   "cmd",
			Transport: "stdio",
		}},
	})
	manager.RegisterDiscoverer(&stubDiscoverer{
		name:        "duplicate",
		description: "dup",
		available:   true,
		servers: []Server{{
			Name:      "alpha",
			Command:   "cmd",
			Transport: "stdio",
		}},
	})
	manager.RegisterDiscoverer(&stubDiscoverer{
		name:        "erroring",
		description: "bad",
		available:   true,
		err:         errors.New("boom"),
	})
	manager.RegisterDiscoverer(&stubDiscoverer{
		name:        "unavailable",
		description: "skip",
		available:   false,
	})

	// When: discovering all
	result := manager.DiscoverAll()

	// Then: duplicates are removed and errors captured
	assert.Equal(t, len(result.Servers), 1)
	assert.Equal(t, result.Servers[0].DuplicatesFound, 1)
	assert.Equal(t, len(result.Errors), 1)
}

func TestListDiscoverers(t *testing.T) {
	// Given: a manager with one discoverer
	manager := &Manager{}
	manager.RegisterDiscoverer(&stubDiscoverer{
		name:        "example",
		description: "desc",
		available:   true,
	})

	// When: listing discoverers
	list := manager.ListDiscoverers()

	// Then: metadata is returned
	assert.Equal(t, len(list), 1)
	assert.Equal(t, list[0]["name"], "example")
	assert.Equal(t, list[0]["description"], "desc")
	assert.Equal(t, list[0]["available"], "true")
}

func findGroup(groups []ConfigFileGroup, sourcePath string) ConfigFileGroup {
	for i := range groups {
		if groups[i].SourcePath == sourcePath {
			return groups[i]
		}
	}
	return ConfigFileGroup{}
}

func findServerByName(servers []Server, name string) *Server {
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i]
		}
	}
	return nil
}

func findNamesByPrefix(servers []Server, prefix string) []string {
	var names []string
	for i := range servers {
		if strings.HasPrefix(servers[i].Name, prefix) {
			names = append(names, servers[i].Name)
		}
	}
	return names
}
