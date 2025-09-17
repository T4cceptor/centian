package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/CentianAI/centian-cli/internal/config"
)

// Basic MCP message structures
type MCPMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ProxyConfig struct {
	ServerCommand string   `json:"server_command"`
	ServerArgs    []string `json:"server_args"`
}

type MCPProxy struct {
	config    *ProxyConfig
	serverCmd *exec.Cmd
	serverIn  io.WriteCloser
	serverOut io.ReadCloser
	clientIn  io.Reader
	clientOut io.Writer
	metadata  interface{} // Store server metadata
	mu        sync.Mutex
}

func NewMCPProxy(config *ProxyConfig) *MCPProxy {
	return &MCPProxy{
		config:    config,
		clientIn:  os.Stdin,
		clientOut: os.Stdout,
	}
}

// Start the MCP server process and establish stdio connection
func (p *MCPProxy) StartServer() error {
	p.serverCmd = exec.Command(p.config.ServerCommand, p.config.ServerArgs...)

	// Setup stdio pipes
	serverStdin, err := p.serverCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create server stdin pipe: %w", err)
	}
	p.serverIn = serverStdin

	serverStdout, err := p.serverCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create server stdout pipe: %w", err)
	}
	p.serverOut = serverStdout

	// Start the server process
	if err := p.serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	log.Printf("Started MCP server: %s %v (PID: %d)",
		p.config.ServerCommand, p.config.ServerArgs, p.serverCmd.Process.Pid)

	return nil
}

// Initialize connection and fetch server metadata
func (p *MCPProxy) Initialize() error {
	// Send initialize request to server
	initRequest := MCPMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"roots": map[string]interface{}{
					"listChanged": true,
				},
				"sampling": map[string]interface{}{},
			},
			"clientInfo": map[string]interface{}{
				"name":    "centian-proxy",
				"version": "1.0.0",
			},
		},
	}

	// Send to server
	if err := p.sendToServer(initRequest); err != nil {
		return fmt.Errorf("failed to send initialize request: %w", err)
	}

	// Read response from server
	response, err := p.readFromServer()
	if err != nil {
		return fmt.Errorf("failed to read initialize response: %w", err)
	}

	// Store metadata for later use/modification
	p.mu.Lock()
	p.metadata = response.Result
	p.mu.Unlock()

	log.Printf("Server initialization successful, capabilities: %+v", response.Result)
	return nil
}

// Send JSON-RPC message to server
func (p *MCPProxy) sendToServer(msg MCPMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	data = append(data, '\n') // MCP uses newline-delimited JSON
	_, err = p.serverIn.Write(data)
	return err
}

// Read JSON-RPC message from server
func (p *MCPProxy) readFromServer() (*MCPMessage, error) {
	scanner := bufio.NewScanner(p.serverOut)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	var msg MCPMessage
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// Send JSON-RPC message to client
func (p *MCPProxy) sendToClient(msg MCPMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = p.clientOut.Write(data)
	return err
}

// Read JSON-RPC message from client
func (p *MCPProxy) readFromClient() (*MCPMessage, error) {
	scanner := bufio.NewScanner(p.clientIn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	var msg MCPMessage
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// Handle the initialization flow
func (p *MCPProxy) HandleInitialization() error {
	// Read initialize request from client
	clientMsg, err := p.readFromClient()
	if err != nil {
		return fmt.Errorf("failed to read client initialize: %w", err)
	}

	if clientMsg.Method != "initialize" {
		return fmt.Errorf("expected initialize method, got: %s", clientMsg.Method)
	}

	log.Printf("Received initialize from client: %+v", clientMsg)

	// Forward to server (you could modify the request here)
	if err := p.sendToServer(*clientMsg); err != nil {
		return fmt.Errorf("failed to forward initialize to server: %w", err)
	}

	// Read server response
	serverResp, err := p.readFromServer()
	if err != nil {
		return fmt.Errorf("failed to read server response: %w", err)
	}

	// Store and potentially modify metadata
	p.mu.Lock()
	p.metadata = serverResp.Result
	// Here you could modify the metadata if needed
	// serverResp.Result = p.modifyMetadata(serverResp.Result)
	p.mu.Unlock()

	// Forward response to client
	if err := p.sendToClient(*serverResp); err != nil {
		return fmt.Errorf("failed to send response to client: %w", err)
	}

	log.Printf("Initialize handshake completed")
	return nil
}

// Start the proxy loop (after initialization)
func (p *MCPProxy) Run() error {
	// Handle initialization first
	if err := p.HandleInitialization(); err != nil {
		return err
	}

	// Start goroutines to handle bidirectional communication
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// Client -> Server
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msg, err := p.readFromClient()
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("client read error: %w", err)
				}
				return
			}

			// Here you can intercept and modify messages
			log.Printf("Client -> Server: %s", msg.Method)

			if err := p.sendToServer(*msg); err != nil {
				errCh <- fmt.Errorf("server write error: %w", err)
				return
			}
		}
	}()

	// Server -> Client
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msg, err := p.readFromServer()
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("server read error: %w", err)
				}
				return
			}

			// Here you can intercept and modify responses
			log.Printf("Server -> Client: method=%s, id=%v", msg.Method, msg.ID)

			if err := p.sendToClient(*msg); err != nil {
				errCh <- fmt.Errorf("client write error: %w", err)
				return
			}
		}
	}()

	// Wait for either completion or error
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Return first error or nil when done
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

// Cleanup resources
func (p *MCPProxy) Close() error {
	log.Printf("Shutting down proxy...")

	if p.serverIn != nil {
		p.serverIn.Close()
	}
	if p.serverOut != nil {
		p.serverOut.Close()
	}

	if p.serverCmd != nil && p.serverCmd.Process != nil {
		if err := p.serverCmd.Process.Kill(); err != nil {
			log.Printf("Error killing server process: %v", err)
		}
		p.serverCmd.Wait()
	}

	return nil
}

func StartCentianProxy() error {
	// Check if config exists first
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	// Load global configuration
	globalConfig, err := config.LoadConfig()
	if err != nil {
		// Check if it's a missing config file specifically
		if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
			return fmt.Errorf(`❌ Centian configuration missing!

Centian requires initialization before it can start.

Please run:
  centian init

This will guide you through the setup process and create a default configuration at %s
Then you can add MCP servers and start the proxy.

For help: centian init --help`, configPath)
		}
		return fmt.Errorf("failed to load configuration from %s: %w", configPath, err)
	}

	// For now, use the first enabled server
	// TODO: Add server selection logic
	enabledServers := globalConfig.ListEnabledServers()
	if len(enabledServers) == 0 {
		return fmt.Errorf(`❌ No enabled MCP servers found!

Your configuration exists but has no enabled servers.

To add a server:
  centian config server add --name "my-server" --command "npx" --args "-y,@upstash/context7-mcp"

To list existing servers:
  centian config server list

To enable an existing server:
  centian config server enable --name "server-name"`)
	}

	serverName := enabledServers[0] // TODO: this should be changed
	server, err := globalConfig.GetServer(serverName)
	if err != nil {
		return fmt.Errorf("failed to get server configuration: %w", err)
	}

	log.Printf("Starting MCP proxy with server: %s", serverName)

	// Create proxy config from server config
	proxyConfig := &ProxyConfig{
		ServerCommand: server.Command,
		ServerArgs:    server.Args,
	}

	proxy := NewMCPProxy(proxyConfig)
	defer proxy.Close()

	// Start the MCP server
	if err := proxy.StartServer(); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	// Initialize connection with server
	if err := proxy.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %v", err)
	}

	// Start proxying
	log.Printf("Starting MCP proxy...")
	if err := proxy.Run(); err != nil {
		return fmt.Errorf("proxy error: %v", err)
	}
	return nil
}
