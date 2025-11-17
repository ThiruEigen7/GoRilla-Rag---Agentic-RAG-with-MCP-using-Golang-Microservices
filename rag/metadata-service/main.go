//metadata-service is a microservice that manages document metadata using SQLite.
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Document struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	FilePath   string    `json:"file_path"`
	Status     string    `json:"status"`
	UploadedAt time.Time `json:"uploaded_at"`
}

var db *sql.DB

func main() {
	dbPath := getEnv("DB_PATH", "./data/metadata.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := initializeDatabase(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/documents", documentsHandler)
	http.HandleFunc("/documents/", documentByIDHandler)

	port := getEnv("PORT", "8083")
	log.Printf("Metadata Service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initializeDatabase() error {
	schema := `
	CREATE TABLE IF NOT EXISTS documents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		file_path TEXT NOT NULL,
		status TEXT NOT NULL,
		uploaded_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_documents_type ON documents(type);
	CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);`
	_, err := db.Exec(schema)
	return err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "metadata-service"})
}

func documentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getDocuments(w, r)
	case http.MethodPost:
		createDocument(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getDocuments(w http.ResponseWriter, r *http.Request) {
	query := "SELECT id, name, type, file_path, status, uploaded_at FROM documents ORDER BY uploaded_at DESC"
	rows, err := db.Query(query)
	if err != nil {
		respondError(w, "Query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var doc Document
		rows.Scan(&doc.ID, &doc.Name, &doc.Type, &doc.FilePath, &doc.Status, &doc.UploadedAt)
		documents = append(documents, doc)
	}

	if documents == nil {
		documents = []Document{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"documents": documents, "count": len(documents)})
}

func createDocument(w http.ResponseWriter, r *http.Request) {
	var doc Document
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if doc.Status == "" {
		doc.Status = "pending"
	}

	query := `INSERT INTO documents (id, name, type, file_path, status, uploaded_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, doc.ID, doc.Name, doc.Type, doc.FilePath, doc.Status, doc.UploadedAt)
	if err != nil {
		respondError(w, "Failed to insert document", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

func documentByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/documents/"):]
	if id == "" {
		respondError(w, "Document ID required", http.StatusBadRequest)
		return
	}

	if len(id) > 7 && id[len(id)-7:] == "/status" {
		docID := id[:len(id)-7]
		updateDocumentStatus(w, r, docID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getDocumentByID(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getDocumentByID(w http.ResponseWriter, r *http.Request, id string) {
	var doc Document
	err := db.QueryRow("SELECT id, name, type, file_path, status, uploaded_at FROM documents WHERE id = ?", id).
		Scan(&doc.ID, &doc.Name, &doc.Type, &doc.FilePath, &doc.Status, &doc.UploadedAt)
	if err == sql.ErrNoRows {
		respondError(w, "Document not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func updateDocumentStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	_, err := db.Exec("UPDATE documents SET status = ? WHERE id = ?", req.Status, id)
	if err != nil {
		respondError(w, "Update failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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
