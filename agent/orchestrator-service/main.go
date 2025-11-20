// agent/orchestrator-service/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genai"
)

// ============================================================================
// DATA MODELS
// ============================================================================

// AgentRequest - User's query to the agent
type AgentRequest struct {
	Query          string            `json:"query"`
	ConversationID string            `json:"conversation_id,omitempty"`
	MaxIterations  int               `json:"max_iterations,omitempty"`
	Context        map[string]string `json:"context,omitempty"`
}

// AgentResponse - Final response from agent
type AgentResponse struct {
	ConversationID string      `json:"conversation_id"`
	Query          string      `json:"query"`
	Answer         string      `json:"answer"`
	Confidence     float64     `json:"confidence"`
	Iterations     int         `json:"iterations"`
	ToolsUsed      []string    `json:"tools_used"`
	Sources        []string    `json:"sources"`
	ProcessTime    float64     `json:"process_time_ms"`
	Steps          []AgentStep `json:"steps"`
	NeedMoreInfo   bool        `json:"need_more_info"`
	FollowUpQ      string      `json:"follow_up_question,omitempty"`
}

// AgentStep - Individual step in agent's reasoning
type AgentStep struct {
	StepNumber  int     `json:"step_number"`
	Type        string  `json:"type"` // "analyze", "plan", "execute", "verify"
	Description string  `json:"description"`
	Action      string  `json:"action,omitempty"`
	Result      string  `json:"result,omitempty"`
	Success     bool    `json:"success"`
	Duration    float64 `json:"duration_ms"`
}

// ExecutionPlan - Agent's plan of action
type ExecutionPlan struct {
	OriginalQuery    string   `json:"original_query"`
	RewrittenQueries []string `json:"rewritten_queries"`
	Actions          []Action `json:"actions"`
	Reasoning        string   `json:"reasoning"`
}

// Action - Individual action in the plan
type Action struct {
	Type        string                 `json:"type"` // "search_rag", "call_tool", "synthesize"
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Conversation - Stores conversation history
type Conversation struct {
	ID        string
	Messages  []Message
	StartTime time.Time
}

// Message - Single message in conversation
type Message struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ============================================================================
// GLOBAL STATE
// ============================================================================

var (
	geminiClient  *genai.Client
	conversations = make(map[string]*Conversation)

	// Service URLs
	RAG_SERVICE_URL    = getEnv("RAG_SERVICE_URL", "http://localhost:8084")
	MCP_GATEWAY_URL    = getEnv("MCP_GATEWAY_URL", "http://localhost:9100")
	QUERY_REWRITER_URL = getEnv("QUERY_REWRITER_URL", "http://localhost:9001")

	// Agent settings
	MAX_ITERATIONS       = 5
	CONFIDENCE_THRESHOLD = 0.7
)

// ============================================================================
// MAIN
// ============================================================================

func main() {
	// Initialize Gemini client
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable not set")
	}

	var err error
	ctx := context.Background()
	geminiClient, err = genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	log.Println("✅ Gemini client initialized")

	// Setup routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/agent/query", agentQueryHandler)
	http.HandleFunc("/agent/plan", planHandler)
	http.HandleFunc("/agent/history/", historyHandler)

	port := getEnv("PORT", "9000")
	log.Printf("🤖 Agent Orchestrator Service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ============================================================================
// HTTP HANDLERS
// ============================================================================

func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{
		"status":  "healthy",
		"service": "agent-orchestrator",
	}, http.StatusOK)
}

// Main agentic query handler
func agentQueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	startTime := time.Now()

	var req AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		respondError(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	if req.MaxIterations == 0 {
		req.MaxIterations = MAX_ITERATIONS
	}

	// Create or get conversation
	if req.ConversationID == "" {
		req.ConversationID = uuid.New().String()
	}

	log.Printf("🤖 Agent processing query: '%s' (conversation: %s)", req.Query, req.ConversationID)

	// Execute agentic loop
	response := executeAgenticLoop(req)
	response.ProcessTime = float64(time.Since(startTime).Milliseconds())

	log.Printf("✅ Agent completed in %.2fms (%d iterations)", response.ProcessTime, response.Iterations)

	respondJSON(w, response, http.StatusOK)
}

// Get execution plan without executing
func planHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	plan, err := createExecutionPlan(req.Query, req.Context)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to create plan: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, plan, http.StatusOK)
}

// Get conversation history
func historyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	conversationID := strings.TrimPrefix(r.URL.Path, "/agent/history/")
	if conversationID == "" {
		respondError(w, "Conversation ID required", http.StatusBadRequest)
		return
	}

	conv, exists := conversations[conversationID]
	if !exists {
		respondError(w, "Conversation not found", http.StatusNotFound)
		return
	}

	respondJSON(w, conv, http.StatusOK)
}

// ============================================================================
// AGENTIC LOOP - THE CORE LOGIC
// ============================================================================

func executeAgenticLoop(req AgentRequest) AgentResponse {
	response := AgentResponse{
		ConversationID: req.ConversationID,
		Query:          req.Query,
		Steps:          []AgentStep{},
		ToolsUsed:      []string{},
		Sources:        []string{},
	}

	var finalAnswer string
	var confidence float64

	// Agentic loop with max iterations
	for iteration := 1; iteration <= req.MaxIterations; iteration++ {
		log.Printf("  🔄 Iteration %d/%d", iteration, req.MaxIterations)

		// STEP 1: ANALYZE QUERY
		step1Start := time.Now()
		analysis := analyzeQuery(req.Query, req.Context)
		response.Steps = append(response.Steps, AgentStep{
			StepNumber:  len(response.Steps) + 1,
			Type:        "analyze",
			Description: "Analyze user query and intent",
			Result:      analysis,
			Success:     true,
			Duration:    float64(time.Since(step1Start).Milliseconds()),
		})
		log.Printf("    ✓ Analysis: %s", analysis)

		// STEP 2: CREATE EXECUTION PLAN
		step2Start := time.Now()
		plan, err := createExecutionPlan(req.Query, req.Context)
		if err != nil {
			response.Steps = append(response.Steps, AgentStep{
				StepNumber:  len(response.Steps) + 1,
				Type:        "plan",
				Description: "Create execution plan",
				Success:     false,
				Duration:    float64(time.Since(step2Start).Milliseconds()),
			})
			response.Answer = fmt.Sprintf("Failed to create plan: %v", err)
			return response
		}
		response.Steps = append(response.Steps, AgentStep{
			StepNumber:  len(response.Steps) + 1,
			Type:        "plan",
			Description: "Create execution plan",
			Result:      plan.Reasoning,
			Success:     true,
			Duration:    float64(time.Since(step2Start).Milliseconds()),
		})
		log.Printf("    ✓ Plan created with %d actions", len(plan.Actions))

		// STEP 3: EXECUTE ACTIONS
		step3Start := time.Now()
		executionResults := executeActions(plan.Actions, &response)
		response.Steps = append(response.Steps, AgentStep{
			StepNumber:  len(response.Steps) + 1,
			Type:        "execute",
			Description: fmt.Sprintf("Execute %d actions", len(plan.Actions)),
			Result:      fmt.Sprintf("Executed %d actions", len(executionResults)),
			Success:     true,
			Duration:    float64(time.Since(step3Start).Milliseconds()),
		})
		log.Printf("    ✓ Executed %d actions", len(executionResults))

		// STEP 4: SYNTHESIZE ANSWER
		step4Start := time.Now()
		finalAnswer = synthesizeAnswer(req.Query, executionResults)
		response.Steps = append(response.Steps, AgentStep{
			StepNumber:  len(response.Steps) + 1,
			Type:        "synthesize",
			Description: "Synthesize final answer",
			Result:      fmt.Sprintf("Generated answer (%d chars)", len(finalAnswer)),
			Success:     true,
			Duration:    float64(time.Since(step4Start).Milliseconds()),
		})
		log.Printf("    ✓ Answer synthesized")

		// STEP 5: VERIFY ANSWER
		step5Start := time.Now()
		verification := verifyAnswer(req.Query, finalAnswer, executionResults)
		confidence = verification.Confidence
		response.Steps = append(response.Steps, AgentStep{
			StepNumber:  len(response.Steps) + 1,
			Type:        "verify",
			Description: "Verify answer quality",
			Result:      fmt.Sprintf("Confidence: %.2f, Complete: %v", verification.Confidence, verification.IsComplete),
			Success:     true,
			Duration:    float64(time.Since(step5Start).Milliseconds()),
		})
		log.Printf("    ✓ Verification: confidence=%.2f, complete=%v", verification.Confidence, verification.IsComplete)

		// STEP 6: DECIDE IF DONE
		if verification.IsComplete && verification.Confidence >= CONFIDENCE_THRESHOLD {
			log.Printf("  ✅ Answer is satisfactory (confidence: %.2f)", confidence)
			response.NeedMoreInfo = false
			break
		}

		// Need another iteration
		log.Printf("  ⚠️  Answer not satisfactory (confidence: %.2f), iterating...", confidence)

		if iteration >= req.MaxIterations {
			log.Printf("  ⚠️  Max iterations reached")
			response.NeedMoreInfo = true
			response.FollowUpQ = "I need more information to answer completely. Can you provide more context about: " + verification.MissingInfo
			break
		}

		// Update query with more context for next iteration
		req.Query = enhanceQueryForIteration(req.Query, verification.MissingInfo)
	}

	response.Answer = finalAnswer
	response.Confidence = confidence
	response.Iterations = len(response.Steps) / 5 // Roughly 5 steps per iteration

	// Store conversation
	storeConversation(req.ConversationID, req.Query, finalAnswer)

	return response
}

// ============================================================================
// STEP 1: ANALYZE QUERY
// ============================================================================

func analyzeQuery(query string, ctxMap map[string]string) string {
	ctx := context.Background()
	modelName := "gemini-2.5-pro"

	prompt := fmt.Sprintf(`Analyze this user query and provide a brief analysis:

Query: "%s"

Provide:
1. Query type (question, request, command)
2. Domain (compliance, kyc, risk, general)
3. Intent (what user wants)
4. Complexity (simple, medium, complex)

Answer in 2-3 sentences.`, query)

	if len(ctxMap) > 0 {
		prompt += fmt.Sprintf("\n\nAdditional context: %v", ctxMap)
	}

	resp, err := geminiClient.Models.GenerateContent(ctx, modelName, genai.Text(prompt), nil)
	if err != nil {
		log.Printf("Analysis failed: %v", err)
		return "Unable to analyze query"
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		parts := resp.Candidates[0].Content.Parts
		if len(parts) > 0 {
			return fmt.Sprintf("%v", parts[0])
		}
	}

	return "Query analysis completed"
}

// ============================================================================
// STEP 2: CREATE EXECUTION PLAN
// ============================================================================

func createExecutionPlan(query string, ctxMap map[string]string) (*ExecutionPlan, error) {
	ctx := context.Background()
	modelName := "gemini-2.5-pro"

	prompt := fmt.Sprintf(`You are an AI agent planning how to answer a user query.

Query: "%s"

Available actions:
1. search_rag - Search knowledge base (collections: regulatory_docs, merchant_docs, kyc_docs)
2. call_tool - Call MCP tools (tools: verify-docs, risk-score, web-search, data-extractor)
3. synthesize - Combine information

Create a plan with 2-4 actions. For each action specify:
- type (one of above)
- description (what this action does)
- parameters (what parameters to pass)

Respond ONLY in JSON format:
{
  "rewritten_queries": ["query1", "query2"],
  "actions": [
    {"type": "search_rag", "description": "...", "parameters": {"query": "...", "collection": "..."}}
  ],
  "reasoning": "Why this plan will work"
}`, query)

	resp, err := geminiClient.Models.GenerateContent(ctx, modelName, genai.Text(prompt), nil)
	if err != nil {
		return nil, err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("no response from model")
	}

	// Extract JSON from response
	parts := resp.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	responseText := fmt.Sprintf("%v", parts[0])

	// Clean JSON (remove markdown if present)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var plan ExecutionPlan
	plan.OriginalQuery = query

	if err := json.Unmarshal([]byte(responseText), &plan); err != nil {
		// If JSON parsing fails, create a simple default plan
		log.Printf("Failed to parse plan JSON, using default: %v", err)
		plan.RewrittenQueries = []string{query}
		plan.Actions = []Action{
			{
				Type:        "search_rag",
				Description: "Search knowledge base",
				Parameters: map[string]interface{}{
					"query":      query,
					"collection": "regulatory_docs",
					"top_k":      5,
				},
			},
		}
		plan.Reasoning = "Default plan: search knowledge base"
	}

	return &plan, nil
}

// ============================================================================
// STEP 3: EXECUTE ACTIONS
// ============================================================================

func executeActions(actions []Action, response *AgentResponse) []map[string]interface{} {
	results := []map[string]interface{}{}

	for i, action := range actions {
		log.Printf("      Action %d/%d: %s", i+1, len(actions), action.Type)

		var result map[string]interface{}
		var err error

		switch action.Type {
		case "search_rag":
			result, err = executeSearchRAG(action.Parameters)
			if err == nil {
				response.Sources = append(response.Sources, "RAG Knowledge Base")
			}

		case "call_tool":
			result, err = executeCallTool(action.Parameters)
			if err == nil {
				if toolName, ok := action.Parameters["tool"].(string); ok {
					response.ToolsUsed = append(response.ToolsUsed, toolName)
				}
			}

		case "synthesize":
			// Synthesis happens later
			result = map[string]interface{}{"status": "deferred"}

		default:
			err = fmt.Errorf("unknown action type: %s", action.Type)
		}

		if err != nil {
			log.Printf("        ✗ Action failed: %v", err)
			result = map[string]interface{}{
				"error":  err.Error(),
				"status": "failed",
			}
		} else {
			log.Printf("        ✓ Action completed")
		}

		result["action_type"] = action.Type
		results = append(results, result)
	}

	return results
}

func executeSearchRAG(params map[string]interface{}) (map[string]interface{}, error) {
	query, _ := params["query"].(string)
	collection, _ := params["collection"].(string)
	topK, _ := params["top_k"].(float64)

	if query == "" {
		query = "default query"
	}
	if collection == "" {
		collection = "regulatory_docs"
	}
	if topK == 0 {
		topK = 5
	}

	requestBody, _ := json.Marshal(map[string]interface{}{
		"query":      query,
		"collection": collection,
		"top_k":      int(topK),
	})

	resp, err := http.Post(
		RAG_SERVICE_URL+"/retrieve",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func executeCallTool(params map[string]interface{}) (map[string]interface{}, error) {
	toolName, _ := params["tool"].(string)
	if toolName == "" {
		return nil, fmt.Errorf("tool name required")
	}

	requestBody, _ := json.Marshal(map[string]interface{}{
		"tool":   toolName,
		"params": params,
	})

	resp, err := http.Post(
		MCP_GATEWAY_URL+"/tools/call",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// ============================================================================
// STEP 4: SYNTHESIZE ANSWER
// ============================================================================

func synthesizeAnswer(query string, results []map[string]interface{}) string {
	ctx := context.Background()
	modelName := "gemini-2.5-pro"

	// Prepare context from results
	contextStr := "Information gathered:\n\n"
	for i, result := range results {
		contextStr += fmt.Sprintf("%d. %v\n\n", i+1, result)
	}

	prompt := fmt.Sprintf(`Based on the information below, answer this question:

Question: "%s"

%s

Provide a clear, concise answer. If information is insufficient, say so.`, query, contextStr)

	resp, err := geminiClient.Models.GenerateContent(ctx, modelName, genai.Text(prompt), nil)
	if err != nil {
		log.Printf("Synthesis failed: %v", err)
		return "Unable to synthesize answer from available information."
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		parts := resp.Candidates[0].Content.Parts
		if len(parts) > 0 {
			return fmt.Sprintf("%v", parts[0])
		}
	}

	return "No answer could be generated."
}

// ============================================================================
// STEP 5: VERIFY ANSWER
// ============================================================================

type Verification struct {
	IsComplete  bool
	Confidence  float64
	MissingInfo string
}

func verifyAnswer(query string, answer string, results []map[string]interface{}) Verification {
	ctx := context.Background()
	modelName := "gemini-2.5-pro"

	prompt := fmt.Sprintf(`Evaluate this answer:

Question: "%s"
Answer: "%s"

Is the answer:
1. Complete (addresses the question fully)
2. Accurate (based on the information)
3. Relevant (stays on topic)

Respond in JSON:
{
  "is_complete": true/false,
  "confidence": 0.0-1.0,
  "missing_info": "what's missing (if not complete)"
}`, query, answer)

	resp, err := geminiClient.Models.GenerateContent(ctx, modelName, genai.Text(prompt), nil)
	if err != nil {
		log.Printf("Verification failed: %v", err)
		return Verification{IsComplete: true, Confidence: 0.5, MissingInfo: ""}
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		parts := resp.Candidates[0].Content.Parts
		if len(parts) > 0 {
			responseText := fmt.Sprintf("%v", parts[0])
			responseText = strings.TrimPrefix(responseText, "```json")
			responseText = strings.TrimPrefix(responseText, "```")
			responseText = strings.TrimSuffix(responseText, "```")
			responseText = strings.TrimSpace(responseText)

			var v Verification
			if err := json.Unmarshal([]byte(responseText), &v); err != nil {
				log.Printf("Failed to parse verification: %v", err)
				return Verification{IsComplete: true, Confidence: 0.7, MissingInfo: ""}
			}
			return v
		}
	}

	return Verification{IsComplete: true, Confidence: 0.7, MissingInfo: ""}
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func enhanceQueryForIteration(originalQuery, missingInfo string) string {
	if missingInfo == "" {
		return originalQuery
	}
	return fmt.Sprintf("%s (specifically about: %s)", originalQuery, missingInfo)
}

func storeConversation(conversationID, query, answer string) {
	conv, exists := conversations[conversationID]
	if !exists {
		conv = &Conversation{
			ID:        conversationID,
			Messages:  []Message{},
			StartTime: time.Now(),
		}
		conversations[conversationID] = conv
	}

	conv.Messages = append(conv.Messages,
		Message{Role: "user", Content: query, Timestamp: time.Now()},
		Message{Role: "assistant", Content: answer, Timestamp: time.Now()},
	)
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
