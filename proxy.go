package geminiproxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	// DefaultKeysFile is the default location for the API keys file
	DefaultKeysFile = "gemini.keys"

	// DefaultPort is the default port for the proxy server
	DefaultPort = 8081

	// GeminiAPIEndpoint is the real Gemini API endpoint
	GeminiAPIEndpoint = "https://generativelanguage.googleapis.com"
)

// KeyManager handles API key rotation
type KeyManager struct {
	keys          []string
	currentKeyIdx int
	mu            sync.Mutex
}

// NewKeyManager creates a new key manager with the given keys file
func NewKeyManager(keysFile string) (*KeyManager, error) {
	keys, err := readAPIKeys(keysFile)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no API keys found in %s", keysFile)
	}
	return &KeyManager{keys: keys}, nil
}

// stored keys in the file gemini.keys one below the other
func readAPIKeys(filename string) ([]string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys file '%s': %v", filename, err)
	}

	var keys []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		key := strings.TrimSpace(line)
		// Skip empty lines and comment lines
		if key != "" && !strings.HasPrefix(key, "#") {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// GetKey returns the next available API key using simple round-robin
func (km *KeyManager) GetKey() string {
	km.mu.Lock()
	defer km.mu.Unlock()

	key := km.keys[km.currentKeyIdx]
	km.currentKeyIdx = (km.currentKeyIdx + 1) % len(km.keys)
	log.Printf("Using API key ending in ...%s", key[len(key)-5:])
	return key
}

type ProxyServer struct {
	keyManager *KeyManager
	server     *http.Server
}

func NewProxyServer(keyManager *KeyManager, port int) *ProxyServer {
	targetURL, _ := url.Parse(GeminiAPIEndpoint)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Add the API key and remove Authorization header
	proxy.Director = func(req *http.Request) {
		// Remove Authorization header if present (added for litellm compatibility)
		req.Header.Del("Authorization")

		apiKey := keyManager.GetKey()
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
		q := req.URL.Query()
		q.Set("key", apiKey)
		req.URL.RawQuery = q.Encode()
	}

	mux := http.NewServeMux()
	mux.Handle("/", proxy)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return &ProxyServer{
		keyManager: keyManager,
		server:     server,
	}
}

func (p *ProxyServer) Start() error {
	log.Printf("Starting simplified Gemini API proxy server on %s", p.server.Addr)
	log.Printf("Proxying requests to: %s", GeminiAPIEndpoint)
	if _, err := url.Parse(GeminiAPIEndpoint); err != nil {
		return fmt.Errorf("invalid GeminiAPIEndpoint: %v", err)
	}
	return p.server.ListenAndServe()
}

// ProxyURL returns the URL for clients to use
func (p *ProxyServer) ProxyURL() string {
	return fmt.Sprintf("http://localhost%s", p.server.Addr)
}
