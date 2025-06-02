package api

import (
	"sync"
	"time"
)

// Default settings values used when a SettingsManager is first initialized.
const (
	DefaultUIRefreshRateSeconds     = 2                                  // Default TUI refresh interval in seconds.
	DefaultAPIKeyDisplayFormat      = "Masked"                           // Default display format for API keys (TUI and API). Corresponds to "Masked" in TUI options.
	DefaultMaxRecentErrorsDisplayed = 10                                 // Default number of recent errors to keep/display.
)

// SettingsManager handles the proxy's configurable global settings.
// These settings primarily affect the TUI's behavior and data display, but can also be queried via the API.
// For this iteration, settings are stored in-memory and initialized with defaults.
// Future enhancements could involve persisting these settings to a configuration file.
// Access to settings is made thread-safe using a sync.RWMutex.
type SettingsManager struct {
	mu                       sync.RWMutex // Read-Write mutex to protect concurrent access to settings fields.
	uiRefreshRateSeconds     int          // TUI refresh rate in seconds.
	apiKeyDisplayFormat      string       // Format for displaying API keys (e.g., "Masked", "Full").
	maxRecentErrorsDisplayed int          // Maximum number of recent errors to display in TUI/API.
	// Note: Management of APIKey enabled/disabled status is handled by geminiproxy.KeyManager,
	// not directly by SettingsManager, though SettingsData includes APIKey information.
}

// NewSettingsManager creates and returns a new SettingsManager instance
// initialized with predefined default settings values.
func NewSettingsManager() *SettingsManager {
	return &SettingsManager{
		uiRefreshRateSeconds:     DefaultUIRefreshRateSeconds,
		apiKeyDisplayFormat:      DefaultAPIKeyDisplayFormat,
		maxRecentErrorsDisplayed: DefaultMaxRecentErrorsDisplayed,
	}
}

// GetSettings retrieves the current application settings, including global configurations
// like UI refresh rate, API key display format, and max recent errors.
// It returns these settings packaged in an api.SettingsData structure.
// The `APIKeys` field within the returned SettingsData is initialized as an empty slice,
// as it's intended to be populated by the API handler by fetching current key statuses
// from the KeyManager.
// This method is thread-safe due to the RWMutex.
func (sm *SettingsManager) GetSettings() (SettingsData, error) {
	sm.mu.RLock() // Acquire a read lock to safely access settings fields.
	defer sm.mu.RUnlock()

	settings := SettingsData{
		LastUpdated:              time.Now(), // Timestamp of when the settings data was retrieved.
		UIRefreshRateSeconds:     sm.uiRefreshRateSeconds,
		APIKeyDisplayFormat:      sm.apiKeyDisplayFormat,
		MaxRecentErrorsDisplayed: sm.maxRecentErrorsDisplayed,
		APIKeys:                  []ApiKeySettingItem{}, // APIKeys slice is filled by the caller (handler).
	}
	return settings, nil // Currently, no error conditions are defined for this simple retrieval.
}

// UpdateSettings applies new values to the global application settings managed by SettingsManager.
// It takes an api.SettingsData struct containing the desired new settings.
// Each setting is validated against a predefined set of allowed values or ranges.
// If any setting is invalid, an error is returned, and no settings are changed.
// The `APIKeys` field within the input `newSettings` is *not* processed by this method;
// API key status changes are handled directly by the API handler interacting with the KeyManager.
// This method is thread-safe due to the RWMutex.
func (sm *SettingsManager) UpdateSettings(newSettings SettingsData) error {
	sm.mu.Lock() // Acquire a write lock to modify settings fields.
	defer sm.mu.Unlock()

	// Validate UIRefreshRateSeconds against a list of allowed values.
	// These values typically correspond to options available in the TUI or API specification.
	validRefreshRate := false
	// Example allowed rates, matching TUI options and OpenAPI spec if defined.
	for _, rate := range []int{1, 2, 3, 5, 10} {
		if newSettings.UIRefreshRateSeconds == rate {
			validRefreshRate = true
			break
		}
	}
	if !validRefreshRate {
		return fmt.Errorf("invalid UiRefreshRateSeconds value: %d. Allowed values: {1, 2, 3, 5, 10}", newSettings.UIRefreshRateSeconds)
	}
	sm.uiRefreshRateSeconds = newSettings.UIRefreshRateSeconds

	// Validate APIKeyDisplayFormat.
	validDisplayFormat := false
	// Example allowed formats, matching TUI options and OpenAPI spec.
	for _, format := range []string{"Masked", "Full", "First 8 Chars"} {
		if newSettings.APIKeyDisplayFormat == format {
			validDisplayFormat = true
			break
		}
	}
	if !validDisplayFormat {
		return fmt.Errorf("invalid ApiKeyDisplayFormat value: '%s'. Allowed values: {'Masked', 'Full', 'First 8 Chars'}", newSettings.APIKeyDisplayFormat)
	}
	sm.apiKeyDisplayFormat = newSettings.APIKeyDisplayFormat

	// Validate MaxRecentErrorsDisplayed.
	validMaxErrors := false
	// Example allowed counts, matching TUI options and OpenAPI spec.
	for _, count := range []int{5, 10, 15, 20, 25} {
		if newSettings.MaxRecentErrorsDisplayed == count {
			validMaxErrors = true
			break
		}
	}
	if !validMaxErrors {
		return fmt.Errorf("invalid MaxRecentErrorsDisplayed value: %d. Allowed values: {5, 10, 15, 20, 25}", newSettings.MaxRecentErrorsDisplayed)
	}
	sm.maxRecentErrorsDisplayed = newSettings.MaxRecentErrorsDisplayed

	// Note: The APIKeys field from newSettings is handled by the POST /settings API handler,
	// which interacts directly with the KeyManager to update key statuses.
	// SettingsManager itself does not manage the list or status of individual API keys.

	// log.Printf("Settings updated in SettingsManager: UiRefreshRateSeconds=%d, APIKeyDisplayFormat='%s', MaxRecentErrorsDisplayed=%d",
	// 	sm.uiRefreshRateSeconds, sm.apiKeyDisplayFormat, sm.maxRecentErrorsDisplayed) // For debugging
	return nil
}

// GetUIRefreshRate returns the current UI refresh rate in seconds.
// This provides thread-safe access to this specific setting.
// While the TUI currently manages its own copy of this setting for ticker control,
// this getter could be used if TUI were to directly reference SettingsManager state.
func (sm *SettingsManager) GetUIRefreshRate() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.uiRefreshRateSeconds
}

// GetAPIKeyDisplayFormat returns the current API key display format setting.
// Provides thread-safe access.
func (sm *SettingsManager) GetAPIKeyDisplayFormat() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.apiKeyDisplayFormat
}

// GetMaxRecentErrorsDisplayed returns the configured maximum number of recent errors to display.
// Provides thread-safe access.
func (sm *SettingsManager) GetMaxRecentErrorsDisplayed() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.maxRecentErrorsDisplayed
}
