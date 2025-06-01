package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var defaultSseHost = "127.0.0.1"
var defaultSsePort = 13080

// responseWriterLogger is a wrapper to capture response status and body
type responseWriterLogger struct {
	ResponseWriter http.ResponseWriter
	status         int
	body           bytes.Buffer
}

func (rw *responseWriterLogger) Header() http.Header {
	return rw.ResponseWriter.Header()
}

func (rw *responseWriterLogger) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriterLogger) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

func main() {
	var transport string
	flag.StringVar(&transport, "t", "stdio", "Transport type (stdio or http)")
	flag.StringVar(&transport, "transport", "stdio", "Transport type (stdio or http)")
	flag.Parse()

	p := provider.New()

	s := server.NewMCPServer(
		p,
	)

	go func() {
		log.Println("Booting provider...")

		if os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo" {
			log.Println("Demo credentials are set, skip.")
			return
		}

		_, err := p.Provide()
		if err != nil {
			log.Fatalf("Error booting provider: %v", err)
		}

		log.Println("Provider booted successfully.")
	}()

	switch transport {
	case "stdio":
		if err := s.ServeStdio(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case "http":
		host := os.Getenv("SLACK_MCP_HOST")
		if host == "" {
			host = defaultSseHost
		}
		port := os.Getenv("SLACK_MCP_PORT")
		if port == "" {
			port = strconv.Itoa(defaultSsePort)
		}

		corsHandler := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
				h.ServeHTTP(w, r)
			})
		}

		// Logging middleware
		loggingHandler := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.Printf("%s %s", r.Method, r.URL.Path)
				if r.Body != nil && (r.Method == "POST" || r.Method == "PUT") {
					bodyBytes, _ := io.ReadAll(r.Body)
					log.Printf("Request Body: %s", string(bodyBytes))
					// Restore the body for downstream handlers
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}

				// Capture response
				rw := &responseWriterLogger{ResponseWriter: w, status: 200}
				h.ServeHTTP(rw, r)
				log.Printf("Response Status: %d", rw.status)
				if rw.body.Len() > 0 {
					log.Printf("Response Body: %s", rw.body.String())
				}
			})
		}

		handler := mcpserver.NewStreamableHTTPServer(s.Server())
		http.Handle("/mcp", loggingHandler(corsHandler(handler)))
		http.Handle("/mcp/", loggingHandler(corsHandler(handler)))
		http.Handle("/", loggingHandler(corsHandler(http.NotFoundHandler())))

		log.Printf("MCP HTTP server listening on %s:%s (POST/GET at /mcp)", host, port)
		if err := http.ListenAndServe(host+":"+port, nil); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	default:
		log.Fatalf("Invalid transport type: %s. Must be 'stdio' or 'http'", transport)
	}
}
