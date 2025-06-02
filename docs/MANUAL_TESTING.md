# Manual Testing Plan for GeminiProxy TUI

This document outlines the steps to manually test the functionality of the GeminiProxy Terminal User Interface (TUI).

## 1. Prerequisites

*   **Go Environment:** A working Go environment (Go 1.18+ recommended).
*   **API Keys:** A valid `gemini.keys` file in the root of the project directory, containing one or more valid Google Gemini API keys.
*   **Dependencies:** If any new external Go dependencies were added for the TUI (e.g., `tview`), ensure they are fetched (`go mod tidy`). `github.com/rivo/tview` is the primary TUI dependency.

## 2. Starting the Application

1.  Open your terminal and navigate to the root directory of the `geminiproxy` project.
2.  Run the application:
    ```bash
    go run cmd/geminiproxy/main.go
    ```
3.  **Expected:** The TUI should launch, displaying the "Dashboard" view by default. The proxy server will also start in the background. You should see log messages from the proxy in the same terminal or a separate one if you redirect its output.

## 3. General TUI Navigation and Interaction

*   **Quit:** Press `Ctrl+C`.
    *   **Expected:** The TUI application should close gracefully, and the Go program should exit.
*   **Tab Navigation:**
    *   Press `Ctrl+N` (Next Tab).
        *   **Expected:** Focus should switch from "Dashboard" to "Settings".
    *   Press `Ctrl+P` (Previous Tab).
        *   **Expected:** Focus should switch from "Settings" back to "Dashboard".
    *   Repeat cycling a few times.
*   **Within-View Navigation:**
    *   On the "Settings" tab, use `Tab` and `Shift+Tab` (or Arrow Keys where appropriate for `tview.Form`) to navigate between dropdowns and the API key checkboxes.
    *   Use `Enter` or `Space` to interact with focused elements (e.g., open a dropdown, toggle a checkbox).
    *   **Expected:** Navigation should be intuitive. Focus indication should be clear.

## 4. Dashboard View Testing

While the Dashboard is active, observe the following panels:

*   **Header:**
    *   **Expected:** Displays "GeminiProxy TUI Dashboard".
*   **Handler Status:**
    *   **Expected:** Shows "Status: [Online]" (or similar if status changes are implemented). Uptime should start from 0s and increase steadily.
*   **Legend:**
    *   **Expected:** Displays: "Ctrl+C: Quit | Ctrl+N/P: Tabs | Tab/Arrows: Navigate | Enter/Space: Select/Toggle". Verify its accuracy.
*   **Recent Errors:**
    *   **Initial State:** Should show "No errors reported yet." or be empty.
    *   **Error Generation:**
        1.  Send an invalid request to the proxy, e.g., `curl http://localhost:8081/invalid_path` or a request that would cause an API error.
        2.  If a key is disabled (see Settings testing), requests attempting to use it (if not blocked earlier) should result in errors.
        *   **Expected:** Errors (e.g., "HTTP 404", "API key invalid", "No enabled API key available") should appear in this panel, with a timestamp. The list should be capped at the configured number (default 10, test with setting).
*   **Proxy Statistics:**
    *   **Initial State:** Should show 0 for most counters.
    *   **Traffic Generation:** Use a tool like `curl` or the provided `cmd/test_client.go` (if available and configured) to send valid requests to `http://localhost:8081/v1beta/models/gemini-pro:generateContent` (or other valid Gemini endpoint).
        ```bash
        # Example with curl - ensure you have a valid request body JSON
        curl -X POST -H "Content-Type: application/json" -d '{"contents":[{"parts":[{"text":"Write a story about a magic backpack"}]}]}' http://localhost:8081/v1beta/models/gemini-pro:generateContent
        ```
    *   **Expected:**
        *   "Total Requests": Increments for each request.
        *   "Successful": Increments for HTTP 2xx responses.
        *   "Failed": Increments for non-2xx responses or proxy errors.
        *   "Error Rate": Updates based on successes/failures.
        *   "Active Connections": Briefly increments during a request (may be hard to see with single, fast requests; more visible with concurrent or slow requests).
        *   "Avg. Latency": Shows an average response time.
*   **API Key Performance:**
    *   **Initial State:** Shows API keys from `gemini.keys` with 0 requests.
    *   **Traffic Generation:** As above, send requests.
    *   **Expected:** For each API key used by the proxy:
        *   "Reqs", "Success", "Fails", "Avg Lat(ms)" should update.
        *   "Status" should show "Active" (or "Disabled" if changed in Settings).
*   **System Information:**
    *   **Expected:**
        *   "Proxy Uptime": Should match Handler Status uptime.
        *   "Go Process Memory": "Alloc" and "Sys" should display plausible memory values (e.g., a few MiB). These values might change slightly over time or with activity.
        *   "Go Process CPU": Should display "N/A (Not Implemented)".

## 5. Settings View Testing

Navigate to the "Settings" tab (`Ctrl+N`).

*   **UI Refresh Rate:**
    1.  Note the current update frequency on the Dashboard (e.g., Uptime incrementing every 2 seconds by default).
    2.  Change "UI Refresh Rate (sec)" to "1".
        *   **Expected:** Status bar shows "Refresh rate set to 1 seconds". Dashboard data (especially Uptime) should now update every second.
    3.  Change to "5".
        *   **Expected:** Status bar shows "Refresh rate set to 5 seconds". Dashboard data updates every 5 seconds.
    4.  Revert to a preferred value (e.g., "2").
*   **Max Recent Errors Displayed:**
    1.  Set to "5".
    2.  Generate more than 5 errors (see "Recent Errors" testing on Dashboard).
        *   **Expected:** Dashboard only shows the latest 5 errors. The title of the "Recent Errors" panel should also update to "Recent Errors (Last 5)".
    3.  Set to "10" and verify more errors are displayed if generated.
*   **API Key Display Format:**
    1.  Cycle through "Masked", "Full", and "First 8 Chars".
    *   **Expected:**
        *   The API key identifiers in the "API Key Management" list (checkbox labels) on the Settings page itself should update according to the selected format.
        *   The "Name/Key" column in the "API Key Performance" panel on the Dashboard should also update its display format.
*   **API Key Management (Enable/Disable):**
    1.  Identify an API key in the "API Key Management" list. Note its current request count on the Dashboard.
    2.  Uncheck its checkbox to disable it.
        *   **Expected:** The checkbox unchecks. The key's status in the list (appended to label) changes to "(Disabled)". The "Status" for that key in the Dashboard's "API Key Performance" panel changes to "Disabled".
    3.  Send several requests to the proxy.
        *   **Expected:** The disabled key's request count ("Reqs") on the Dashboard should NOT increase. Other enabled keys should take over.
    4.  Re-check the checkbox to enable the key.
        *   **Expected:** The checkbox checks. Status changes to "(Active)" in Settings and Dashboard. The key should now be used for new requests.
    5.  **Disable All Keys:**
        *   Uncheck all API key checkboxes.
        *   **Expected:**
            *   All keys show "(Disabled)".
            *   The proxy's console log should show "CRITICAL: All API keys are disabled!".
            *   New requests to the proxy should now fail. These failures should be logged in the "Recent Errors" panel on the Dashboard (e.g., as errors from the Gemini API endpoint due to missing API key, or transport errors if the proxy fails to connect).
    6.  **Re-enable Keys:** Enable at least one key.
        *   **Expected:** Proxy functionality should resume. Requests using the re-enabled key(s) should succeed.

## 6. Visual and Layout Testing

*   **Window Resizing (if possible):**
    *   If your terminal emulator allows, try moderately resizing the window (making it smaller or larger, wider or narrower).
    *   **Expected:** The TUI layout should adapt reasonably. Panels should not severely overlap or become unreadable. Text within panels should wrap or be truncated where appropriate. Scrollbars should appear on scrollable views if content exceeds visible area. (This is a qualitative test; `tview` handles much of this automatically, but extreme resizes can expose issues).
*   **Consistency:**
    *   Check for consistent use of borders, titles, and padding across different UI elements.

## 7. Status Bar Messages

*   **Expected:**
    *   When changing "UI Refresh Rate", a message like "Refresh rate set to X seconds" appears for a few seconds.
    *   If an error occurs toggling an API key (e.g., if the key somehow wasn't found - unlikely with current UI), an error message should appear. (This may be hard to trigger intentionally).
    *   Messages should clear automatically after their duration.

This testing plan covers the main functionalities and interactions of the TUI. Testers should also report any unexpected behavior or usability issues encountered.The `docs/MANUAL_TESTING.md` file has been created.

Now, for Phase 1 (Code Comments) and Phase 3 (README.md update). I'll start by reading the relevant files and then apply the documentation changes.
