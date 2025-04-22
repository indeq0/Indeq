package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var rateLimiter *RateLimiterService

func init() {
	rateLimiter = NewRateLimiterService()
}

// Rate limit Config
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
}

// RateLimiterService manages both project-wide and per-user rate limits
type RateLimiterService struct {
	// User-specific limiters
	userLimiters sync.Map

	// Project-wide limiters (one per service)
	projectLimiters map[string]*rate.Limiter
	projectMutex    sync.RWMutex

	// Configurations
	userConfigs    map[string]RateLimitConfig
	projectConfigs map[string]RateLimitConfig
}

// NewRateLimiterService creates a thread-safe rate limiter service
func NewRateLimiterService() *RateLimiterService {
	// User-specific rate limits
	userConfigs := map[string]RateLimitConfig{
		"GOOGLE_DOCS":   {50, 100},  // Increased from 50 to 100 req/s per user
		"GOOGLE_SLIDES": {10, 20},   // Increased from 10 to 20 req/s per user
		"GOOGLE_GMAIL":  {250, 500}, // Increased from 250 to 500 req/s per user
		"GOOGLE_DRIVE":  {100, 200}, // Increased from 100 to 200 req/s per user
		"NOTION":        {3, 3},     // Increased from 3 to 3 req/s per user
	}

	// Project-wide rate limits
	projectConfigs := map[string]RateLimitConfig{
		"GOOGLE_DOCS":   {50, 100},      // Increased from 50 to 100 req/s
		"GOOGLE_SLIDES": {50, 100},      // Increased from 50 to 100 req/s
		"GOOGLE_DRIVE":  {200, 400},     // Increased from 200 to 400 req/s
		"GOOGLE_GMAIL":  {20000, 25000}, // Increased from 20000 to 25000 req/s
		"NOTION":        {3, 3},         // Increased from 3 to 3 req/s
	}

	projectLimiters := make(map[string]*rate.Limiter)
	for service, config := range projectConfigs {
		projectLimiters[service] = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.Burst)
	}

	return &RateLimiterService{
		userConfigs:     userConfigs,
		projectConfigs:  projectConfigs,
		projectLimiters: projectLimiters,
	}
}

// GetUserLimiter returns a rate limiter for a specific user and service
func (s *RateLimiterService) GetUserLimiter(service, userID string) *rate.Limiter {
	key := fmt.Sprintf("%s:%s", service, userID)

	if limiter, exists := s.userLimiters.Load(key); exists {
		return limiter.(*rate.Limiter)
	}

	config, exists := s.userConfigs[service]
	if !exists {
		config = RateLimitConfig{2, 2}
	}

	limiter := rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.Burst)

	actualLimiter, _ := s.userLimiters.LoadOrStore(key, limiter)
	return actualLimiter.(*rate.Limiter)
}

// GetProjectLimiter returns the project-wide limiter for a service
func (s *RateLimiterService) GetProjectLimiter(service string) *rate.Limiter {
	s.projectMutex.RLock()
	limiter, exists := s.projectLimiters[service]
	s.projectMutex.RUnlock()

	if exists {
		return limiter
	}

	s.projectMutex.Lock()
	defer s.projectMutex.Unlock()

	if limiter, exists = s.projectLimiters[service]; exists {
		return limiter
	}

	config, exists := s.projectConfigs[service]
	if !exists {
		config = RateLimitConfig{20, 20}
	}

	limiter = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.Burst)
	s.projectLimiters[service] = limiter
	return limiter
}

// Wait respects both project-wide and per-user rate limits
func (s *RateLimiterService) Wait(ctx context.Context, service, userID string) error {
	maxRetries := 3
	backoff := 1 * time.Second

	for retry := 0; retry < maxRetries; retry++ {
		projectLimiter := s.GetProjectLimiter(service)
		if err := projectLimiter.Wait(ctx); err != nil {
			if retry < maxRetries-1 {
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
			return fmt.Errorf("project-wide rate limit exceeded for %s: %w", service, err)
		}

		userLimiter := s.GetUserLimiter(service, userID)
		if err := userLimiter.Wait(ctx); err != nil {
			if retry < maxRetries-1 {
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
			return fmt.Errorf("user rate limit exceeded for %s:%s: %w", service, userID, err)
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded for rate limiting")
}
