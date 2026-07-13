package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"strings"
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

func (s *Store) flushLoop() {
	ticker := time.NewTicker(10 * time.Second) // 10s for testing; 30s+ in real life
	for range ticker.C {
		s.flushClicks(context.Background())
	}
}

func (s *Store) flushClicks(ctx context.Context) {
	keys, err := s.cache.Keys(ctx, "clicks:*").Result()
	if err != nil {
		log.Printf("flush: listing keys failed: %v", err)
		return
	}

	for _, key := range keys {
		countStr, err := s.cache.GetDel(ctx, key).Result()
		if err != nil {
			log.Printf("flush: getdel %s failed: %v", key, err)
			continue
		}

		count, err := strconv.Atoi(countStr)
		if err != nil {
			log.Printf("flush: bad count %q for %s: %v", countStr, key, err)
			continue
		}

		code := strings.TrimPrefix(key, "clicks:")

		if _, err := s.db.Exec(
			"UPDATE urls SET clicks = clicks + $1 WHERE code = $2",
			count, code,
		); err != nil {
			log.Printf("flush: update %s failed: %v", code, err)
		}
	}
}
