package proxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func RunBasicHTTPProxy() {
	downstreamURL := "https://api.githubcopilot.com/mcp/"
	githubPAT := os.Getenv("GITHUB_PAT")
	if githubPAT == "" {
		log.Fatal("GITHUB_PAT environment variable not set")
	}

	// 1. Define the destination server
	target := downstreamURL
	remote, err := url.Parse(target)
	if err != nil {
		log.Fatal("Invalid target URL:", err)
	}

	// 2. Initialize the Reverse Proxy
	proxy := httputil.NewSingleHostReverseProxy(remote)

	// 3. Configure the proxy for streaming (SSE support)
	// FlushInterval: -1 ensures that the data is sent to the client immediately
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1

	// 4. Create the handler function
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Proxying request: %s %s", r.Method, r.URL.Path)

		// Optional: Update the host header to match the target
		r.Host = remote.Host
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", githubPAT))

		proxy.ServeHTTP(w, r)
	})

	// 5. Start the Proxy Server
	log.Printf("Proxy server started on :9000 -> forwarding to %s", target)
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
