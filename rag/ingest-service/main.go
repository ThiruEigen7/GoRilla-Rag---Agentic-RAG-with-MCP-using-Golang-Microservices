// rag/ingest-service/main.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

// ============================================================================
// DATA MODELS
// ============================================================================

type Document struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	FilePath   string    `json:"file_path"`
	Status     string    `json:"status"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type Chunk struct {
	ID         string `json:"id"`
	DocumentID string `json:"document_id"`
	Text       string `json:"text"`
	Position   int    `json:"position"`
}

type IngestRequest struct {
	DocumentName string `json:"document_name"`
	DocumentType string `json:"document_type"`
	FilePath     string `json:"file_path"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
}

type IngestResponse struct {
	DocumentID string `json:"document_id"`
	Status     string `json:"status"`
	Chunks     int    `json:"chunks"`
	Message    string `json:"message"`
}

// ============================================================================
// ENV + CONFIG
// ============================================================================

var (
	EMBED_SERVICE_URL    = getEnv("EMBED_SERVICE_URL", "http://localhost:8081")
	VECTOR_SERVICE_URL   = getEnv("VECTOR_SERVICE_URL", "http://localhost:8082")
	METADATA_SERVICE_URL = getEnv("METADATA_SERVICE_URL", "http://localhost:8083")
	DATA_DIR             = getEnv("DATA_DIR", "./data/docs")
)

// ============================================================================
// MAIN ENTRY
// ============================================================================

func main() {
	if err := os.MkdirAll(DATA_DIR, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/ingest", ingestHandler)

	port := getEnv("PORT", "8080")
	log.Printf("Ingest Service running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ============================================================================
// HEALTH
// ============================================================================
func healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{
		"status":  "healthy",
		"service": "ingest-service",
	})
}

// ============================================================================
// FILE UPLOAD HANDLER
// ============================================================================
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		respondError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileID := uuid.New().String()
	filePath := fmt.Sprintf("%s/%s_%s", DATA_DIR, fileID, header.Filename)

	dst, err := os.Create(filePath)
	if err != nil {
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		respondError(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{
		"file_id":   fileID,
		"file_name": header.Filename,
		"file_path": filePath,
	})
}

// ============================================================================
// INGEST HANDLER
// ============================================================================
func ingestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ChunkSize == 0 {
		req.ChunkSize = 500
	}
	if req.ChunkOverlap == 0 {
		req.ChunkOverlap = 50
	}

	log.Printf("Ingesting document: %s", req.DocumentName)

	// --- PDF/TXT extraction
	text, err := extractText(req.FilePath)
	if err != nil {
		respondError(w, "Failed to extract text: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(strings.TrimSpace(text)) < 10 {
		respondError(w, "No readable text found in the document", http.StatusBadRequest)
		return
	}

	// --- Create metadata
	doc := Document{
		ID:         uuid.New().String(),
		Name:       req.DocumentName,
		Type:       req.DocumentType,
		FilePath:   req.FilePath,
		Status:     "processing",
		UploadedAt: time.Now(),
	}

	if err := saveDocumentMetadata(doc); err != nil {
		respondError(w, "Failed to save metadata: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// --- Chunk
	chunks := chunkText(text, doc.ID, req.ChunkSize, req.ChunkOverlap)
	log.Printf("Chunks created: %d", len(chunks))

	// --- Embed using embed-service
	embeddings, err := getEmbeddings(chunks)
	if err != nil {
		updateDocumentStatus(doc.ID, "failed")
		respondError(w, "Embedding failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// --- Store vectors
	if err := storeVectors(chunks, embeddings, req.DocumentType); err != nil {
		updateDocumentStatus(doc.ID, "failed")
		respondError(w, "Vector storage failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	updateDocumentStatus(doc.ID, "completed")

	// --- Final response
	jsonResponse(w, IngestResponse{
		DocumentID: doc.ID,
		Status:     "completed",
		Chunks:     len(chunks),
		Message:    "Ingestion finished successfully",
	})
}

// ============================================================================
// TEXT EXTRACTION
// ============================================================================

func extractText(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath)) // FIXED

	switch ext {
	case ".txt":
		return extractTextFromTXT(filePath)
	case ".pdf":
		return extractTextFromPDF(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
}

func extractTextFromTXT(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func extractTextFromPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open PDF: %w", err)
	}
	defer f.Close()

	var out strings.Builder

	total := r.NumPage()
	log.Printf("PDF pages: %d", total)

	for i := 1; i <= total; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		txt, err := page.GetPlainText(nil)
		if err != nil {
			log.Printf("PDF page %d error: %v", i, err)
			continue
		}

		clean := cleanText(txt)
		if clean != "" {
			out.WriteString(clean + "\n\n")
		}
	}

	result := out.String()
	if len(strings.TrimSpace(result)) == 0 {
		return "", fmt.Errorf("no extractable text found")
	}

	return result, nil
}

func cleanText(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len(l) > 0 {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}

// ============================================================================
// CHUNKING
// ============================================================================

func chunkText(text, docID string, size, overlap int) []Chunk {
	var chunks []Chunk
	runes := []rune(text)
	pos := 0

	for i := 0; i < len(runes); i += size - overlap {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}

		part := strings.TrimSpace(string(runes[i:end]))
		if len(part) == 0 {
			continue
		}

		chunks = append(chunks, Chunk{
			ID:         uuid.New().String(),
			DocumentID: docID,
			Text:       part,
			Position:   pos,
		})

		pos++
		if end >= len(runes) {
			break
		}
	}

	return chunks
}

// ============================================================================
// EMBEDDING SERVICE CALL
// ============================================================================

func getEmbeddings(chunks []Chunk) ([][]float32, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	body, _ := json.Marshal(map[string]interface{}{
		"texts": texts,
	})

	resp, err := http.Post(EMBED_SERVICE_URL+"/embed-batch", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return out.Embeddings, nil
}

// ============================================================================
// VECTOR SERVICE CALL
// ============================================================================

func storeVectors(chunks []Chunk, embeddings [][]float32, docType string) error {
	points := make([]map[string]interface{}, len(chunks))

	for i, c := range chunks {
		points[i] = map[string]interface{}{
			"id":     c.ID,
			"vector": embeddings[i],
			"payload": map[string]interface{}{
				"text":        c.Text,
				"document_id": c.DocumentID,
				"position":    c.Position,
			},
		}
	}

	collection := "regulatory_docs"
	if docType == "merchant" {
		collection = "merchant_docs"
	} else if docType == "kyc" {
		collection = "kyc_docs"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"collection": collection,
		"points":     points,
	})

	resp, err := http.Post(VECTOR_SERVICE_URL+"/upsert", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ============================================================================
// METADATA SERVICE CALL
// ============================================================================

func saveDocumentMetadata(doc Document) error {
	body, _ := json.Marshal(doc)
	resp, err := http.Post(METADATA_SERVICE_URL+"/documents", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func updateDocumentStatus(id, status string) error {
	body, _ := json.Marshal(map[string]string{"status": status})

	req, _ := http.NewRequest(http.MethodPut, METADATA_SERVICE_URL+"/documents/"+id+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	_, err := http.DefaultClient.Do(req)
	return err
}

// ============================================================================
// HELPERS
// ============================================================================

func respondError(w http.ResponseWriter, msg string, code int) {
	w.WriteHeader(code)
	jsonResponse(w, map[string]string{"error": msg})
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
