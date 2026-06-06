package RateLimiter

import (
	"errors"
	"time"
)

type TokenBucket struct {
	apiKey          string
	capacity        int // Max tokens
	refillRate      int // Number of tokens added per second
	availableTokens int // Current Available tokens
	timestamp       int64
}

var tokenBucket []*TokenBucket

func NewTokenBucket(apiKey string, currentTimeInSecs int64) *TokenBucket {
	// Search for existing bucket using api key
	if apiKey == "" {
		return nil
	}

	for _, bucket := range tokenBucket {
		if bucket.apiKey == apiKey {
			return bucket
		}
	}

	tokenBucket = append(tokenBucket, &TokenBucket{
		apiKey:          apiKey,
		capacity:        10,
		refillRate:      1,
		availableTokens: 10,
		timestamp:       currentTimeInSecs,
	})
	return tokenBucket[len(tokenBucket)-1]
}

func (t *TokenBucket) AddTokens(tokensToAdd int) error {

	if tokensToAdd <= 0 {
		return errors.New("invalid tokens")
	}

	if tokensToAdd > t.capacity {
		tokensToAdd = t.capacity
	}
	// Check if availableTokens is zero and Reduce availableTokens by tokens

	t.availableTokens += tokensToAdd
	if t.availableTokens > t.capacity {
		t.availableTokens = t.capacity
	}
	t.timestamp = time.Now().Unix()
	return nil
}

func (t *TokenBucket) reduceAndValidateTokens(tokensToReduce int) error {
	if tokensToReduce <= 0 {
		return errors.New("invalid tokens")
	}
	if tokensToReduce > t.availableTokens {
		return errors.New("tokens to reduce exceeds available tokens")
	}
	t.availableTokens -= tokensToReduce
	return nil
}
func (l *Limiter) validate(apiKey string) (bool, error) {
	// If key is present return existing bucket, else new bucket
	currentTimeInSec := time.Now().Unix()
	bucket := NewTokenBucket(apiKey, currentTimeInSec)
	if bucket == nil {
		return false, errors.New("invalid api key")
	}
	//timeDiff := int(currentTimeInSec - bucket.timestamp)
	//tokenToAdd := timeDiff * bucket.refillRate
	//err := bucket.AddTokens(tokenToAdd)
	//if err != nil {
	//	return false, err
	//}
	errInReduce := bucket.reduceAndValidateTokens(1)
	if errInReduce != nil {
		return false, errInReduce
	}
	return true, nil
}
