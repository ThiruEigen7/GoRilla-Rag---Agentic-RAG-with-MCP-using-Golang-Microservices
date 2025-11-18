 $$ Agentic Era $$

 # RAG System - Complete cURL Commands Reference

## 📋 Table of Contents

1. [Health Checks](#health-checks)
2. [Document Upload & Ingestion](#document-upload--ingestion)
3. [Search & Retrieval](#search--retrieval)
4. [Metadata Operations](#metadata-operations)
5. [Vector Operations](#vector-operations)
6. [Embedding Operations](#embedding-operations)
7. [Complete Workflows](#complete-workflows)

---

## 🏥 Health Checks

### Check All Services

```bash
# Ingest Service
curl http://localhost:8080/health

# Embed Service
curl http://localhost:8081/health

# Vector Service
curl http://localhost:8082/health

# Metadata Service
curl http://localhost:8083/health

# Retrieval Service
curl http://localhost:8084/health
```

### Expected Response
```json
{
  "status": "healthy",
  "service": "service-name"
}
```

---

## 📤 Document Upload & Ingestion

### 1. Upload a File

```bash
# Upload PDF
curl -X POST http://localhost:8080/upload \
  -F "file=@/path/to/your/document.pdf"

# Upload Text File
curl -X POST http://localhost:8080/upload \
  -F "file=@/path/to/your/document.txt"
```

**Response:**
```json
{
  "file_path": "./data/docs/abc123_document.pdf",
  "file_name": "document.pdf",
  "file_id": "abc123-def456-ghi789"
}
```

### 2. Ingest Document (Basic)

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "document_name": "RBI Payment Guidelines 2023",
    "document_type": "regulatory",
    "file_path": "./data/docs/abc123_document.pdf"
  }'
```

### 3. Ingest Document (With Custom Chunking)

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "document_name": "Merchant KYC Requirements",
    "document_type": "kyc",
    "file_path": "./data/docs/xyz789_kyc_doc.pdf",
    "chunk_size": 500,
    "chunk_overlap": 50
  }'
```

### 4. Ingest to Specific Collection

```bash
# Regulatory documents → regulatory_docs collection
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "document_name": "FSA Compliance Guide",
    "document_type": "regulatory",
    "file_path": "./data/docs/fsa_guide.pdf"
  }'

# Merchant documents → merchant_docs collection
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "document_name": "Merchant Agreement Template",
    "document_type": "merchant",
    "file_path": "./data/docs/merchant_agreement.pdf"
  }'

# KYC documents → kyc_docs collection
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "document_name": "KYC Verification Guide",
    "document_type": "kyc",
    "file_path": "./data/docs/kyc_guide.pdf"
  }'
```

**Response:**
```json
{
  "document_id": "doc-xyz789-abc123",
  "status": "completed",
  "chunks": 15,
  "message": "Successfully ingested 15 chunks"
}
```

---

## 🔍 Search & Retrieval

### 1. Basic Search

```bash
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the KYC requirements for merchant onboarding?",
    "top_k": 5,
    "collection": "regulatory_docs"
  }'
```

### 2. Search with Specific Collection

```bash
# Search in regulatory documents
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the minimum net worth requirement?",
    "top_k": 3,
    "collection": "regulatory_docs"
  }'

# Search in merchant documents
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "merchant agreement terms",
    "top_k": 5,
    "collection": "merchant_docs"
  }'

# Search in KYC documents
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "PAN card verification process",
    "top_k": 3,
    "collection": "kyc_docs"
  }'
```

### 3. Search with Filters (Planned Feature)

```bash
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "settlement terms",
    "top_k": 5,
    "collection": "regulatory_docs",
    "filters": {
      "document_type": "regulatory"
    }
  }'
```

### 4. Get More Results

```bash
# Get top 10 results instead of default 5
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "risk assessment criteria",
    "top_k": 10,
    "collection": "regulatory_docs"
  }'
```

### 5. Pretty Print Results

```bash
# With jq (if installed)
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What documents are required?",
    "top_k": 3,
    "collection": "kyc_docs"
  }' | jq '.'

# Show only result texts
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "bank statements requirements",
    "top_k": 3,
    "collection": "regulatory_docs"
  }' | jq '.results[].text'

# Show only scores and sources
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "compliance requirements",
    "top_k": 5,
    "collection": "regulatory_docs"
  }' | jq '.results[] | {score: .score, source: .source}'
```

**Response:**
```json
{
  "query": "What are the KYC requirements?",
  "results": [
    {
      "id": "chunk-abc123",
      "score": 0.92,
      "text": "All merchants must submit: 1. PAN card...",
      "document_id": "doc-xyz789",
      "source": "RBI Guidelines 2023",
      "metadata": {
        "document_type": "regulatory",
        "position": 5
      }
    }
  ],
  "count": 5,
  "process_time_ms": 234
}
```

---

## 📋 Metadata Operations

### 1. List All Documents

```bash
curl http://localhost:8083/documents
```

### 2. List Documents by Type

```bash
# Get only regulatory documents
curl "http://localhost:8083/documents?type=regulatory"

# Get only merchant documents
curl "http://localhost:8083/documents?type=merchant"

# Get only KYC documents
curl "http://localhost:8083/documents?type=kyc"
```

### 3. List Documents by Status

```bash
# Get completed documents
curl "http://localhost:8083/documents?status=completed"

# Get processing documents
curl "http://localhost:8083/documents?status=processing"

# Get failed documents
curl "http://localhost:8083/documents?status=failed"
```

### 4. Get Specific Document

```bash
curl http://localhost:8083/documents/doc-abc123-xyz789
```

### 5. Update Document Status

```bash
curl -X PUT http://localhost:8083/documents/doc-abc123/status \
  -H "Content-Type: application/json" \
  -d '{
    "status": "completed"
  }'
```

### 6. Delete Document Metadata

```bash
curl -X DELETE http://localhost:8083/documents/doc-abc123
```

**Response:**
```json
{
  "documents": [
    {
      "id": "doc-abc123",
      "name": "RBI Guidelines 2023",
      "type": "regulatory",
      "file_path": "./data/docs/rbi_guidelines.pdf",
      "status": "completed",
      "uploaded_at": "2024-01-15T10:30:00Z"
    }
  ],
  "count": 1
}
```

---

## 🗄️ Vector Operations

### 1. List Collections

```bash
curl http://localhost:8082/collections
```

**Response:**
```json
{
  "collections": [
    "regulatory_docs",
    "merchant_docs",
    "kyc_docs"
  ]
}
```

### 2. Manual Vector Upsert (Advanced)

```bash
curl -X POST http://localhost:8082/upsert \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "regulatory_docs",
    "points": [
      {
        "id": "test-chunk-1",
        "vector": [0.1, 0.2, 0.3, 0.4, 0.5],
        "payload": {
          "text": "Sample text",
          "document_id": "test-doc",
          "position": 0
        }
      }
    ]
  }'
```

### 3. Manual Vector Search (Advanced)

```bash
curl -X POST http://localhost:8082/search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "regulatory_docs",
    "query": [0.1, 0.2, 0.3, 0.4, 0.5],
    "top_k": 5
  }'
```

---

## 🧮 Embedding Operations

### 1. Generate Single Embedding

```bash
curl -X POST http://localhost:8081/embed \
  -H "Content-Type: application/json" \
  -d '{
    "text": "What are the KYC requirements?"
  }'
```

**Response:**
```json
{
  "embedding": [0.234, 0.567, 0.891, ...],
  "dimension": 768
}
```

### 2. Generate Batch Embeddings

```bash
curl -X POST http://localhost:8081/embed-batch \
  -H "Content-Type: application/json" \
  -d '{
    "texts": [
      "First text to embed",
      "Second text to embed",
      "Third text to embed"
    ]
  }'
```

**Response:**
```json
{
  "embeddings": [
    [0.234, 0.567, ...],
    [0.891, 0.123, ...],
    [0.456, 0.789, ...]
  ],
  "count": 3,
  "dimension": 768
}
```

---

## 🔄 Complete Workflows

### Workflow 1: Upload and Search a Document

```bash
#!/bin/bash

# Step 1: Upload
UPLOAD_RESPONSE=$(curl -s -X POST http://localhost:8080/upload \
  -F "file=@my_document.pdf")

FILE_PATH=$(echo $UPLOAD_RESPONSE | jq -r '.file_path')
echo "Uploaded to: $FILE_PATH"

# Step 2: Ingest
INGEST_RESPONSE=$(curl -s -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d "{
    \"document_name\": \"My Document\",
    \"document_type\": \"regulatory\",
    \"file_path\": \"$FILE_PATH\"
  }")

DOC_ID=$(echo $INGEST_RESPONSE | jq -r '.document_id')
echo "Document ID: $DOC_ID"

# Step 3: Wait for processing
sleep 5

# Step 4: Search
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is this document about?",
    "top_k": 3,
    "collection": "regulatory_docs"
  }' | jq '.'
```

### Workflow 2: Batch Upload Multiple Documents

```bash
#!/bin/bash

# Array of files to upload
FILES=(
  "doc1.pdf:RBI Guidelines:regulatory"
  "doc2.pdf:MAS Circular:regulatory"
  "doc3.pdf:KYC Guide:kyc"
)

for FILE_INFO in "${FILES[@]}"; do
  IFS=':' read -r FILE NAME TYPE <<< "$FILE_INFO"
  
  echo "Processing: $FILE"
  
  # Upload
  UPLOAD=$(curl -s -X POST http://localhost:8080/upload -F "file=@$FILE")
  PATH=$(echo $UPLOAD | jq -r '.file_path')
  
  # Ingest
  curl -s -X POST http://localhost:8080/ingest \
    -H "Content-Type: application/json" \
    -d "{
      \"document_name\": \"$NAME\",
      \"document_type\": \"$TYPE\",
      \"file_path\": \"$PATH\"
    }" | jq '{document_id, status, chunks}'
  
  sleep 2
done
```

### Workflow 3: Search Across All Collections

```bash
#!/bin/bash

QUERY="merchant onboarding requirements"

echo "Searching across all collections for: $QUERY"
echo "================================================"

for COLLECTION in regulatory_docs merchant_docs kyc_docs; do
  echo ""
  echo "Collection: $COLLECTION"
  echo "------------------------"
  
  curl -s -X POST http://localhost:8084/retrieve \
    -H "Content-Type: application/json" \
    -d "{
      \"query\": \"$QUERY\",
      \"top_k\": 2,
      \"collection\": \"$COLLECTION\"
    }" | jq '.results[] | {score, source, text: .text[:100]}'
done
```

### Workflow 4: Create Test Document and Search

```bash
#!/bin/bash

# Create test document
cat > test_kyc.txt << 'EOF'
KYC Requirements for Merchants:
1. PAN Card - Mandatory for all merchants
2. GST Certificate - Required for businesses with turnover > 20 lakhs
3. Bank Statements - Last 6 months required
4. Address Proof - Utility bills or rental agreement
5. Business Registration - Certificate of Incorporation
EOF

# Upload
echo "Uploading test document..."
UPLOAD=$(curl -s -X POST http://localhost:8080/upload -F "file=@test_kyc.txt")
FILE_PATH=$(echo $UPLOAD | jq -r '.file_path')

# Ingest
echo "Ingesting document..."
curl -s -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d "{
    \"document_name\": \"Test KYC Guide\",
    \"document_type\": \"kyc\",
    \"file_path\": \"$FILE_PATH\"
  }" | jq '.'

# Wait
echo "Waiting 5 seconds for processing..."
sleep 5

# Search
echo "Testing search..."
curl -s -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What documents are required for KYC?",
    "top_k": 3,
    "collection": "kyc_docs"
  }' | jq '.results[] | {score, text}'
```

### Workflow 5: Check System Status

```bash
#!/bin/bash

echo "Checking RAG System Status"
echo "=========================="
echo ""

# Check services
SERVICES=(
  "Ingest:8080"
  "Embed:8081"
  "Vector:8082"
  "Metadata:8083"
  "Retrieval:8084"
)

for SERVICE in "${SERVICES[@]}"; do
  IFS=':' read -r NAME PORT <<< "$SERVICE"
  
  STATUS=$(curl -s http://localhost:$PORT/health | jq -r '.status')
  
  if [ "$STATUS" == "healthy" ]; then
    echo "✅ $NAME Service (port $PORT): $STATUS"
  else
    echo "❌ $NAME Service (port $PORT): NOT RESPONDING"
  fi
done

echo ""
echo "Document Statistics:"
echo "-------------------"

# Get document counts
TOTAL=$(curl -s http://localhost:8083/documents | jq '.count')
echo "Total documents: $TOTAL"

# Get collections
echo ""
echo "Vector Collections:"
curl -s http://localhost:8082/collections | jq '.collections[]'
```

---

## 🧪 Testing Commands

### Test 1: Basic Functionality

```bash
# 1. Health check
curl http://localhost:8080/health

# 2. Create simple test file
echo "Test document content" > test.txt

# 3. Upload
curl -X POST http://localhost:8080/upload -F "file=@test.txt"

# 4. Check documents
curl http://localhost:8083/documents
```

### Test 2: Search Quality

```bash
# Upload a known document and search for specific content
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "specific phrase from your document",
    "top_k": 1,
    "collection": "regulatory_docs"
  }' | jq '.results[0].score'

# Score should be > 0.8 for exact matches
```

### Test 3: Performance

```bash
# Measure retrieval time
time curl -s -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "test query",
    "top_k": 5,
    "collection": "regulatory_docs"
  }' > /dev/null

# Or check process_time_ms in response
curl -s -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "test query",
    "top_k": 5,
    "collection": "regulatory_docs"
  }' | jq '.process_time_ms'
```

---

## 🔧 Troubleshooting Commands

### Check Qdrant Connection

```bash
# Via vector service
curl http://localhost:8082/collections

# Direct to Qdrant
curl http://localhost:6333/collections
```

### Check Document in Database

```bash
# Get specific document details
curl http://localhost:8083/documents/YOUR_DOC_ID | jq '.'

# Check if ingestion completed
curl http://localhost:8083/documents | jq '.documents[] | select(.status=="completed")'
```

### Verify Embeddings Work

```bash
# Test single embedding
curl -X POST http://localhost:8081/embed \
  -H "Content-Type: application/json" \
  -d '{"text": "test"}' | jq '.dimension'

# Should return 768
```

### Check Vector Storage

```bash
# Search with dummy vector to see if data exists
curl -X POST http://localhost:8082/search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "regulatory_docs",
    "query": [0.1, 0.2, 0.3],
    "top_k": 1
  }' | jq '.count'

# Should return > 0 if documents exist
```

---

## 📊 Monitoring Commands

### Get System Statistics

```bash
# Document count by type
curl -s http://localhost:8083/documents | \
  jq '.documents | group_by(.type) | map({type: .[0].type, count: length})'

# Document count by status
curl -s http://localhost:8083/documents | \
  jq '.documents | group_by(.status) | map({status: .[0].status, count: length})'

# Recent documents (last 5)
curl -s http://localhost:8083/documents | \
  jq '.documents | sort_by(.uploaded_at) | reverse | .[0:5] | .[] | {name, uploaded_at, status}'
```

### Performance Monitoring

```bash
# Average retrieval time over 10 queries
for i in {1..10}; do
  curl -s -X POST http://localhost:8084/retrieve \
    -H "Content-Type: application/json" \
    -d '{"query": "test", "collection": "regulatory_docs"}' | \
    jq '.process_time_ms'
done | awk '{sum+=$1; count++} END {print "Average: " sum/count " ms"}'
```

---

## 🎯 Common Query Examples

### Finance/Compliance Queries

```bash
# Net worth requirements
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the minimum net worth requirement for payment aggregators?",
    "top_k": 3,
    "collection": "regulatory_docs"
  }'

# KYC documentation
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Which KYC documents are required for merchant onboarding?",
    "top_k": 5,
    "collection": "kyc_docs"
  }'

# Settlement terms
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What are the settlement cycle terms?",
    "top_k": 3,
    "collection": "regulatory_docs"
  }'

# Risk assessment
curl -X POST http://localhost:8084/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "How is merchant risk category determined?",
    "top_k": 5,
    "collection": "regulatory_docs"
  }'
```

---

## 💡 Pro Tips

### Save Common Queries

```bash
# Create a queries file
cat > common_queries.json << 'EOF'
[
  {
    "name": "KYC Requirements",
    "query": "What documents are required for KYC?",
    "collection": "kyc_docs"
  },
  {
    "name": "Net Worth",
    "query": "What is the minimum net worth requirement?",
    "collection": "regulatory_docs"
  }
]
EOF

# Run saved queries
cat common_queries.json | jq -c '.[]' | while read query; do
  NAME=$(echo $query | jq -r '.name')
  QUERY=$(echo $query | jq -r '.query')
  COLLECTION=$(echo $query | jq -r '.collection')
  
  echo "Running: $NAME"
  curl -s -X POST http://localhost:8084/retrieve \
    -H "Content-Type: application/json" \
    -d "{\"query\": \"$QUERY\", \"collection\": \"$COLLECTION\"}" | \
    jq '.results[0] | {score, source}'
done
```

### Create Aliases

```bash
# Add to ~/.bashrc or ~/.zshrc
alias rag-upload='curl -X POST http://localhost:8080/upload -F "file=@"'
alias rag-search='curl -X POST http://localhost:8084/retrieve -H "Content-Type: application/json" -d'
alias rag-docs='curl http://localhost:8083/documents | jq'
alias rag-health='curl http://localhost:8080/health && curl http://localhost:8081/health && curl http://localhost:8082/health'

# Usage
rag-upload mydoc.pdf
rag-search '{"query": "test", "collection": "regulatory_docs"}'
```

---

**All commands are copy-paste ready! Test your RAG system now! 🚀**
