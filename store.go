package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	db    *sql.DB
	cache *redis.Client
}

func (s *Store) Save(code, longURL string) error {
	_, err := s.db.Exec(
		"INSERT INTO urls (code, long_url) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING",
		code, longURL,
	)
	return err
}

func (s *Store) Get(ctx context.Context, code string) (string, error) {
	longURL, err := s.cache.Get(ctx, code).Result()

	if err == nil {
		return longURL, nil
	}

	if !errors.Is(err, redis.Nil) {
		log.Printf("redis get failed: %v", err)
	}

	err = s.db.QueryRow(
		"SELECT long_url FROM urls WHERE code = $1", code,
	).Scan(&longURL)
	if err != nil {
		return "", err
	}

	if err := s.cache.Set(ctx, code, longURL, time.Hour).Err(); err != nil {
		log.Printf("cache set failed: %v", err)
	}

	return longURL, nil
}
