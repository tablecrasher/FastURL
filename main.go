package main

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

type Store struct {
	mu   sync.RWMutex
	urls map[string]string // code -> long URL
}

type shortenRequest struct {
	URL string `json:"url"`
}

func generateCode(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base32.StdEncoding.EncodeToString(hash[:5])
	return encoded
}

func (s *Store) shortenHandler(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	code := generateCode(req.URL)

	s.mu.Lock()
	s.urls[code] = req.URL
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{
		"short_url": "http://localhost:8080/" + code,
	})
	if err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (s *Store) redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	s.mu.RLock()
	longURL, ok := s.urls[code]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, longURL, http.StatusFound)
}

func main() {
	store := &Store{
		urls: make(map[string]string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", store.shortenHandler)
	mux.HandleFunc("GET /{code}", store.redirectHandler)
	log.Fatal(http.ListenAndServe(":8080", mux))

}
