package mcpgateway
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// Tool definition
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Endpoint    string                 `json:"endpoint"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Tool registry
var (
	toolRegistry = make(map[string]Tool)
	registryMutex sync.RWMutex
)

func main() {
	// Register default tools
	registerDefaultTools()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/tools/list", listToolsHandler)
	http.HandleFunc("/tools/call", callToolHandler)
	http.HandleFunc("/tools/register", registerToolHandler)

	port := getEnv("PORT", "9100")
	log.Printf("🔧 MCP Gateway starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func registerDefaultTools() {
	tools := []Tool{
		{
			Name:        "verify-docs",
			Description: "Verify and extract information from KYC documents",
			Endpoint:    "http://localhost:9101/verify",
			Parameters: map[string]interface{}{
				"document_type": "string",
				"file_path":     "string (optional)",
			},
		},
		{
			Name:        "risk-score",
			Description: "Calculate merchant risk score",
			Endpoint:    "http://localhost:9102/calculate",
			Parameters: map[string]interface{}{
				"merchant_data": "object",
			},
		},
		{
			Name:        "web-search",
			Description: "Search web for latest information",
			Endpoint:    "http://localhost:9103/search",
			Parameters: map[string]interface{}{
				"query": "string",
			},
		},
	}

	for _, tool := range tools {
		toolRegistry[tool.Name] = tool
		log.Printf("  ✓ Registered tool: %s", tool.Name)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]interface{}{
		"status": "healthy",
		"service": "mcp-gateway",
		"tools_count": len(toolRegistry),
	}, http.StatusOK)
}

func listToolsHandler(w http.ResponseWriter, r *http.Request) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	tools := make([]Tool, 0, len(toolRegistry))
	for _, tool := range toolRegistry {
		tools = append(tools, tool)
	}

	respondJSON(w, map[string]interface{}{
		"tools": tools,
		"count": len(tools),
	}, http.StatusOK)
}

func callToolHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Tool   string                 `json:"tool"`
		Params map[string]interface{} `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	registryMutex.RLock()
	tool, exists := toolRegistry[req.Tool]
	registryMutex.RUnlock()

	if !exists {
		respondError(w, "Tool not found", http.StatusNotFound)
		return
	}

	log.Printf("🔧 Calling tool: %s", tool.Name)

	// Forward request to tool
	requestBody, _ := json.Marshal(req.Params)
	resp, err := http.Post(tool.Endpoint, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		respondError(w, fmt.Sprintf("Tool call failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respondError(w, "Failed to decode tool response", http.StatusInternalServerError)
		return
	}

	respondJSON(w, result, http.StatusOK)
}

func registerToolHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var tool Tool
	if err := json.NewDecoder(r.Body).Decode(&tool); err != nil {
		respondError(w, "Invalid tool definition", http.StatusBadRequest)
		return
	}

	registryMutex.Lock()
	toolRegistry[tool.Name] = tool
	registryMutex.Unlock()

	log.Printf("✓ Registered new tool: %s", tool.Name)
	respondJSON(w, map[string]string{"status": "registered"}, http.StatusOK)
}

func respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, message string, status int) {
	respondJSON(w, map[string]string{"error": message}, status)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
