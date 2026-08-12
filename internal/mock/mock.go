package mock

import (
	"encoding/json"
	"net/http"
	"sync"
)

type MockMode string

const (
	ModeNormal           MockMode = "NORMAL"
	ModeTypeBreak        MockMode = "TYPE_BREAK"    // id: 1024 -> id: "1024"
	ModeMissingField     MockMode = "MISSING_FIELD" // email removed
	ModeNullabilityBreak MockMode = "NULL_BREAK"    // role -> null
	ModeAddedField       MockMode = "ADDED_FIELD"   // new_flag: true
)

type MockController struct {
	mu           sync.RWMutex
	CurrentMode  MockMode `json:"current_mode"`
	RequestCount int64    `json:"request_count"`
}

func NewMockController() *MockController {
	return &MockController{
		CurrentMode: ModeNormal,
	}
}

func (m *MockController) SetMode(mode MockMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentMode = mode
}

func (m *MockController) GetMode() MockMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.CurrentMode
}

func (m *MockController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.RequestCount++
	mode := m.CurrentMode
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/_driftwood/mock/users", "/api/users":
		m.serveUsersPayload(w, mode)
	case "/_driftwood/mock/products", "/api/products":
		m.serveProductsPayload(w, mode)
	default:
		m.serveUsersPayload(w, mode)
	}
}

func (m *MockController) serveUsersPayload(w http.ResponseWriter, mode MockMode) {
	var payload map[string]interface{}

	switch mode {
	case ModeTypeBreak:
		payload = map[string]interface{}{
			"id":        "usr_99812", // BREAKING: Expected integer (99812), now string!
			"username":  "alex_dev",
			"email":     "alex@company.com",
			"score":     98.5,
			"is_active": true,
			"roles":     []string{"admin", "developer"},
		}
	case ModeMissingField:
		payload = map[string]interface{}{
			"id": 99812,
			// BREAKING: "email" field missing!
			"username":  "alex_dev",
			"score":     98.5,
			"is_active": true,
			"roles":     []string{"admin", "developer"},
		}
	case ModeNullabilityBreak:
		payload = map[string]interface{}{
			"id":        99812,
			"username":  "alex_dev",
			"email":     nil, // BREAKING: email expected string, now null!
			"score":     98.5,
			"is_active": true,
			"roles":     []string{"admin", "developer"},
		}
	case ModeAddedField:
		payload = map[string]interface{}{
			"id":         99812,
			"username":   "alex_dev",
			"email":      "alex@company.com",
			"score":      98.5,
			"is_active":  true,
			"roles":      []string{"admin", "developer"},
			"new_feature": "beta_v2_enabled", // WARNING/INFO: new field added
		}
	case ModeNormal:
		fallthrough
	default:
		payload = map[string]interface{}{
			"id":        99812,
			"username":  "alex_dev",
			"email":     "alex@company.com",
			"score":     98.5,
			"is_active": true,
			"roles":     []string{"admin", "developer"},
		}
	}

	_ = json.NewEncoder(w).Encode(payload)
}

func (m *MockController) serveProductsPayload(w http.ResponseWriter, mode MockMode) {
	payload := map[string]interface{}{
		"status": "success",
		"items": []map[string]interface{}{
			{
				"sku":   "PRD-101",
				"name":  "Developer Mechanical Keyboard",
				"price": 149.99,
				"stock": 42,
			},
		},
	}
	_ = json.NewEncoder(w).Encode(payload)
}
