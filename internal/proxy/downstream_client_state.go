package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file defines the proxy-owned model of upstream client state that is
// mirrored into downstream connections for pooling and synchronization.

// DownstreamSamplingHandler forwards sampling requests from a downstream session.
type DownstreamSamplingHandler func(context.Context, *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error)

// DownstreamElicitationHandler forwards elicitation requests from a downstream session.
type DownstreamElicitationHandler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)

// DownstreamClientState describes the effective upstream client state mirrored downstream.
type DownstreamClientState struct {
	ProtocolVersion         string
	ClientCapabilities      *mcp.ClientCapabilities
	Roots                   []*mcp.Root
	CapabilitiesFingerprint string
	RootsFingerprint        string
}

// DownstreamConnectOptions configures downstream client setup.
type DownstreamConnectOptions struct {
	ForwardedHeaders   map[string]string
	ClientState        DownstreamClientState
	SamplingHandler    DownstreamSamplingHandler
	ElicitationHandler DownstreamElicitationHandler
}

func cloneDownstreamConnectOptions(options *DownstreamConnectOptions) *DownstreamConnectOptions {
	if options == nil {
		return &DownstreamConnectOptions{}
	}

	return &DownstreamConnectOptions{
		ForwardedHeaders: cloneAuthHeaders(options.ForwardedHeaders),
		ClientState: DownstreamClientState{
			ProtocolVersion:         options.ClientState.ProtocolVersion,
			ClientCapabilities:      cloneClientCapabilities(options.ClientState.ClientCapabilities),
			Roots:                   normalizeRoots(options.ClientState.Roots),
			CapabilitiesFingerprint: options.ClientState.CapabilitiesFingerprint,
			RootsFingerprint:        options.ClientState.RootsFingerprint,
		},
		SamplingHandler:    options.SamplingHandler,
		ElicitationHandler: options.ElicitationHandler,
	}
}

func normalizeClientCapabilities(capabilities *mcp.ClientCapabilities) *mcp.ClientCapabilities {
	if capabilities == nil {
		return &mcp.ClientCapabilities{}
	}

	return cloneClientCapabilities(capabilities)
}

func cloneClientCapabilities(capabilities *mcp.ClientCapabilities) *mcp.ClientCapabilities {
	if capabilities == nil {
		return nil
	}

	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return &mcp.ClientCapabilities{}
	}

	var cloned mcp.ClientCapabilities
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return &mcp.ClientCapabilities{}
	}
	if capabilities.RootsV2 != nil {
		rootsV2 := *capabilities.RootsV2
		cloned.RootsV2 = &rootsV2
		cloned.Roots.ListChanged = rootsV2.ListChanged
	}
	return &cloned
}

func normalizeRoots(roots []*mcp.Root) []*mcp.Root {
	if len(roots) == 0 {
		return nil
	}

	cloned := make([]*mcp.Root, 0, len(roots))
	for _, root := range roots {
		if root == nil {
			continue
		}
		rootCopy := *root
		cloned = append(cloned, &rootCopy)
	}

	sort.Slice(cloned, func(i, j int) bool {
		if cloned[i].URI == cloned[j].URI {
			return cloned[i].Name < cloned[j].Name
		}
		return cloned[i].URI < cloned[j].URI
	})
	return cloned
}

func fingerprintClientCapabilities(capabilities *mcp.ClientCapabilities) string {
	return fingerprintJSON(normalizeClientCapabilities(capabilities))
}

func fingerprintRoots(roots []*mcp.Root) string {
	return fingerprintJSON(normalizeRoots(roots))
}

func fingerprintJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "invalid"
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

func clientSupportsRoots(capabilities *mcp.ClientCapabilities) bool {
	if capabilities == nil {
		return false
	}
	return capabilities.RootsV2 != nil
}

func buildDownstreamClientState(protocolVersion string, capabilities *mcp.ClientCapabilities, roots []*mcp.Root) *DownstreamClientState {
	normalizedCapabilities := normalizeClientCapabilities(capabilities)
	normalizedRoots := normalizeRoots(roots)
	return &DownstreamClientState{
		ProtocolVersion:         protocolVersion,
		ClientCapabilities:      normalizedCapabilities,
		Roots:                   normalizedRoots,
		CapabilitiesFingerprint: fingerprintJSON(normalizedCapabilities),
		RootsFingerprint:        fingerprintJSON(normalizedRoots),
	}
}
