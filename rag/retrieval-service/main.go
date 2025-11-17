// retrieval-service is a microservice that handles document retrieval for RAG applications.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type RetrievalRequest struct {
	Query      string            `json:"query"`      // User's question: "What are KYC requirements?"
	TopK       int               `json:"top_k"`      // How many results to return (default: 5)
	Collection string            `json:"collection"` // Which collection to search: "regulatory_docs", "merchant_docs", etc.
	Filters    map[string]string `json:"filters"`    // Optional filters: {"type": "regulatory"}
}

// RetrievalResult - A single search result
type RetrievalResult struct {
	ID         string                 `json:"id"`          // Chunk ID
	Score      float64                `json:"score"`       // Relevance score (0-1, higher is better)
	Text       string                 `json:"text"`        // The actual text content
	DocumentID string                 `json:"document_id"` // Which document this came from
	Source     string                 `json:"source"`      // Document name
	Metadata   map[string]interface{} `json:"metadata"`    // Additional info
}

// RetrievalResponse - Complete response sent back to user
type RetrievalResponse struct {
	Query       string            `json:"query"`           // Echo back the query
	Results     []RetrievalResult `json:"results"`         // Array of matching chunks
	Count       int               `json:"count"`           // Number of results
	ProcessTime float64           `json:"process_time_ms"` // How long it took (milliseconds)
}

// ============================================================================
// CONFIGURATION
// ============================================================================

var (
	// URLs of other microservices
	EMBED_SERVICE_URL    = getEnv("EMBED_SERVICE_URL", "http://localhost:8081")
	VECTOR_SERVICE_URL   = getEnv("VECTOR_SERVICE_URL", "http://localhost:8082")
	METADATA_SERVICE_URL = getEnv("METADATA_SERVICE_URL", "http://localhost:8083")
)

// ============================================================================
// MAIN FUNCTION
// ============================================================================

func main() {
	// Setup HTTP routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/retrieve", retrieveHandler)

	port := getEnv("PORT", "8084")
	log.Printf("🚀 Retrieval Service starting on port %s", port)
	log.Println("📡 Connected services:")
	log.Printf("   - Embed Service:    %s", EMBED_SERVICE_URL)
	log.Printf("   - Vector Service:   %s", VECTOR_SERVICE_URL)
	log.Printf("   - Metadata Service: %s", METADATA_SERVICE_URL)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ============================================================================
// HTTP HANDLERS
// ============================================================================

// healthHandler - Check if service is running
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "retrieval-service",
	})
}

// retrieveHandler - Main RAG retrieval endpoint
// This is where the magic happens! 🪄
func retrieveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	startTime := time.Now()

	// Parse request
	var req RetrievalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Query == "" {
		respondError(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.TopK == 0 {
		req.TopK = 5
	}
	if req.Collection == "" {
		req.Collection = "regulatory_docs"
	}

	log.Printf("🔍 Retrieval started: '%s' (TopK=%d, Collection=%s)",
		req.Query, req.TopK, req.Collection)

	// ========================================================================
	// STEP 1: Generate Query Embedding
	// ========================================================================
	// Convert user's text query into a vector so we can do semantic search
	log.Println("   Step 1/4: Generating query embedding...")
	queryEmbedding, err := getQueryEmbedding(req.Query)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to generate embedding: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("   ✓ Generated embedding (dimension: %d)", len(queryEmbedding))

	// ========================================================================
	// STEP 2: Search Vector Database
	// ========================================================================
	// Find the most similar chunks using cosine similarity
	log.Println("   Step 2/4: Searching vector database...")
	vectorResults, err := searchVectorDB(req.Collection, queryEmbedding, req.TopK, req.Filters)
	if err != nil {
		respondError(w, fmt.Sprintf("Vector search failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("   ✓ Found %d results", len(vectorResults))

	// ========================================================================
	// STEP 3: Enrich with Metadata
	// ========================================================================
	// Add document names, types, and other metadata to results
	log.Println("   Step 3/4: Enriching with metadata...")
	enrichedResults, err := enrichWithMetadata(vectorResults)
	if err != nil {
		respondError(w, fmt.Sprintf("Metadata enrichment failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Println("   ✓ Enriched results")

	// ========================================================================
	// STEP 4: Rerank Results
	// ========================================================================
	// Improve ranking by considering keyword matches
	log.Println("   Step 4/4: Reranking results...")
	rerankedResults := rerankResults(req.Query, enrichedResults)
	log.Println("   ✓ Reranked results")

	// Build response
	processTime := time.Since(startTime).Milliseconds()
	response := RetrievalResponse{
		Query:       req.Query,
		Results:     rerankedResults,
		Count:       len(rerankedResults),
		ProcessTime: float64(processTime),
	}

	log.Printf("✅ Retrieval completed in %dms (returned %d results)",
		processTime, len(rerankedResults))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// STEP 1: EMBEDDING
// ============================================================================

// getQueryEmbedding - Converts text query to vector embedding
func getQueryEmbedding(query string) ([]float32, error) {
	// Prepare request to embed service
	requestBody, _ := json.Marshal(map[string]string{
		"text": query,
	})

	// Call embed service
	resp, err := http.Post(
		EMBED_SERVICE_URL+"/embed",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call embed service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed service returned status: %d", resp.StatusCode)
	}

	// Parse response
	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	return result.Embedding, nil
}

// ============================================================================
// STEP 2: VECTOR SEARCH
// ============================================================================

// searchVectorDB - Finds similar chunks in Qdrant
func searchVectorDB(collection string, query []float32, topK int, filters map[string]string) ([]RetrievalResult, error) {
	// Prepare search request
	requestBody, _ := json.Marshal(map[string]interface{}{
		"collection": collection,
		"query":      query,
		"top_k":      topK,
		"filter":     filters,
	})

	// Call vector service
	resp, err := http.Post(
		VECTOR_SERVICE_URL+"/search",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call vector service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vector service returned status: %d", resp.StatusCode)
	}

	// Parse response
	var vectorResponse struct {
		Results []struct {
			ID      string                 `json:"id"`
			Score   float64                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vectorResponse); err != nil {
		return nil, fmt.Errorf("failed to decode vector response: %w", err)
	}

	// Convert to retrieval results
	results := make([]RetrievalResult, len(vectorResponse.Results))
	for i, r := range vectorResponse.Results {
		result := RetrievalResult{
			ID:       r.ID,
			Score:    r.Score,
			Metadata: r.Payload,
		}

		// Extract text and document ID from payload
		if text, ok := r.Payload["text"].(string); ok {
			result.Text = text
		}
		if docID, ok := r.Payload["document_id"].(string); ok {
			result.DocumentID = docID
		}

		results[i] = result
	}

	return results, nil
}

// ============================================================================
// STEP 3: METADATA ENRICHMENT
// ============================================================================

// enrichWithMetadata - Adds document names and metadata to results
func enrichWithMetadata(results []RetrievalResult) ([]RetrievalResult, error) {
	// Collect unique document IDs
	docIDs := make(map[string]bool)
	for _, r := range results {
		if r.DocumentID != "" {
			docIDs[r.DocumentID] = true
		}
	}

	// Fetch metadata for each document
	docMetadata := make(map[string]map[string]interface{})
	for docID := range docIDs {
		resp, err := http.Get(METADATA_SERVICE_URL + "/documents/" + docID)
		if err != nil {
			log.Printf("⚠️  Failed to fetch metadata for %s: %v", docID, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var doc map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&doc); err == nil {
				docMetadata[docID] = doc
			}
		}
	}

	// Enrich results with metadata
	enriched := make([]RetrievalResult, len(results))
	for i, r := range results {
		enriched[i] = r

		// Add document metadata if available
		if meta, ok := docMetadata[r.DocumentID]; ok {
			// Set source as document name
			if name, ok := meta["name"].(string); ok {
				enriched[i].Source = name
			}

			// Add metadata fields
			if enriched[i].Metadata == nil {
				enriched[i].Metadata = make(map[string]interface{})
			}
			enriched[i].Metadata["document_name"] = meta["name"]
			enriched[i].Metadata["document_type"] = meta["type"]
			enriched[i].Metadata["uploaded_at"] = meta["uploaded_at"]
		}
	}

	return enriched, nil
}

// ============================================================================
// STEP 4: RERANKING
// ============================================================================

// rerankResults - Improves ranking using keyword matching
// WHY RERANK? Vector search is good at semantic similarity, but might miss
// exact keyword matches. Reranking combines both approaches.
func rerankResults(query string, results []RetrievalResult) []RetrievalResult {
	// Split query into terms
	queryTerms := strings.Fields(strings.ToLower(query))

	// Score each result
	type scoredResult struct {
		result  RetrievalResult
		boosted float64
	}

	scored := make([]scoredResult, len(results))
	for i, r := range results {
		// Calculate keyword match score
		matchScore := calculateMatchScore(queryTerms, r.Text)

		// Combine vector score (70%) with keyword match (30%)
		boostedScore := (r.Score * 0.7) + (matchScore * 0.3)

		scored[i] = scoredResult{
			result:  r,
			boosted: boostedScore,
		}
	}

	// Sort by boosted score (simple bubble sort for clarity)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].boosted > scored[i].boosted {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Extract reranked results
	reranked := make([]RetrievalResult, len(scored))
	for i, s := range scored {
		reranked[i] = s.result
		reranked[i].Score = s.boosted // Update with new score
	}

	return reranked
}

// calculateMatchScore - Percentage of query terms found in text
func calculateMatchScore(queryTerms []string, text string) float64 {
	if len(queryTerms) == 0 {
		return 0
	}

	textLower := strings.ToLower(text)
	matches := 0

	for _, term := range queryTerms {
		if strings.Contains(textLower, term) {
			matches++
		}
	}

	return float64(matches) / float64(len(queryTerms))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

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
