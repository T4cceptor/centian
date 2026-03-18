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

// DownstreamLoggingHandler forwards logging notifications from a downstream session.
type DownstreamLoggingHandler func(context.Context, *mcp.LoggingMessageRequest)

// DownstreamResourceListChangedHandler forwards resource list change notifications from a downstream session.
type DownstreamResourceListChangedHandler func(context.Context, *mcp.ResourceListChangedRequest)

// DownstreamResourceUpdatedHandler forwards resource update notifications from a downstream session.
type DownstreamResourceUpdatedHandler func(context.Context, *mcp.ResourceUpdatedNotificationRequest)

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
	ForwardedHeaders           map[string]string
	ClientState                DownstreamClientState
	IdentityKey                string
	GatewayName                string
	SamplingHandler            DownstreamSamplingHandler
	ElicitationHandler         DownstreamElicitationHandler
	LoggingHandler             DownstreamLoggingHandler
	ResourceListChangedHandler DownstreamResourceListChangedHandler
	ResourceUpdatedHandler     DownstreamResourceUpdatedHandler
}

type clientCapabilitiesWire struct {
	Experimental map[string]any               `json:"experimental,omitempty"`
	Extensions   map[string]any               `json:"extensions,omitempty"`
	Roots        *mcp.RootCapabilities        `json:"roots,omitempty"`
	Sampling     *mcp.SamplingCapabilities    `json:"sampling,omitempty"`
	Elicitation  *mcp.ElicitationCapabilities `json:"elicitation,omitempty"`
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
		IdentityKey:                options.IdentityKey,
		GatewayName:                options.GatewayName,
		SamplingHandler:            options.SamplingHandler,
		ElicitationHandler:         options.ElicitationHandler,
		LoggingHandler:             options.LoggingHandler,
		ResourceListChangedHandler: options.ResourceListChangedHandler,
		ResourceUpdatedHandler:     options.ResourceUpdatedHandler,
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

	wire := clientCapabilitiesToWire(capabilities)
	encoded, err := json.Marshal(wire)
	if err != nil {
		return &mcp.ClientCapabilities{}
	}

	var clonedWire clientCapabilitiesWire
	if err := json.Unmarshal(encoded, &clonedWire); err != nil {
		return &mcp.ClientCapabilities{}
	}
	return clientCapabilitiesFromWire(&clonedWire)
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
	return fingerprintJSON(clientCapabilitiesToWire(normalizeClientCapabilities(capabilities)))
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

func clientCapabilitiesToWire(capabilities *mcp.ClientCapabilities) *clientCapabilitiesWire {
	if capabilities == nil {
		return &clientCapabilitiesWire{}
	}

	return &clientCapabilitiesWire{
		Experimental: capabilities.Experimental,
		Extensions:   capabilities.Extensions,
		Roots:        capabilities.RootsV2,
		Sampling:     capabilities.Sampling,
		Elicitation:  capabilities.Elicitation,
	}
}

func clientCapabilitiesFromWire(wire *clientCapabilitiesWire) *mcp.ClientCapabilities {
	if wire == nil {
		return &mcp.ClientCapabilities{}
	}

	return &mcp.ClientCapabilities{
		Experimental: wire.Experimental,
		Extensions:   wire.Extensions,
		RootsV2:      wire.Roots,
		Sampling:     wire.Sampling,
		Elicitation:  wire.Elicitation,
	}
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
