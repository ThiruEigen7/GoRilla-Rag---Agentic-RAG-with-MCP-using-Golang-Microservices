// vector-service is a microservice that interacts with Qdrant to manage vector embeddings.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type UpsertRequest struct {
	Collection string                   `json:"collection"`
	Points     []map[string]interface{} `json:"points"`
}

type SearchRequest struct {
	Collection string                 `json:"collection"`
	Query      []float32              `json:"query"`
	TopK       int                    `json:"top_k"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
}

type SearchResult struct {
	ID      string                 `json:"id"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Count   int            `json:"count"`
}

var (
	collectionsClient qdrant.CollectionsClient
	pointsClient      qdrant.PointsClient
	systemClient      qdrant.QdrantClient
	grpcConn          *grpc.ClientConn
	clientOnce        sync.Once
	ctx               = context.Background()
)

func main() {
	qdrantAddr := getEnv("QDRANT_ADDRESS", "localhost:6334")

	clientOnce.Do(func() {
		var err error
		grpcConn, err = grpc.DialContext(ctx, qdrantAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect to Qdrant: %v", err)
		}
		collectionsClient = qdrant.NewCollectionsClient(grpcConn)
		pointsClient = qdrant.NewPointsClient(grpcConn)
		systemClient = qdrant.NewQdrantClient(grpcConn)
	})

	log.Printf("Connected to Qdrant at %s", qdrantAddr)
	initializeCollections()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/upsert", upsertHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/collections", collectionsHandler)

	port := getEnv("PORT", "8082")
	log.Printf("Vector Service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initializeCollections() {
	collections := []struct {
		name string
		size uint64
	}{
		{"regulatory_docs", 768},
		{"merchant_docs", 768},
		{"kyc_docs", 768},
	}

	for _, coll := range collections {
		_, err := collectionsClient.Get(ctx, &qdrant.GetCollectionInfoRequest{CollectionName: coll.name})
		if err == nil {
			continue
		}

		if status.Code(err) != codes.NotFound {
			log.Printf("Error checking collection %s: %v", coll.name, err)
			continue
		}

		log.Printf("Creating collection: %s", coll.name)
		_, err = collectionsClient.Create(ctx, &qdrant.CreateCollection{
			CollectionName: coll.name,
			VectorsConfig: &qdrant.VectorsConfig{
				Config: &qdrant.VectorsConfig_Params{
					Params: &qdrant.VectorParams{
						Size:     coll.size,
						Distance: qdrant.Distance_Cosine,
					},
				},
			},
		})
		if err != nil {
			log.Printf("Failed to create collection %s: %v", coll.name, err)
		} else {
			log.Printf("Collection %s created successfully", coll.name)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]string{"status": "healthy", "service": "vector-service"}
	if systemClient != nil {
		reply, err := systemClient.HealthCheck(ctx, &qdrant.HealthCheckRequest{})
		if err != nil {
			response["status"] = "degraded"
			response["error"] = err.Error()
		} else if reply.GetVersion() != "" {
			response["qdrant_version"] = reply.GetVersion()
		}
	}

	json.NewEncoder(w).Encode(response)
}

func collectionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	collections := []string{"regulatory_docs", "merchant_docs", "kyc_docs"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"collections": collections})
}

func upsertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Collection == "" {
		respondError(w, "Collection name required", http.StatusBadRequest)
		return
	}

	log.Printf("Upserting %d points to collection: %s", len(req.Points), req.Collection)

	qdrantPoints := make([]*qdrant.PointStruct, len(req.Points))
	for i, point := range req.Points {
		id, ok := point["id"].(string)
		if !ok {
			respondError(w, "Point ID must be a string", http.StatusBadRequest)
			return
		}

		vectorValue, ok := point["vector"]
		if !ok {
			respondError(w, "Point vector must be provided", http.StatusBadRequest)
			return
		}

		vector, err := convertVector(vectorValue)
		if err != nil {
			respondError(w, err.Error(), http.StatusBadRequest)
			return
		}

		payload := make(map[string]*qdrant.Value)
		if payloadRaw, ok := point["payload"].(map[string]interface{}); ok {
			for key, val := range payloadRaw {
				payload[key] = toQdrantValue(val)
			}
		}

		qdrantPoints[i] = &qdrant.PointStruct{
			Id: &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: id},
			},
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{
					Vector: &qdrant.Vector{Data: vector},
				},
			},
			Payload: payload,
		}
	}

	wait := true
	_, err := pointsClient.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: req.Collection,
		Points:         qdrantPoints,
		Wait:           &wait,
	})
	if err != nil {
		respondError(w, "Failed to upsert: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"collection": req.Collection,
		"points":     len(req.Points),
	})
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.TopK == 0 {
		req.TopK = 5
	}

	log.Printf("Searching in collection: %s, TopK: %d", req.Collection, req.TopK)

	withPayload := &qdrant.WithPayloadSelector{
		SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
	}

	searchResults, err := pointsClient.Search(ctx, &qdrant.SearchPoints{
		CollectionName: req.Collection,
		Vector:         req.Query,
		Limit:          uint64(req.TopK),
		WithPayload:    withPayload,
	})
	if err != nil {
		respondError(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	points := searchResults.GetResult()
	results := make([]SearchResult, len(points))
	for i, hit := range points {
		payload := make(map[string]interface{})
		for key, val := range hit.GetPayload() {
			payload[key] = fromQdrantValue(val)
		}

		results[i] = SearchResult{
			ID:      pointIDToString(hit.GetId()),
			Score:   float64(hit.GetScore()),
			Payload: payload,
		}
	}

	response := SearchResponse{Results: results, Count: len(results)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func toQdrantValue(val interface{}) *qdrant.Value {
	switch v := val.(type) {
	case string:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
	case int:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int32:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int64:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: v}}
	case float32:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: float64(v)}}
	case float64:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: v}}
	case bool:
		return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: v}}
	case []interface{}:
		values := make([]*qdrant.Value, len(v))
		for i, item := range v {
			values[i] = toQdrantValue(item)
		}
		return &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: &qdrant.ListValue{Values: values}}}
	case map[string]interface{}:
		fields := make(map[string]*qdrant.Value)
		for key, item := range v {
			fields[key] = toQdrantValue(item)
		}
		return &qdrant.Value{Kind: &qdrant.Value_StructValue{StructValue: &qdrant.Struct{Fields: fields}}}
	default:
		return &qdrant.Value{Kind: &qdrant.Value_NullValue{NullValue: qdrant.NullValue_NULL_VALUE}}
	}
}

func fromQdrantValue(val *qdrant.Value) interface{} {
	if val == nil {
		return nil
	}
	switch v := val.GetKind().(type) {
	case *qdrant.Value_StringValue:
		return v.StringValue
	case *qdrant.Value_IntegerValue:
		return v.IntegerValue
	case *qdrant.Value_DoubleValue:
		return v.DoubleValue
	case *qdrant.Value_BoolValue:
		return v.BoolValue
	case *qdrant.Value_ListValue:
		items := make([]interface{}, len(v.ListValue.GetValues()))
		for i, item := range v.ListValue.GetValues() {
			items[i] = fromQdrantValue(item)
		}
		return items
	case *qdrant.Value_StructValue:
		result := make(map[string]interface{})
		for key, field := range v.StructValue.GetFields() {
			result[key] = fromQdrantValue(field)
		}
		return result
	default:
		return nil
	}
}

func convertVector(raw interface{}) ([]float32, error) {
	switch v := raw.(type) {
	case []interface{}:
		vec := make([]float32, len(v))
		for i, item := range v {
			switch num := item.(type) {
			case float64:
				vec[i] = float32(num)
			case float32:
				vec[i] = num
			case int:
				vec[i] = float32(num)
			case int32:
				vec[i] = float32(num)
			case int64:
				vec[i] = float32(num)
			default:
				return nil, fmt.Errorf("vector contains non-numeric value")
			}
		}
		return vec, nil
	case []float64:
		vec := make([]float32, len(v))
		for i, num := range v {
			vec[i] = float32(num)
		}
		return vec, nil
	case []float32:
		return append([]float32(nil), v...), nil
	default:
		return nil, fmt.Errorf("point vector must be an array")
	}
}

func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	return strconv.FormatUint(id.GetNum(), 10)
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
