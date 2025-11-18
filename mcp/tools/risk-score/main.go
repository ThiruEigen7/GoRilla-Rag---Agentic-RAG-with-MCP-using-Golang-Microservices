package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/calculate", calculateHandler)

	port := getEnv("PORT", "9102")
	log.Printf("⚠️  risk-score tool starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "healthy", "tool": "risk-score"}, http.StatusOK)
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	merchantData, _ := req["merchant_data"].(map[string]interface{})

	log.Printf("⚠️  Calculating risk score for merchant")

	// Calculate risk score based on various factors
	score := calculateRiskScore(merchantData)
	category := getRiskCategory(score)

	result := map[string]interface{}{
		"risk_score":    score,
		"risk_category": category,
		"factors": []map[string]interface{}{
			{"factor": "Business Age", "score": 0.2, "weight": 0.3},
			{"factor": "Transaction Volume", "score": 0.3, "weight": 0.3},
			{"factor": "Industry Type", "score": 0.4, "weight": 0.2},
			{"factor": "Compliance History", "score": 0.1, "weight": 0.2},
		},
		"recommendations": []string{},
	}

	if category == "high" {
		result["recommendations"] = []string{
			"Require additional documentation",
			"Implement enhanced monitoring",
			"Limit initial transaction volume",
		}
	} else if category == "medium" {
		result["recommendations"] = []string{
			"Standard monitoring procedures",
			"Periodic reviews required",
		}
	} else {
		result["recommendations"] = []string{
			"Standard onboarding process",
			"Regular compliance checks",
		}
	}

	respondJSON(w, result, http.StatusOK)
}

func calculateRiskScore(data map[string]interface{}) float64 {
	// Simplified risk calculation
	score := 0.0

	if businessAge, ok := data["business_age"].(float64); ok {
		if businessAge < 1 {
			score += 0.3
		} else if businessAge < 3 {
			score += 0.2
		} else {
			score += 0.1
		}
	}

	if turnover, ok := data["annual_turnover"].(float64); ok {
		if turnover > 50000000 { // > 5 crores
			score += 0.3
		} else if turnover > 5000000 { // > 50 lakhs
			score += 0.2
		} else {
			score += 0.1
		}
	}

	if industry, ok := data["industry"].(string); ok {
		highRiskIndustries := []string{"gaming", "forex", "crypto"}
		for _, hr := range highRiskIndustries {
			if industry == hr {
				score += 0.4
				break
			}
		}
	}

	return math.Min(score, 1.0)
}

func getRiskCategory(score float64) string {
	if score >= 0.7 {
		return "high"
	} else if score >= 0.4 {
		return "medium"
	}
	return "low"
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
