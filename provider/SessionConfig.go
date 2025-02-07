package provider

import (
	"context"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-redis/redis"
)

type SessionConfig struct {
	ProjectID       string
	SessionSecret   string        // Secret key for signing cookies
	SessionTimeout  time.Duration // Session expiration duration
	UseFirestore    bool          // Whether to use Firestore for sessions
	UseRedis        bool          // Whether to use Redis for sessions
	FirestoreClient *firestore.Client
	RedisClient     *redis.Client
}

func NewSessionConfig(ctx context.Context) *SessionConfig {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {

	}
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		sessionSecret = "super-secret-key" // Replace with a secure key in production
	}

	sessionTimeout := 30 * time.Minute // Default session expiration

	// Initialize Firestore client if needed
	var firestoreClient *firestore.Client
	if os.Getenv("USE_FIRESTORE") == "true" {
		client, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			// log.Fatalf("Failed to initialize Firestore client: %v", err)
		}
		firestoreClient = client
	}

	// Initialize Redis client if needed
	var redisClient *redis.Client
	if os.Getenv("USE_REDIS") == "true" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDIS_ADDRESS"), // Redis endpoint
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
	}

	// Return the configuration object
	return &SessionConfig{
		ProjectID:       projectID,
		SessionSecret:   sessionSecret,
		SessionTimeout:  sessionTimeout,
		UseFirestore:    firestoreClient != nil,
		UseRedis:        redisClient != nil,
		FirestoreClient: firestoreClient,
		RedisClient:     redisClient,
	}
}
