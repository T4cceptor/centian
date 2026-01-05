Logging Integration Outline

  1. Logger Setup (Add to CentianServer struct)

  Location: Lines 58-62 (struct definition)

  Add these fields to CentianServer:
  type CentianServer struct {
      config         *CentianConfig
      mux            *http.ServeMux
      server         *http.Server
      logger         *logging.Logger    // Add: for request/response logging
      sessionID      string              // Add: unique session identifier
      processorChain *processor.Chain    // Add: optional processor chain
  }

  Location: Lines 76-89 (NewCentianHTTPProxy)

  Initialize logger in constructor:
  func NewCentianHTTPProxy(config *CentianConfig) (*CentianServer, error) {
      mux := http.NewServeMux()
      server := &http.Server{
          Addr:         ":" + config.ProxyConfiguration.Port,
          Handler:      mux,
          ReadTimeout:  getSecondsFromInt(config.ProxyConfiguration.Timeout),
          WriteTimeout: getSecondsFromInt(config.ProxyConfiguration.Timeout),
      }

      // Create logger
      logger, err := logging.NewLogger()
      if err != nil {
          return nil, fmt.Errorf("failed to create logger: %w", err)
      }

      // Generate session ID
      timestamp := time.Now().UnixNano()
      sessionID := fmt.Sprintf("http_session_%d", timestamp)

      return &CentianServer{
          config:    config,
          mux:       mux,
          server:    server,
          logger:    logger,
          sessionID: sessionID,
      }, nil
  }

  ---
  2. Request Logging (Director Hook)

  Location: Lines 144-152 (inside RegisterProxy)

  Replace TODO at line 145 with request logging:

  proxy.Director = func(r *http.Request) {
      // Generate unique request ID
      requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())

      // Create server ID for this specific downstream server
      serverID := fmt.Sprintf("http_%s_%s_%s", gatewayName, serverName, endpoint)

      // Log the incoming client request BEFORE forwarding
      if c.logger != nil {
          // Read and log request body (for MCP JSON-RPC)
          bodyBytes, err := io.ReadAll(r.Body)
          if err == nil {
              requestBody := string(bodyBytes)

              // Log to file
              _ = c.logger.LogRequest(requestID, c.sessionID, endpoint, nil, serverID, requestBody)

              // Debug output to stderr
              fmt.Fprintf(os.Stderr, "[CLIENT->SERVER] [%s] %s\n", endpoint, requestBody)

              // Restore body for forwarding (important!)
              r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
          }
      }

      // Store requestID in context for response logging
      ctx := context.WithValue(r.Context(), "requestID", requestID)
      ctx = context.WithValue(ctx, "serverID", serverID)
      ctx = context.WithValue(ctx, "endpoint", endpoint)
      *r = *r.WithContext(ctx)

      // TODO: Execute processor chain on request (if configured)

      // Set target URL and headers
      r.URL.Scheme = target.Scheme
      r.URL.Host = target.Host
      r.Host = target.Host
      for k, v := range headers {
          r.Header.Set(k, v)
      }
  }

  ---
  3. Response Logging (ModifyResponse Hook)

  Location: Lines 153 (after Director, before mux.HandleFunc)

  Add ModifyResponse hook:

  proxy.ModifyResponse = func(resp *http.Response) error {
      // Retrieve request metadata from context
      requestID := resp.Request.Context().Value("requestID").(string)
      serverID := resp.Request.Context().Value("serverID").(string)
      endpoint := resp.Request.Context().Value("endpoint").(string)

      // Log the response from downstream server BEFORE returning to client
      if c.logger != nil {
          // Read and log response body
          bodyBytes, err := io.ReadAll(resp.Body)
          if err == nil {
              responseBody := string(bodyBytes)

              // Log to file
              _ = c.logger.LogResponse(requestID, c.sessionID, endpoint, nil, serverID, responseBody, true, "")

              // Debug output to stderr
              fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] [%s] %s\n", endpoint, responseBody)

              // Restore body for client (important!)
              resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
          }
      }

      // TODO: Execute processor chain on response (if configured)

      return nil
  }

  ---
  4. Proxy Lifecycle Logging

  Location: Line 123 (StartCentianServer)

  Add proxy start logging:
  func (c *CentianServer) StartCentianServer() error {
      config := c.config

      // Log proxy start
      if c.logger != nil {
          _ = c.logger.LogProxyStart(c.sessionID, "http_proxy",
              []string{fmt.Sprintf("port=%s", config.ProxyConfiguration.Port)},
              fmt.Sprintf("http_proxy_%d", time.Now().UnixNano()))
      }

      // ... rest of existing code ...
  }

  Location: Line 164 (Shutdown - need to add this method)

  Add shutdown with logging:
  func (c *CentianServer) Shutdown(ctx context.Context) error {
      log.Println("Shutting down proxy server...")

      // Log proxy stop
      if c.logger != nil {
          _ = c.logger.LogProxyStop(c.sessionID, "http_proxy",
              []string{fmt.Sprintf("port=%s", c.config.ProxyConfiguration.Port)},
              fmt.Sprintf("http_proxy_%d", time.Now().UnixNano()),
              true, "")
          _ = c.logger.Close()
      }

      return c.server.Shutdown(ctx)
  }

  ---
  5. Required Imports

  Add these imports at the top:
  import (
      "bytes"
      "context"
      "fmt"
      "io"
      "log"
      "net/http"
      "net/http/httputil"
      "net/url"
      "os"
      "time"

      "github.com/CentianAI/centian-cli/internal/logging"  // Add
      "github.com/CentianAI/centian-cli/internal/processor" // Add (for future processor integration)
  )

  ---
  Summary of Changes

  | Location                   | Change                                       | Purpose                      |
  |----------------------------|----------------------------------------------|------------------------------|
  | Struct (L58-62)            | Add logger, sessionID, processorChain fields | Store logging infrastructure |
  | Constructor (L76-89)       | Create logger, generate sessionID            | Initialize logging           |
  | Director Hook (L145)       | Read body → Log request → Restore body       | Log every client request     |
  | ModifyResponse Hook (L153) | Read body → Log response → Restore body      | Log every server response    |
  | Start (L123)               | Add LogProxyStart call                       | Log proxy lifecycle          |
  | Shutdown (L164)            | Add LogProxyStop + logger.Close()            | Clean shutdown with logging  |

  Key Pattern from Stdio:
  - ✅ Same logger instance (logging.NewLogger())
  - ✅ Same log methods (LogRequest, LogResponse, LogProxyStart, LogProxyStop)
  - ✅ Same debug output format ([CLIENT->SERVER], [SERVER->CLIENT])
  - ✅ Request ID generation (req_<timestamp>)
  - ✅ Session tracking (sessionID per proxy instance)
  - ✅ Processor integration points (marked with TODOs)

  This gives you the same logging pattern as stdio proxy, adapted for HTTP transport with ReverseProxy hooks.