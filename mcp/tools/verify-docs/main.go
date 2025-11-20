package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/verify", verifyHandler)

	port := getEnv("PORT", "9101")
	log.Printf("🔍 verify-docs tool starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "healthy", "tool": "verify-docs"}, http.StatusOK)
}

func verifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	docType, _ := req["document_type"].(string)

	log.Printf("🔍 Verifying document type: %s", docType)

	// Simulate document verification
	result := map[string]interface{}{
		"valid":          true,
		"extracted_data": map[string]interface{}{},
		"issues":         []string{},
	}

	switch strings.ToLower(docType) {
	case "pan":
		result["extracted_data"] = map[string]string{
			"pan_number": "ABCDE1234F",
			"name":       "Sample Merchant",
			"dob":        "01/01/1990",
		}
		result["checks"] = map[string]bool{
			"format_valid": true,
			"name_matches": true,
			"not_expired":  true,
		}

	case "gst":
		result["extracted_data"] = map[string]string{
			"gst_number":        "27ABCDE1234F1Z5",
			"business_name":     "Sample Business Pvt Ltd",
			"registration_date": "01/01/2020",
		}
		result["checks"] = map[string]bool{
			"format_valid":  true,
			"active_status": true,
			"verified":      true,
		}

	case "bank_statement":
		result["extracted_data"] = map[string]interface{}{
			"account_number":  "1234567890",
			"bank_name":       "Sample Bank",
			"average_balance": 250000,
			"months_covered":  6,
		}
		result["checks"] = map[string]bool{
			"sufficient_balance":     true,
			"regular_transactions":   true,
			"no_suspicious_activity": true,
		}

	case "kyc":
		result["required_documents"] = []string{
			"PAN Card",
			"GST Certificate",
			"Bank Statements (6 months)",
			"Business Registration",
			"Address Proof",
		}
		result["checks"] = map[string]bool{
			"all_present": false,
			"verified":    false,
		}
		result["missing"] = []string{"Bank Statements"}

	default:
		result["valid"] = false
		result["issues"] = []string{"Unknown document type"}
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
