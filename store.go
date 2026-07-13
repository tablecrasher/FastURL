package main

import "database/sql"

type Store struct {
	db *sql.DB
}

func (s *Store) Save(code, longURL string) error {
	_, err := s.db.Exec(
		"INSERT INTO urls (code, long_url) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING",
		code, longURL,
	)
	return err
}

func (s *Store) Get(code string) (string, error) {
	var longURL string
	err := s.db.QueryRow(
		"SELECT long_url FROM urls WHERE code = $1", code,
	).Scan(&longURL)
	return longURL, err
}
