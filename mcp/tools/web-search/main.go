package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/search", searchHandler)

	port := getEnv("PORT", "9103")
	log.Printf("🌐 web-search tool starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "healthy", "tool": "web-search"}, http.StatusOK)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	query, _ := req["query"].(string)

	log.Printf("🌐 Searching web for: %s", query)

	// Simulated web search results
	results := []map[string]interface{}{
		{
			"title":   "RBI Updates Payment Aggregator Guidelines 2024",
			"url":     "https://rbi.org.in/guidelines/payment-aggregator-2024",
			"snippet": "The Reserve Bank of India has updated guidelines for payment aggregators, increasing minimum net worth requirement to Rs 25 crores...",
			"date":    "2024-01-15",
		},
		{
			"title":   "New KYC Norms for Fintech Companies",
			"url":     "https://example.com/kyc-norms-2024",
			"snippet": "Latest KYC requirements include enhanced verification for high-risk merchants and mandatory video KYC...",
			"date":    "2024-02-01",
		},
		{
			"title":   "Merchant Onboarding Best Practices",
			"url":     "https://example.com/merchant-onboarding",
			"snippet": "Complete guide to merchant onboarding including document requirements, risk assessment, and compliance...",
			"date":    "2023-12-10",
		},
	}

	result := map[string]interface{}{
		"query":     query,
		"results":   results,
		"count":     len(results),
		"timestamp": time.Now().Format(time.RFC3339),
		"source":    "simulated_web_search",
	}

	respondJSON(w, result, http.StatusOK)
}

func respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
