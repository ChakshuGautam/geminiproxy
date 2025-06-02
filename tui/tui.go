// Package tui provides a terminal user interface (TUI) for monitoring and managing the Gemini API proxy.
// It uses the tview library to create a tabbed interface with a Dashboard for live statistics
// and a Settings page for configuring TUI behavior and managing API keys.
package tui

import (
	"fmt"
	"log" // For logging errors from ToggleKeyStatus and other internal issues.
	"strconv"
	"strings"
	"sync/atomic" // For atomic access to uiRefreshRateSeconds.
	"time"

	"geminiproxy" // Imports the core proxy logic for data retrieval.
	"github.com/rivo/tview" // The TUI library used.
)

// Global TUI application and layout variables.
var (
	app        *tview.Application // The main tview application instance.
	pages      *tview.Pages       // Manages the different views/tabs (Dashboard, Settings).
	mainFlex   *tview.Flex      // The root Flex container for the entire TUI layout (pages + tabInfo + statusBar).
	tabInfo    *tview.TextView    // Displays available tabs and basic navigation hints at the bottom.
	statusBar  *tview.TextView    // Displays temporary status messages (e.g., errors, confirmations) at the very bottom.

	// Instances of proxy components, passed from main.go, used for data fetching.
	proxyInstance    *geminiproxy.ProxyServer
	keyManagerInstance *geminiproxy.KeyManager

	// --- Dashboard View UI Elements ---
	// These TextViews are updated periodically to display live data from the proxy.
	headerTextView        *tview.TextView // Displays the main "GeminiProxy TUI Dashboard" title.
	handlerStatusTextView *tview.TextView // Shows proxy handler status (Online/Offline) and uptime.
	legendTextView        *tview.TextView // Displays keybinding information for TUI navigation.
	recentErrorsTextView  *tview.TextView // Shows a list of recent errors from the proxy.
	proxyStatsTextView    *tview.TextView // Displays global proxy statistics (request counts, latencies, etc.).
	apiKeyPerfTextView    *tview.TextView // Displays performance metrics for each API key.
	systemInfoTextView    *tview.TextView // Shows system/process information (uptime, memory usage).

	// --- Settings View Configuration Variables ---
	// These variables store the current values of user-configurable settings.
	// Default values are assigned here.
	uiRefreshRateSeconds     int32  = 2    // UI data refresh rate in seconds. Atomically accessed.
	maxRecentErrorsToDisplay int    = 10   // Max number of recent errors to show on the dashboard.
	apiKeyDisplayType        string = "Masked" // How API keys are displayed ("Masked", "Full", "First 8 Chars").

	// Options for settings dropdowns.
	uiRefreshRateOptionsStr = []string{"1", "2", "3", "5", "10"} // String representations for dropdown.
	uiRefreshRateOptionsInt = []int{1, 2, 3, 5, 10}             // Integer values corresponding to options.

	maxRecentErrorsOptionsStr = []string{"5", "10", "15", "20", "25"}
	maxRecentErrorsOptionsInt = []int{5, 10, 15, 20, 25}

	apiKeyDisplayOptions = []string{"Masked", "Full", "First 8 Chars"}

	// --- TUI Internals ---
	dataUpdateTicker              *time.Ticker // Ticker that triggers periodic refresh of dashboard data.
	apiKeyManagementListContainer *tview.Flex  // Flex container in Settings page holding API key checkboxes.
	settingsPageMainLayout        *tview.Flex  // The root Flex layout for the entire Settings page.
)

// Constants defining page names for the tview.Pages widget.
const (
	pageDashboard = "Dashboard" // Name identifier for the Dashboard page.
	pageSettings  = "Settings"  // Name identifier for the Settings page.
)

// pageNames is a slice of page name constants, used for tab navigation logic.
var pageNames = []string{pageDashboard, pageSettings}
var currentPageIndex = 0

// getCurrentRefreshInterval safely reads the current UI refresh interval (in seconds)
// from the atomic variable `uiRefreshRateSeconds` and returns it as a time.Duration.
// This is used to configure the data update ticker.
func getCurrentRefreshInterval() time.Duration {
	return time.Duration(atomic.LoadInt32(&uiRefreshRateSeconds)) * time.Second
}

// ShowStatusMessage displays a given message in the status bar at the bottom of the TUI.
// The message will be cleared after the specified duration.
// This function is thread-safe as it uses app.QueueUpdateDraw for UI modifications.
// If the app or statusBar is not initialized, it does nothing.
func ShowStatusMessage(message string, duration time.Duration) {
	if statusBar == nil || app == nil { // Ensure TUI components are initialized.
		return
	}
	app.QueueUpdateDraw(func() { // UI updates must run on the main TUI goroutine.
		statusBar.SetText(message)
	})
	// Schedule the message to be cleared after the duration.
	time.AfterFunc(duration, func() {
		app.QueueUpdateDraw(func() {
			// Only clear the message if it's still the one we set,
			// to avoid clearing a newer message that might have been set in the meantime.
			if statusBar.GetText(false) == message {
				statusBar.SetText("")
			}
		})
	})
}

// createDashboardPage initializes and returns the root primitive for the Dashboard view.
// This view displays real-time statistics and information about the proxy's operation.
// It consists of a header and a grid of various informational panels.
// The TextView elements for dynamic data are assigned to package-level variables
// so they can be updated by the `updateUIData` function.
func createDashboardPage() tview.Primitive {
	headerTextView = tview.NewTextView().
		SetText("GeminiProxy TUI Dashboard"). // Main title for the dashboard.
		SetTextAlign(tview.AlignCenter)

	handlerStatusTextView = tview.NewTextView().
		SetDynamicColors(true).
		SetBorder(true).
		SetTitle("Handler Status")

	legendTextView = tview.NewTextView().
		SetText("Ctrl+C: Quit | Ctrl+N/P: Tabs | Tab/Arrows: Navigate | Enter/Space: Select/Toggle"). // Updated legend
		SetBorder(true).
		SetTitle("Legend")

	recentErrorsTextView = tview.NewTextView().
		SetScrollable(true).
		SetBorder(true).
		SetTitle(fmt.Sprintf("Recent Errors (Last %d)", maxRecentErrorsToDisplay))

	proxyStatsTextView = tview.NewTextView().
		SetBorder(true).
		SetTitle("Proxy Statistics")

	apiKeyPerfTextView = tview.NewTextView().
		SetScrollable(true).
		SetBorder(true).
		SetTitle("API Key Performance")

	systemInfoTextView = tview.NewTextView().
		SetBorder(true).
		SetTitle("System Information")

	// Grid layout for dashboard content
	contentGrid := tview.NewGrid().
		SetRows(3, 0, 0).
		SetColumns(0, 0, 0).
		SetBorders(false)

	contentGrid.AddItem(handlerStatusTextView, 0, 0, 1, 1, 0, 0, false)
	contentGrid.AddItem(legendTextView, 0, 1, 1, 2, 0, 0, false)
	contentGrid.AddItem(recentErrorsTextView, 1, 0, 1, 3, 0, 0, false)
	contentGrid.AddItem(proxyStatsTextView, 2, 0, 1, 1, 0, 0, false)
	contentGrid.AddItem(apiKeyPerfTextView, 2, 1, 1, 1, 0, 0, false)
	contentGrid.AddItem(systemInfoTextView, 2, 2, 1, 1, 0, 0, false)

	// Flex layout for the whole dashboard page (header + content grid)
	dashboardLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(headerTextView, 1, 0, false).
		AddItem(contentGrid, 0, 1, true)

	return dashboardLayout
}

// refreshAPIKeySettingsList clears and rebuilds the list of API key checkboxes
// in the API Key Management section of the Settings page.
// This function is called when the Settings page is focused or when a setting
// affecting key display (like APIKeyDisplayFormat) is changed.
// It fetches the latest key information and rebuilds the list of checkboxes.
// UI updates are queued to run on the main TUI goroutine.
func refreshAPIKeySettingsList() {
	if apiKeyManagementListContainer == nil || keyManagerInstance == nil || app == nil {
		// Ensure necessary components are initialized.
		return
	}

	app.QueueUpdateDraw(func() { // All UI manipulations must be on the main goroutine.
		apiKeyManagementListContainer.Clear() // Remove all previous checkboxes/text views.

		keyInfos := keyManagerInstance.GetKeyInfoSnapshot() // Get current state of all API keys.
		if len(keyInfos) == 0 {
			// Display a message if no keys are loaded/found.
			apiKeyManagementListContainer.AddItem(tview.NewTextView().SetText("No API keys found or loaded."), 0, 1, false)
			return
		}

		// Add a static header for the API key list.
		header := fmt.Sprintf("%-20s %s", "API Key Identifier", "Status (Click to Toggle)") // Clarified action
		apiKeyManagementListContainer.AddItem(tview.NewTextView().SetText(header), 1, 0, false) // Fixed size row
		apiKeyManagementListContainer.AddItem(tview.NewTextView().SetText(strings.Repeat("-", len(header)+5)), 1, 0, false) // Separator line

		// Iterate through each API key and create a checkbox for it.
		for _, ki := range keyInfos {
			currentKeyInfo := ki // Capture range variable for use in the closure below.

			var displayLabel string // Determine how the key identifier is displayed based on settings.
			switch apiKeyDisplayType {
			case "Full":
				displayLabel = currentKeyInfo.Key
			case "First 8 Chars":
				if len(currentKeyInfo.Key) > 8 {
					displayLabel = currentKeyInfo.Key[:8] + "..."
				} else {
					displayLabel = currentKeyInfo.Key
				}
			default: // "Masked" is the default.
				displayLabel = currentKeyInfo.Name
			}
			// Append the key's current status (e.g., "Active", "Disabled") to its display label.
			// This status is part of the checkbox label itself, not a separate TextView.
			checkboxLabel := fmt.Sprintf("%-20s (%s)", displayLabel, currentKeyInfo.Status)

			// Create the checkbox for enabling/disabling the key.
			checkbox := tview.NewCheckbox().
				SetLabel(checkboxLabel). // Set the composite label.
				SetChecked(currentKeyInfo.Enabled). // Set initial checked state based on key's Enabled field.
				SetChangedFunc(func(checked bool) { // Callback when checkbox state is changed by user.
					// Attempt to toggle the key's status in the backend.
					err := keyManagerInstance.ToggleKeyStatus(currentKeyInfo.Key)
					if err != nil {
						// Log the error and show a message in the TUI status bar.
						log.Printf("Error toggling key %s: %v", currentKeyInfo.Name, err)
						ShowStatusMessage(fmt.Sprintf("Error toggling key %s: %v", currentKeyInfo.Name, err), 3*time.Second)
						// Note: The list will refresh anyway, potentially reverting the checkbox if the backend toggle failed.
					}
					// Refresh the entire list of API key checkboxes to reflect the true state from the backend.
					refreshAPIKeySettingsList()
					// Update dashboard data as API key status changes can affect displayed metrics.
					updateUIData()
				})
			// Add the checkbox to the container. Each checkbox gets its own row.
			// `fixedSize=1` means it takes one row height. `proportion=0` means it doesn't grow with extra space. `focus=true` means it's navigable.
			apiKeyManagementListContainer.AddItem(checkbox, 1, 0, true)
		}
	})
}

// createSettingsPage initializes and returns the root primitive for the Settings view.
// This view is composed of two main sections:
// 1. A form for general TUI settings (e.g., refresh rate, display formats).
// 2. A dynamically generated list of checkboxes for managing API key statuses (Enable/Disable).
// The overall layout is managed by `settingsPageMainLayout` (a Flex container).
func createSettingsPage() tview.Primitive {
	// --- General Settings Form ---
	form := tview.NewForm()
	form.SetBorder(false) // Border will be on the main settings layout

	// UI Refresh Rate Dropdown
	currentRefreshRateIndex := 0
	currentRateSecs := int(atomic.LoadInt32(&uiRefreshRateSeconds))
	for i, rate := range uiRefreshRateOptionsInt {if rate == currentRateSecs {currentRefreshRateIndex = i; break}}
	form.AddDropDown("UI Refresh Rate (sec)", uiRefreshRateOptionsStr, currentRefreshRateIndex,
		func(_ string, optionIndex int) {
			atomic.StoreInt32(&uiRefreshRateSeconds, int32(uiRefreshRateOptionsInt[optionIndex]))
			if dataUpdateTicker != nil {dataUpdateTicker.Reset(getCurrentRefreshInterval())}
			ShowStatusMessage(fmt.Sprintf("Refresh rate set to %d seconds", uiRefreshRateOptionsInt[optionIndex]), 2*time.Second)
		})

	// Max Recent Errors Displayed Dropdown
	currentMaxErrorsIndex := 0
	for i, val := range maxRecentErrorsOptionsInt {if val == maxRecentErrorsToDisplay {currentMaxErrorsIndex = i; break}}
	form.AddDropDown("Max Recent Errors Displayed", maxRecentErrorsOptionsStr, currentMaxErrorsIndex,
		func(_ string, optionIndex int) {
			maxRecentErrorsToDisplay = maxRecentErrorsOptionsInt[optionIndex]
			if recentErrorsTextView != nil && app != nil {
				app.QueueUpdateDraw(func() {
					recentErrorsTextView.SetTitle(fmt.Sprintf("Recent Errors (Last %d)", maxRecentErrorsToDisplay))
				})}
			updateUIData() // Refresh dashboard to reflect new error count limit
		})

	// API Key Display Format Dropdown
	currentAPIKeyFormatIndex := 0
	for i, format := range apiKeyDisplayOptions {if format == apiKeyDisplayType {currentAPIKeyFormatIndex = i; break}}
	form.AddDropDown("API Key Display Format", apiKeyDisplayOptions, currentAPIKeyFormatIndex,
		func(_ string, optionIndex int) {
			apiKeyDisplayType = apiKeyDisplayOptions[optionIndex]
			refreshAPIKeySettingsList() // Key display format changed, refresh checkbox labels
			updateUIData() // Refresh dashboard API key table
		})

	form.AddTextView("", "Note: Dropdown changes are applied immediately.", 0,1,true,false)

	// --- API Key Management Section ---
	apiKeyTitle := tview.NewTextView().SetText("API Key Management").SetTextAlign(tview.AlignCenter)
	// apiKeyManagementListContainer is a Flex container (FlexRow for vertical list) that holds checkboxes
	apiKeyManagementListContainer = tview.NewFlex().SetDirection(tview.FlexRow)
	// Frame to wrap the list container for a border and scrollability (though Flex itself doesn't scroll)
    // For scrolling, the list container would need to be inside a scrollable TextView or a List.
    // For now, if too many keys, it might not be ideal. A tview.List might be better for many keys.
	apiKeySectionFrame := tview.NewFrame(apiKeyManagementListContainer).
		SetBorders(0,0,0,0,0,0). // No inner border for the frame itself
		AddText("Enable/Disable API Keys", true, tview.AlignCenter, tview.Styles.SecondaryTextColor)


	// --- Overall Layout for Settings Page ---
	// settingsPageMainLayout is a Flex container (FlexColumn for vertical sections)
	settingsPageMainLayout = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(form, 0, 2, true). // General settings form, proportional size
		AddItem(tview.NewBox().SetBorder(false), 1, 0, false). // Spacer row, fixed size
		AddItem(apiKeySectionFrame, 0, 3, true) // API keys section, proportional size, focusable items inside

	settingsPageMainLayout.SetBorder(true).SetTitle("Settings").SetPadding(1,1,1,1)
	return settingsPageMainLayout
}

// formatBytes converts a uint64 byte count into a human-readable string,
// using appropriate units (B, KiB, MiB, GiB, etc.).
// It aims for one decimal place precision for units KiB and larger.
func formatBytes(b uint64) string {
	const unit = 1024 // Use 1024 for KiB, MiB (binary prefixes)
	if b < unit {
		return fmt.Sprintf("%d B", b) // Bytes
	}
	div, exp := int64(unit), 0
	// Loop to find the correct unit (KiB, MiB, GiB...)
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// %ciB will use 'K', 'M', 'G', ... from "KMGTPE"
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatDuration converts a time.Duration into a human-readable string format (e.g., "1d 2h 3m 4s").
// It rounds the duration to the nearest second before formatting.
// If the duration is less than a second (after rounding), it will show "0s".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	var parts []string
	if days > 0 {parts = append(parts, fmt.Sprintf("%dd", days))}
	if hours > 0 {parts = append(parts, fmt.Sprintf("%dh", hours))}
	if minutes > 0 {parts = append(parts, fmt.Sprintf("%dm", minutes))}
	if seconds > 0 || len(parts) == 0 {parts = append(parts, fmt.Sprintf("%ds", seconds))}
	return strings.Join(parts, " ")
}

// updateUIData fetches fresh data from proxyInstance and keyManagerInstance,
// then formats this data into strings and updates the corresponding TextView elements
// on the Dashboard page. This function is called periodically by a ticker.
// All UI updates are queued to run on the main TUI goroutine using `app.QueueUpdateDraw`.
func updateUIData() {
	// Ensure proxy and key manager instances are available.
	if proxyInstance == nil || keyManagerInstance == nil || app == nil {
		return // Should not happen if Start() is called correctly.
	}

	// Fetch the latest data snapshots.
	pStats := proxyInstance.GetGlobalProxyStats()
	keyInfos := keyManagerInstance.GetKeyInfoSnapshot() // Snapshot now includes Enabled and full Key string.
	hInfo := proxyInstance.GetHandlerInfo()             // HandlerInfo now includes memory statistics.

	// --- Format data for Handler Status panel ---
	handlerText := fmt.Sprintf("Status: [%s]\nUptime: %s",
		hInfo.Status,
		formatDuration(time.Since(hInfo.Uptime)))

	// --- Format data for Proxy Statistics panel ---
	var totalRequestsForErrorRate uint64 = 1 // Avoid division by zero if no requests yet.
	if pStats.TotalRequests > 0 {
		totalRequestsForErrorRate = pStats.TotalRequests
	}
	errorRate := (float64(pStats.TotalFailures) / float64(totalRequestsForErrorRate)) * 100
	statsText := fmt.Sprintf(
		"Total Requests: %d\nSuccessful: %d\nFailed: %d\nError Rate: %.2f%%\nActive Connections: %d\nAvg. Latency: %.2f ms",
		pStats.TotalRequests, pStats.TotalSuccesses, pStats.TotalFailures, errorRate,
		pStats.ActiveConnections, float64(pStats.OverallAverageLatencyMicroseconds)/1000.0, // Convert micros to millis.
	)

	// --- Format data for API Key Performance panel ---
	var apiKeyTextBuilder strings.Builder
	// Header for the API key table.
	header := fmt.Sprintf("%-18s | %-8s | %-8s | %-8s | %-12s | %s\n", "Name/Key", "Reqs", "Success", "Fails", "Avg Lat(ms)", "Status")
	apiKeyTextBuilder.WriteString(header)
	apiKeyTextBuilder.WriteString(strings.Repeat("-", len(header)+2) + "\n") // Dynamic underline.

	for _, ki := range keyInfos {
		displayKey := ki.Name // Default to masked name.
		switch apiKeyDisplayType { // Adjust display based on settings.
		case "Full":
			displayKey = ki.Key
		case "First 8 Chars":
			if len(ki.Key) > 8 {
				displayKey = ki.Key[:8] + "..."
			} else {
				displayKey = ki.Key
			}
		}
		// ki.Status directly from snapshot, which is updated by ToggleKeyStatus in proxy.go.
		apiKeyTextBuilder.WriteString(fmt.Sprintf("%-18s | %-8d | %-8d | %-8d | %-12.2f | %s\n",
			displayKey, ki.Requests, ki.Successes, ki.Failures,
			float64(ki.AverageLatencyMicroseconds)/1000.0, // Convert micros to millis.
			ki.Status,
		))
	}

	// --- Format data for Recent Errors panel ---
	var errorsTextBuilder strings.Builder
	errorsToDisplay := pStats.RecentErrors
	// Respect the 'maxRecentErrorsToDisplay' setting.
	if len(errorsToDisplay) > maxRecentErrorsToDisplay {
		errorsToDisplay = errorsToDisplay[:maxRecentErrorsToDisplay]
	}
	if len(errorsToDisplay) == 0 {
		errorsTextBuilder.WriteString("No errors reported yet.")
	} else {
		for i, errMsg := range errorsToDisplay {
			errorsTextBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, errMsg))
		}
	}

	// --- Format data for System Information panel ---
	// Now includes Go process memory statistics. CPU is marked as N/A.
	sysInfoText := fmt.Sprintf(
		"Proxy Uptime: %s\nGo Process Memory:\n  Alloc: %s\n  Sys:   %s\nGo Process CPU: N/A (Not Implemented)",
		formatDuration(time.Since(hInfo.Uptime)),
		formatBytes(hInfo.MemAllocBytes), // From hInfo.
		formatBytes(hInfo.MemSysBytes),   // From hInfo.
	)

	// Queue all text view updates to be performed on the main TUI goroutine.
	app.QueueUpdateDraw(func() {
		if handlerStatusTextView != nil {handlerStatusTextView.SetText(handlerText)}
		if proxyStatsTextView != nil {proxyStatsTextView.SetText(statsText)}
		if apiKeyPerfTextView != nil {apiKeyPerfTextView.SetText(apiKeyTextBuilder.String())}
		if recentErrorsTextView != nil {
			recentErrorsTextView.SetText(errorsTextBuilder.String())
			recentErrorsTextView.ScrollToBeginning() // Ensure latest errors are visible if list was scrolled.
		}
		if systemInfoTextView != nil {systemInfoTextView.SetText(sysInfoText)}
	})
}

// Start is the main entry point for launching the Terminal User Interface.
// It initializes the tview application, sets up pages (Dashboard, Settings),
// configures layout, input handling (keyboard shortcuts), and starts a ticker
// for periodically refreshing dynamic data on the Dashboard.
// Parameters:
//   p: An initialized *geminiproxy.ProxyServer instance, used to fetch global proxy stats and handler info.
//   km: An initialized *geminiproxy.KeyManager instance, used to fetch API key stats and manage their status.
func Start(p *geminiproxy.ProxyServer, km *geminiproxy.KeyManager) {
	// Store references to proxy components for data fetching.
	proxyInstance = p
	keyManagerInstance = km

	// Initialize the tview application.
	app = tview.NewApplication()

	// Create and configure pages for tabbed navigation.
	pages = tview.NewPages()
	dashboardPageContent := createDashboardPage() // Build dashboard UI.
	pages.AddPage(pageDashboard, dashboardPageContent, true, true) // Add dashboard, make it resizable and visible.

	settingsPageContent := createSettingsPage() // Build settings UI.
	pages.AddPage(pageSettings, settingsPageContent, true, false) // Add settings, resizable but initially not visible.

	// Create and configure the TabInfo view (shows available tabs).
	tabInfo = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	updateTabInfo() // Set initial tab information.

	// Create and configure the StatusBar view (for temporary messages).
	statusBar = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	statusBar.SetText("") // Initially empty.

	// Configure the main application layout using Flexbox.
	// This arranges pages, tabInfo, and statusBar vertically.
	mainFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, true).    // Pages content takes most space, is focusable.
		AddItem(tabInfo, 1, 0, false). // TabInfo is 1 row high, not focusable.
		AddItem(statusBar, 1, 0, false) // StatusBar is 1 row high, not focusable.

	// Set up global input capture for keyboard shortcuts (tab switching, quitting).
	app.SetInputCapture(func(event *tview.EventKey) *tview.EventKey {
		currentFocus := app.GetFocus()
        // If a form or checkbox has focus, let it handle non-global keys first.
        // This allows standard navigation (Tab, Arrows, Enter, Space) within those elements.
        if _, ok := currentFocus.(*tview.Form); ok && event.Key() != tview.KeyCtrlN && event.Key() != tview.KeyCtrlP && event.Key() != tview.KeyCtrlC {
            return event
        }
         if _, ok := currentFocus.(*tview.Checkbox); ok && event.Key() != tview.KeyCtrlN && event.Key() != tview.KeyCtrlP && event.Key() != tview.KeyCtrlC {
            return event
        }

		switch event.Key() {
		case tview.KeyCtrlN: // Next tab
			currentPageIndex = (currentPageIndex + 1) % len(pageNames)
			pages.SwitchToPage(pageNames[currentPageIndex])
			updateTabInfo() // Update the displayed tab names.
			// Set focus to the appropriate main content area of the new page.
			if pageNames[currentPageIndex] == pageDashboard {
				app.SetFocus(dashboardPageContent)
			} else { // pageSettings
				if apiKeyManagementListContainer != nil {
					// Refresh API key list when settings tab is selected.
					refreshAPIKeySettingsList()
				}
				app.SetFocus(settingsPageMainLayout)
			}
			return nil // Event handled.
		case tview.KeyCtrlP: // Previous tab
			currentPageIndex = (currentPageIndex - 1 + len(pageNames)) % len(pageNames) // Modulo for wrap-around.
			pages.SwitchToPage(pageNames[currentPageIndex])
			updateTabInfo()
			if pageNames[currentPageIndex] == pageDashboard {
				app.SetFocus(dashboardPageContent)
			} else { // pageSettings
				if apiKeyManagementListContainer != nil {
					refreshAPIKeySettingsList()
				}
				app.SetFocus(settingsPageMainLayout)
			}
			return nil // Event handled.
		case tview.KeyCtrlC: // Quit application
			app.Stop()
			return nil // Event handled.
		}
		return event // Event not handled by global shortcuts, pass to focused element.
	})

	// Initialize and start the ticker for periodic data updates on the dashboard.
	dataUpdateTicker = time.NewTicker(getCurrentRefreshInterval())
	go func() {
		updateUIData() // Perform an initial data load immediately.
		for {
			select {
			case <-dataUpdateTicker.C: // Triggered by the ticker.
				updateUIData()
			case <-app.Context().Done(): // Triggered when app.Stop() is called.
				dataUpdateTicker.Stop() // Clean up the ticker.
				return
			}
		}
	}()

	// Set the root UI element for the application, set initial focus, and run the TUI event loop.
	// This is a blocking call and will run until app.Stop() is called.
	if err := app.SetRoot(mainFlex, true).SetFocus(dashboardPageContent).Run(); err != nil {
		// If Run() exits with an error, panic to ensure the error is visible.
		// This is typically for critical setup errors.
		panic(err)
	}
}

// updateTabInfo refreshes the text displayed in the `tabInfo` TextView,
// highlighting the currently active tab and showing navigation shortcuts.
// It's called when switching tabs.
func updateTabInfo() {
	if tabInfo == nil { return }
	tabInfoText := "[::b]Tabs:[::-] "
	for i, name := range pageNames {
		if i == currentPageIndex {tabInfoText += fmt.Sprintf("[yellowgreen:black:b]%s[::-] ", name)} else {tabInfoText += fmt.Sprintf("%s ", name)}
	}
	tabInfoText += "| [::b]Ctrl+N[::-]:Next | [::b]Ctrl+P[::-]:Previous | [::b]Ctrl+C[::-]:Quit"
	tabInfo.SetText(tabInfoText)
}
