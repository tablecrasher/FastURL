package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type shortenRequest struct {
	URL string `json:"url"`
}

func generateCode(url string) string {
	hash := sha256.Sum256([]byte(url))
	return base32.StdEncoding.EncodeToString(hash[:5])
}

func (s *Store) shortenHandler(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	code := generateCode(req.URL)

	if err := s.Save(code, req.URL); err != nil {
		log.Printf("save failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"short_url": "http://localhost:8080/" + code,
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (s *Store) redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	longURL, err := s.Get(code)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("lookup failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, longURL, http.StatusFound)
}
