<div align="center">
  <h1>geminiproxy</h1>
  <br/>
</div>

![Gemini API Proxy Demo](./docs/geminiproxy.gif)

A simple Go proxy server for the Gemini API that provides automatic API key rotation.

## Features

- Proxies requests to the Gemini API
- Automatically rotates through multiple API keys in round-robin fashion
- Transparent to clients - they don't need to provide API keys
- Compatible with go-genai client library

## Setup

1. Clone the repository:

   ```bash
   git clone git@github.com:ChakshuGautam/geminiproxy.git
   cd geminiproxy
   ```

2. Create a `gemini.keys` file in the root directory with your Gemini API keys (one per line):

   ```
   # API Keys (one per line)
   AIzaSyA...key1
   AIzaSyB...key2
   AIzaSyC...key3
   ```

3. Build and run:
   ```bash
   go build -o geminiproxy ./cmd/geminiproxy
   # Then run the executable
   ./geminiproxy
   ```
   Alternatively, to run the test client:
   ```bash
   go run ./cmd/test_client.go
   ```

## Docker Setup

Alternatively, you can build and run the proxy using Docker and Docker Compose.

1.  **Prerequisites:** Ensure you have Docker and Docker Compose installed.
2.  **Create `gemini.keys`:** As described in the main setup, create a `gemini.keys` file in the project root directory containing your Gemini API keys (one per line). **Important:** Add `gemini.keys` to your `.gitignore` file.
3.  **Build and Run with Docker Compose:**
    ```bash
    docker-compose up --build -d
    ```
    This command will build the Docker image (if it doesn't exist) and start the proxy container in the background. The proxy will be accessible at `http://localhost:8081`.
4.  **Stopping the Container:**
    ```bash
    docker-compose down
    ```

## Usage

### Usage with go-genai Client

```go
import (
	"context"
	"github.com/google/generative-ai-go/genai"
)
func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx,
		option.WithAPIKey("DUMMY_API_KEY"),
		option.WithEndpoint("http://localhost:8081"), //proxy starts at this port
	)
	// Use the client normally
	model := client.GenerativeModel("gemini-2.0-flash")
	prompt := "Write a short poem about Go programming"

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
}
```

A complete example demonstrating API key rotation across multiple requests is provided in the [test_client.go](./cmd/test_client.go) file. When you run it, you'll see each request using a different API key from your key pool.

### Usage with Cline

Start the server in whatever way you prefer (Docker, Docker Compose, or directly). Then just setup like below

![Cline Setup Proxy](./docs/cline.png)

### Usage with LiteLLM

```bash
docker compose up --build -d

# The builds the geminiproxy image and starts the LiteLLM server with a DB and Promeetheus
```

This will start the proxy server on port 8081. LLM Proxy will be available at `http://localhost:4000`. The UI would be available at `http://localhost:4000/ui` and the palyground at `http://localhost:4000/ui/?page=llm-playground`.

![LiteLLM Setup Proxy](./docs/litellm.png)

If you want to test the API directly, you can use the following curl command:

```bash
curl --location 'http://0.0.0.0:4000/chat/completions' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer sk-1234' \
--data '{
    "model": "gemini-2.5-flash-preview-04-17",
    "messages": [
        {
            "role": "system",
            "content": "You are a helpful math tutor. Guide the user through the solution step by step."
        },
        {
            "role": "user",
            "content": "how can I solve 8x + 7 = -23"
        }
    ]
}'
```

## Contributing
Contributions are welcome! Please open an issue or submit a pull request for any bugs, features, or improvements.

## Interactive TUI Mode

This proxy includes a Terminal User Interface (TUI) for interactive monitoring and management. When you run the proxy directly (e.g., `go run cmd/geminiproxy/main.go` or `./geminiproxy`), the TUI will start automatically.

**Features:**
- **Dashboard:** View real-time statistics including:
    - Handler status and uptime.
    - Recent proxy errors.
    - Overall proxy performance (total requests, success/failure rates, active connections, average latency).
    - Performance and status of individual API keys.
    - System information (proxy uptime, Go process memory usage).
- **Settings:**
    - Configure TUI refresh rate.
    - Adjust the number of recent errors displayed.
    - Change the display format for API keys.
    - Enable or disable individual API keys in real-time.

**Keybindings:**
- `Ctrl+C`: Quit the application.
- `Ctrl+N`: Switch to the next tab (Dashboard -> Settings).
- `Ctrl+P`: Switch to the previous tab (Settings -> Dashboard).
- `Tab` / `Shift+Tab`: Navigate between UI elements within a page.
- `Arrow Keys`: Navigate within dropdowns or lists.
- `Enter` / `Space`: Select an item, open a dropdown, or toggle a checkbox.

The TUI provides an easy way to observe the proxy's behavior and manage your API keys without needing to restart the application.

## License

MIT
