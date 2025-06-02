package api

import (
	"encoding/json"
	"geminiproxy"
	"log"
	"net/http"
	"time"
)

// GetSettingsHandler returns an http.HandlerFunc that serves the GET /api/v1/settings endpoint.
// This handler retrieves the current application-wide settings from the SettingsManager
// and the status of all API keys from the KeyManager. It then compiles this information
// into an api.SettingsData structure and responds with it as JSON.
//
// Dependencies:
//   - settingsManager *SettingsManager: The manager for global application settings.
//   - keyManager *geminiproxy.KeyManager: The manager for API key information.
//
// HTTP Responses:
//   - 200 OK: Successfully retrieved and returned settings data. Content-Type: application/json.
//   - 500 Internal Server Error: If essential components (SettingsManager, KeyManager) are not initialized,
//     or if there's an error during data retrieval or JSON marshaling.
func GetSettingsHandler(settingsManager *SettingsManager, keyManager *geminiproxy.KeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settingsManager == nil || keyManager == nil {
			log.Println("ERROR: GetSettingsHandler: SettingsManager or KeyManager not initialized")
			http.Error(w, "Internal server error: components not available", http.StatusInternalServerError)
			return
		}

		// Get global settings from SettingsManager
		settingsData, err := settingsManager.GetSettings()
		if err != nil {
			log.Printf("ERROR: GetSettingsHandler: Failed to get settings from manager: %v", err)
			http.Error(w, "Internal server error: failed to retrieve settings", http.StatusInternalServerError)
			return
		}

		// Fetch API key information from KeyManager
		keyInfosSnapshot := keyManager.GetKeyInfoSnapshot()
		apiKeySettings := make([]ApiKeySettingItem, len(keyInfosSnapshot))

		for i, ki := range keyInfosSnapshot {
			// The KeyAlias in ApiKeySettingItem should match how keys are identified/displayed.
			// Using ki.Name (masked version) as the alias.
			// The full key could be included based on context or permissions, but omitted for safety here.
			apiKeySettings[i] = ApiKeySettingItem{
				KeyAlias:  ki.Name,    // Using the masked name from KeyInfo
				// Key:    ki.Key,    // Optionally include the full key if the API needs to expose it
				IsEnabled: ki.Enabled, // Reflects current enabled/disabled status
			}
		}
		settingsData.APIKeys = apiKeySettings
		settingsData.LastUpdated = time.Now() // Update timestamp to reflect fresh data fetch

		// Marshal and send response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(settingsData); err != nil {
			log.Printf("ERROR: GetSettingsHandler: Failed to marshal JSON: %v", err)
			// Avoid writing to http.Error if headers/body already partially written
			return
		}
	}
}

// UpdateSettingsHandler creates an http.HandlerFunc that serves the POST /api/v1/settings endpoint.
// It decodes the incoming JSON request, updates settings via SettingsManager,
// updates API key statuses via KeyManager, and returns the updated settings configuration.
//
// Dependencies:
//   - settingsManager *SettingsManager: The manager for global application settings.
//   - keyManager *geminiproxy.KeyManager: The manager for API key information.
//
// Request Body:
//   - Expects a JSON body mapping to the api.SettingsData structure.
//   - For API key updates within the `apiKeys` array, the `key` field (full API key string)
//     MUST be provided to identify the key, along with the desired `isEnabled` status.
//
// HTTP Responses:
//   - 200 OK: Settings successfully updated. Returns the complete updated settings data as JSON.
//     Content-Type: application/json.
//   - 400 Bad Request: If the request body cannot be decoded, or if validation of global settings
//     (e.g., refresh rate, display format) fails. The response body will contain an error message.
//   - 500 Internal Server Error: If essential components are not initialized, if there's an error
//     updating API key statuses (e.g., a key specified in the request is not found), or if
//     there's an error during JSON marshaling of the response.
func UpdateSettingsHandler(settingsManager *SettingsManager, keyManager *geminiproxy.KeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settingsManager == nil || keyManager == nil {
			log.Println("ERROR: UpdateSettingsHandler: SettingsManager or KeyManager not initialized")
			http.Error(w, "Internal server error: components not available", http.StatusInternalServerError)
			return
		}

		// Decode the request body into SettingsData struct
		var reqSettings SettingsData
		if err := json.NewDecoder(r.Body).Decode(&reqSettings); err != nil {
			log.Printf("ERROR: UpdateSettingsHandler: Failed to decode request body: %v", err)
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Apply global settings updates using SettingsManager
		// Note: UpdateSettings validates the global settings part of reqSettings.
		// The APIKeys part of reqSettings is not used by sm.UpdateSettings directly.
		if err := settingsManager.UpdateSettings(reqSettings); err != nil {
			log.Printf("ERROR: UpdateSettingsHandler: Validation/Error updating global settings: %v", err)
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest) // Validation errors are client's fault
			return
		}

		// Apply API key enable/disable changes using KeyManager
		// The request's ApiKeySettingItem.Key field MUST contain the full API key string for identification.
		var keyUpdateErrors []string
		for _, apiKeyToUpdate := range reqSettings.APIKeys {
			if apiKeyToUpdate.Key == "" {
				// If Key field is empty, we cannot identify the key to update.
				// KeyAlias might not be unique or might not be the identifier KeyManager uses.
				// For this implementation, we require the full key string for SetKeyStatus.
				log.Printf("WARN: UpdateSettingsHandler: APIKeySettingItem received without 'Key' field for alias '%s'. Skipping update for this item.", apiKeyToUpdate.KeyAlias)
				// Optionally, add to keyUpdateErrors or handle as a more severe error.
				// For now, just skipping it.
				continue
			}
			err := keyManager.SetKeyStatus(apiKeyToUpdate.Key, apiKeyToUpdate.IsEnabled)
			if err != nil {
				log.Printf("ERROR: UpdateSettingsHandler: Failed to update key %s status: %v", apiKeyToUpdate.KeyAlias, err)
				keyUpdateErrors = append(keyUpdateErrors, fmt.Sprintf("Failed to update key %s: %v", apiKeyToUpdate.KeyAlias, err))
			}
		}

		if len(keyUpdateErrors) > 0 {
			// Decide on error response: partial success or overall failure.
			// For simplicity, returning a 500 if any key update fails.
			// A 207 Multi-Status response could be used for partial successes.
			errorMsg := "Internal server error: failed to update one or more API key statuses. Details: " + fmt.Sprintf("%v", keyUpdateErrors)
			http.Error(w, errorMsg, http.StatusInternalServerError)
			return
		}

		// Fetch the complete, updated settings to return in the response, ensuring consistency.
		// This is similar to GetSettingsHandler logic.
		finalSettingsData, err := settingsManager.GetSettings()
		if err != nil {
			log.Printf("ERROR: UpdateSettingsHandler: Failed to retrieve combined settings post-update: %v", err)
			http.Error(w, "Internal server error: failed to retrieve updated settings", http.StatusInternalServerError)
			return
		}

		keyInfosSnapshot := keyManager.GetKeyInfoSnapshot()
		finalApiKeySettings := make([]ApiKeySettingItem, len(keyInfosSnapshot))
		for i, ki := range keyInfosSnapshot {
			finalApiKeySettings[i] = ApiKeySettingItem{
				KeyAlias:  ki.Name,
				Key:       ki.Key, // Include the full key in the response for clarity/completeness
				IsEnabled: ki.Enabled,
			}
		}
		finalSettingsData.APIKeys = finalApiKeySettings
		finalSettingsData.LastUpdated = time.Now()

		// Marshal and send response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(finalSettingsData); err != nil {
			log.Printf("ERROR: UpdateSettingsHandler: Failed to marshal final JSON response: %v", err)
			// Cannot send http.Error if headers already written or body partially written.
		}
	}
}
