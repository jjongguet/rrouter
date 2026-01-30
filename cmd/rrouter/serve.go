package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const modeFilePath = ".rrouter/mode"

var (
	requestCount  atomic.Uint64
	appConfig     *Config
	upstreamURL   string
	listenAddr    string
	configWatcher *ConfigWatcher
	autoSwitch    *autoState
)

// proxyResult captures per-request error info from the reverse proxy ErrorHandler.
type proxyResult struct {
	mu        sync.Mutex
	isTimeout bool
	err       error
}

type contextKey string

const proxyResultKey contextKey = "proxyResult"

// stripThinkingBlocks removes thinking blocks from messages for non-Claude backends.
// If a message's content becomes empty after stripping, the message is removed entirely.
func stripThinkingBlocks(messages []interface{}) []interface{} {
	result := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			result = append(result, msg)
			continue
		}

		content, hasContent := msgMap["content"]
		if !hasContent {
			result = append(result, msg)
			continue
		}

		// Handle content as array of blocks
		contentArr, isArray := content.([]interface{})
		if !isArray {
			// String content or other format - keep as is
			result = append(result, msg)
			continue
		}

		// Filter out thinking blocks
		filteredContent := make([]interface{}, 0, len(contentArr))
		for _, block := range contentArr {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				filteredContent = append(filteredContent, block)
				continue
			}
			blockType, _ := blockMap["type"].(string)
			if blockType == "thinking" {
				// Skip thinking blocks
				continue
			}
			filteredContent = append(filteredContent, block)
		}

		// If content is empty after filtering, skip this message entirely
		if len(filteredContent) == 0 {
			continue
		}

		// Create new message with filtered content
		newMsg := make(map[string]interface{})
		for k, v := range msgMap {
			newMsg[k] = v
		}
		newMsg["content"] = filteredContent
		result = append(result, newMsg)
	}
	return result
}

func modifyRequestBody(body []byte, modeConfig *ModeConfig, mode string) ([]byte, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if modelVal, ok := data["model"]; ok {
		if modelStr, ok := modelVal.(string); ok {
			originalModel := modelStr

			// Step 1: Apply standard model rewriting (existing behavior)
			newModel := rewriteModelWithConfig(modelStr, modeConfig)

			// Step 2: Agent-type routing override (only when agentRouting is configured and enabled)
			if modeConfig != nil && modeConfig.AgentRouting != nil && modeConfig.AgentRouting.Enabled {
				agentName := detectAgentName(data)
				if agentName != "" {
					agentType := classifyAgent(agentName, modeConfig.AgentRouting)
					switch agentType {
					case AgentTypeGroup1:
						newModel = modeConfig.AgentRouting.Group1Model
						log.Printf("[Mode: %s] Agent routing: %s (group1) -> %s", mode, agentName, newModel)
					case AgentTypeGroup2:
						log.Printf("[Mode: %s] Agent routing: %s (group2) -> %s (standard)", mode, agentName, newModel)
					default:
						log.Printf("[Mode: %s] Agent routing: %s (unknown, fallback) -> %s", mode, agentName, newModel)
					}
				}
			}

			if newModel != originalModel {
				data["model"] = newModel
				log.Printf("[Mode: %s] Rewriting model: %s -> %s", mode, originalModel, newModel)
			}
		}
	}

	// Strip thinking blocks for non-claude modes (Gemini doesn't support them)
	if mode != "claude" {
		if messages, ok := data["messages"].([]interface{}); ok {
			stripped := stripThinkingBlocks(messages)
			data["messages"] = stripped
		}
	}

	return json.Marshal(data)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

// switchableResponseWriter starts in "deciding" mode. On WriteHeader:
// - If status >= 400: switch to buffer mode (capture body for potential retry)
// - If status < 400: switch to passthrough mode (write directly to real writer)
// This allows error responses to be buffered for retry while successful
// streaming responses pass through immediately.
type switchableResponseWriter struct {
	real       http.ResponseWriter
	statusCode int
	header     http.Header
	body       bytes.Buffer
	mode       int // 0=deciding, 1=buffering, 2=passthrough
}

const (
	modeDeciding    = 0
	modeBuffering   = 1
	modePassthrough = 2
)

func newSwitchableResponseWriter(w http.ResponseWriter) *switchableResponseWriter {
	return &switchableResponseWriter{
		real:       w,
		statusCode: http.StatusOK,
		header:     make(http.Header),
		mode:       modeDeciding,
	}
}

func (s *switchableResponseWriter) Header() http.Header {
	if s.mode == modePassthrough {
		return s.real.Header()
	}
	return s.header
}

func (s *switchableResponseWriter) WriteHeader(code int) {
	s.statusCode = code
	if s.mode == modeDeciding {
		if code >= 400 {
			// Error response: buffer it for potential retry
			s.mode = modeBuffering
		} else {
			// Success: passthrough mode, copy headers and write
			s.mode = modePassthrough
			for k, v := range s.header {
				for _, vv := range v {
					s.real.Header().Add(k, vv)
				}
			}
			s.real.WriteHeader(code)
		}
	} else if s.mode == modePassthrough {
		s.real.WriteHeader(code)
	}
	// In buffering mode, we just store the status code (already done above)
}

func (s *switchableResponseWriter) Write(data []byte) (int, error) {
	// If WriteHeader wasn't called, assume 200 OK (per http spec)
	if s.mode == modeDeciding {
		s.WriteHeader(http.StatusOK)
	}
	if s.mode == modePassthrough {
		return s.real.Write(data)
	}
	// Buffering mode
	return s.body.Write(data)
}

// Flush implements http.Flusher. In passthrough mode, flush to real writer.
// In buffering mode, this is a no-op (we're capturing the response).
func (s *switchableResponseWriter) Flush() {
	if s.mode == modePassthrough {
		if f, ok := s.real.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// IsBuffered returns true if the response was buffered (error status)
func (s *switchableResponseWriter) IsBuffered() bool {
	return s.mode == modeBuffering
}

// StatusCode returns the captured status code
func (s *switchableResponseWriter) StatusCode() int {
	return s.statusCode
}

// WriteTo writes the buffered response to the given writer.
// Only valid if IsBuffered() is true.
func (s *switchableResponseWriter) WriteTo(w http.ResponseWriter) {
	for k, v := range s.header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(s.statusCode)
	w.Write(s.body.Bytes())
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func createReverseProxy(upstream string) *httputil.ReverseProxy {
	target, _ := url.Parse(upstream)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1 // Enable streaming for SSE

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	// Custom error handler: distinguishes timeouts (504) from connection errors (502)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if result, ok := r.Context().Value(proxyResultKey).(*proxyResult); ok {
			result.mu.Lock()
			result.err = err
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				result.isTimeout = true
			}
			result.mu.Unlock()
		}

		log.Printf("[PROXY ERROR] %v", err)

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		} else {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		}
	}

	return proxy
}

func proxyHandler(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqNum := requestCount.Add(1)
		intent := configWatcher.GetMode()

		// Resolve "auto" -> concrete target
		target := autoSwitch.resolveRouting(intent)

		if intent == "auto" {
			log.Printf("[Req #%d] %s %s (mode: auto, target: %s)", reqNum, r.Method, r.URL.Path, target)
		} else {
			log.Printf("[Req #%d] %s %s (mode: %s)", reqNum, r.Method, r.URL.Path, target)
		}

		// Look up mode config using resolved target (not intent)
		mc, ok := appConfig.Modes[target]
		var modeConfig *ModeConfig
		if ok {
			modeConfig = &mc
		}

		// Read and modify request body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[Req #%d] Error reading body: %v", reqNum, err)
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		var modifiedBody []byte
		if len(bodyBytes) > 0 {
			modifiedBody, err = modifyRequestBody(bodyBytes, modeConfig, target)
			if err != nil {
				log.Printf("[Req #%d] Error modifying body: %v", reqNum, err)
				http.Error(w, "Error processing request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(modifiedBody))
			r.ContentLength = int64(len(modifiedBody))
		} else {
			modifiedBody = bodyBytes
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		// Set up per-request error tracking via context
		result := &proxyResult{}
		ctx := context.WithValue(r.Context(), proxyResultKey, result)
		r = r.WithContext(ctx)

		// AUTO MODE with internal retry
		if intent == "auto" {
			startTime := time.Now()

			// Use switchable writer: buffers error responses, passes through success
			sw := newSwitchableResponseWriter(w)
			proxy.ServeHTTP(sw, r)
			elapsed := time.Since(startTime)

			// Check if we got an error that was buffered
			result.mu.Lock()
			resultErr := result.err
			resultIsTimeout := result.isTimeout
			result.mu.Unlock()

			// Determine if retry is needed
			needsRetry := false
			if resultErr != nil {
				// Proxy error (timeout/connection failure)
				needsRetry = true
				log.Printf("[Req #%d] Response: proxy error (%s)", reqNum, formatDuration(elapsed))
			} else if sw.IsBuffered() && sw.StatusCode() >= 400 {
				// Error response was buffered
				needsRetry = true
				log.Printf("[Req #%d] Response: %d (%s)", reqNum, sw.StatusCode(), formatDuration(elapsed))
			} else {
				// Success (already passed through to client)
				log.Printf("[Req #%d] Response: %d (%s)", reqNum, sw.StatusCode(), formatDuration(elapsed))
				autoSwitch.recordUpstreamResponse(sw.StatusCode(), false)
				return
			}

			if needsRetry {
				// Record failure for auto-switch state
				if resultErr != nil {
					autoSwitch.recordUpstreamResponse(0, resultIsTimeout)
				} else {
					autoSwitch.recordUpstreamResponse(sw.StatusCode(), false)
				}

				// Get fallback target
				fallback := oppositeTarget(target)
				log.Printf("[AUTO-RETRY] %s failed, retrying on %s", target, fallback)

				// Re-modify body for fallback target
				fallbackMC, fbOK := appConfig.Modes[fallback]
				var retryBody []byte
				if len(bodyBytes) > 0 {
					var fbModeConfig *ModeConfig
					if fbOK {
						fbModeConfig = &fallbackMC
					}
					retryBody, err = modifyRequestBody(bodyBytes, fbModeConfig, fallback)
					if err != nil {
						log.Printf("[AUTO-RETRY] Error modifying body for %s: %v", fallback, err)
						// Fall back to original error response
						sw.WriteTo(w)
						return
					}
				} else {
					retryBody = bodyBytes
				}

				// Reset request body for retry
				r.Body = io.NopCloser(bytes.NewReader(retryBody))
				r.ContentLength = int64(len(retryBody))

				// Fresh proxyResult for retry
				retryResult := &proxyResult{}
				retryCtx := context.WithValue(r.Context(), proxyResultKey, retryResult)
				r = r.WithContext(retryCtx)

				// Retry directly to client (no more buffering)
				lrw := newLoggingResponseWriter(w)
				retryStart := time.Now()
				proxy.ServeHTTP(lrw, r)
				retryElapsed := time.Since(retryStart)

				// Record retry result
				retryResult.mu.Lock()
				retryErr := retryResult.err
				retryIsTimeout := retryResult.isTimeout
				retryResult.mu.Unlock()

				if retryErr != nil {
					autoSwitch.recordUpstreamResponse(0, retryIsTimeout)
					log.Printf("[AUTO-RETRY] Retry on %s: proxy error (%s)", fallback, formatDuration(retryElapsed))
				} else {
					autoSwitch.recordUpstreamResponse(lrw.statusCode, false)
					log.Printf("[AUTO-RETRY] Retry on %s: HTTP %d (%s)", fallback, lrw.statusCode, formatDuration(retryElapsed))
				}
				return
			}
			return
		}

		// NON-AUTO MODE: existing behavior unchanged
		lrw := newLoggingResponseWriter(w)
		startTime := time.Now()
		proxy.ServeHTTP(lrw, r)
		elapsed := time.Since(startTime)
		log.Printf("[Req #%d] Response: %d (%s)", reqNum, lrw.statusCode, formatDuration(elapsed))
	}
}

func serveHealthHandler(w http.ResponseWriter, r *http.Request) {
	intent := configWatcher.GetMode()
	target := autoSwitch.resolveRouting(intent)

	response := map[string]interface{}{
		"status":        "ok",
		"mode":          intent,
		"currentTarget": target,
		"requestCount":  requestCount.Load(),
		"listenAddr":    listenAddr,
		"upstreamURL":   upstreamURL,
		"defaultMode":   appConfig.DefaultMode,
	}

	// Add auto-switch details when in auto mode
	if intent == "auto" {
		autoInfo := autoSwitch.HealthInfo()
		for k, v := range autoInfo {
			response[k] = v
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// setupDateLogger sets up logging to ~/.rrouter/logs/YYYY-MM-DD.log
// Returns a cleanup function to close the file.
func setupDateLogger() func() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Cannot get home dir for logging: %v", err)
		return func() {}
	}

	logDir := filepath.Join(homeDir, ".rrouter", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Cannot create log dir: %v", err)
		return func() {}
	}

	logFile := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Cannot open log file: %v", err)
		return func() {}
	}

	log.SetOutput(f)
	return func() { f.Close() }
}

func writePIDFile() {
	homeDir, _ := os.UserHomeDir()
	pidPath := filepath.Join(homeDir, ".rrouter", "rrouter.pid")
	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func removePIDFile() {
	homeDir, _ := os.UserHomeDir()
	pidPath := filepath.Join(homeDir, ".rrouter", "rrouter.pid")
	os.Remove(pidPath)
}

// cmdServe runs the proxy server in foreground mode.
func cmdServe() {
	cleanup := setupDateLogger()
	defer cleanup()

	// Migrate old PID file
	migratePIDFile()

	listenAddr, upstreamURL = getConfig()
	appConfig = loadConfigWithDefaults()

	// Validate agent routing configs
	for modeName, modeConfig := range appConfig.Modes {
		if modeConfig.AgentRouting != nil {
			validateAgentRoutingConfig(modeConfig.AgentRouting, modeName)
		}
	}

	autoSwitch = newAutoState(appConfig.DefaultMode)

	// Initialize filesystem watcher for mode and config
	homeDir, _ := os.UserHomeDir()
	rrouterDir := filepath.Join(homeDir, ".rrouter")
	configWatcher = newConfigWatcher(rrouterDir, appConfig)
	defer configWatcher.Close()

	// Write PID file (for launchd/systemd-started daemons)
	writePIDFile()

	proxy := createReverseProxy(upstreamURL)

	http.HandleFunc("/health", serveHealthHandler)
	http.HandleFunc("/", proxyHandler(proxy))

	log.Println("=======================================================")
	log.Println("  rrouter started")
	log.Printf("  Listen:  %s", listenAddr)
	log.Printf("  Upstream: %s", upstreamURL)
	log.Printf("  Mode:    %s", configWatcher.GetMode())
	log.Printf("  Modes:   %d loaded", len(appConfig.Modes))
	log.Println("=======================================================")

	// Graceful shutdown on SIGTERM/SIGINT
	srv := &http.Server{Addr: listenAddr}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		removePIDFile()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Server stopped")
}
