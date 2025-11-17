package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	embedModel        = "text-embedding-004"
	embedModelPath    = "models/" + embedModel
	geminiAPIBasePath = "https://generativelanguage.googleapis.com/v1beta"
	maxBatchSize      = 100
)

type EmbedRequest struct {
	Text string `json:"text"`
}

type EmbedBatchRequest struct {
	Texts []string `json:"texts"`
}

type EmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Dimension int       `json:"dimension"`
}

type EmbedBatchResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Count      int         `json:"count"`
	Dimension  int         `json:"dimension"`
}

type geminiAPIError struct {
	Error struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

func callGeminiAPI(endpoint string, payload interface{}, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s", geminiAPIBasePath, endpoint), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		var apiErr geminiAPIError
		if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("gemini api error: %s (%s)", apiErr.Error.Message, apiErr.Error.Status)
		}
		return fmt.Errorf("gemini api error: status %d: %s", resp.StatusCode, string(data))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

func buildContentPayload(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": map[string]interface{}{
			"parts": []map[string]string{
				{"text": text},
			},
		},
	}
}

var (
	ctx        = context.Background()
	httpClient = &http.Client{Timeout: 30 * time.Second}
	apiKey     string
)

func main() {
	apiKey = os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable not set")
	}

	log.Println("Gemini API key loaded successfully")

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/embed", embedHandler)
	http.HandleFunc("/embed-batch", embedBatchHandler)

	port := getEnv("PORT", "8081")
	log.Printf("Embed Service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "embed-service",
		"model":   "text-embedding-004",
	})
}

func embedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EmbedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		respondError(w, "Text cannot be empty", http.StatusBadRequest)
		return
	}

	embedding, err := generateEmbedding(req.Text)
	if err != nil {
		respondError(w, "Failed to generate embedding: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := EmbedResponse{
		Embedding: embedding,
		Dimension: len(embedding),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func embedBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EmbedBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Texts) == 0 {
		respondError(w, "Texts array cannot be empty", http.StatusBadRequest)
		return
	}

	log.Printf("Generating embeddings for %d texts", len(req.Texts))

	embeddings, err := generateBatchEmbeddings(req.Texts)
	if err != nil {
		respondError(w, "Failed to generate embeddings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := EmbedBatchResponse{
		Embeddings: embeddings,
		Count:      len(embeddings),
		Dimension:  len(embeddings[0]),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func generateEmbedding(text string) ([]float32, error) {
	var response struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := callGeminiAPI(fmt.Sprintf("%s:embedContent", embedModelPath), buildContentPayload(text), &response); err != nil {
		return nil, err
	}

	if response.Embedding.Values == nil {
		return nil, fmt.Errorf("gemini api returned empty embedding")
	}

	return response.Embedding.Values, nil
}

func generateBatchEmbeddings(texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}

		requests := make([]map[string]interface{}, end-start)
		for i, text := range texts[start:end] {
			req := buildContentPayload(text)
			req["model"] = embedModelPath
			requests[i] = req
		}

		var response struct {
			Embeddings []struct {
				Values []float32 `json:"values"`
			} `json:"embeddings"`
		}

		payload := map[string]interface{}{
			"model":    embedModelPath,
			"requests": requests,
		}

		if err := callGeminiAPI(fmt.Sprintf("%s:batchEmbedContents", embedModelPath), payload, &response); err != nil {
			return nil, err
		}

		if len(response.Embeddings) != len(requests) {
			return nil, fmt.Errorf("gemini api returned %d embeddings for %d texts", len(response.Embeddings), len(requests))
		}

		for _, emb := range response.Embeddings {
			result = append(result, emb.Values)
		}
	}

	return result, nil
}

func respondError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
