// Package secrets loads a JSON blob from AWS Secrets Manager into the process env.
// Canonical SM fetch for kalke-auth; cmd/loadsecret wraps FetchMap for shell export.
package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const loadedEnv = "KALKE_SECRETS_LOADED"

// FetchMap returns key→string values from SECRET_ID (or secretID). Empty when unset.
func FetchMap(ctx context.Context, secretID string) (map[string]string, error) {
	sid := strings.TrimSpace(secretID)
	if sid == "" {
		sid = strings.TrimSpace(os.Getenv("SECRET_ID"))
	}
	if sid == "" {
		return nil, nil
	}
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(sid),
	})
	if err != nil {
		return nil, fmt.Errorf("get secret %s: %w", sid, err)
	}
	raw := ""
	if out.SecretString != nil {
		raw = *out.SecretString
	} else if len(out.SecretBinary) > 0 {
		raw = string(out.SecretBinary)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("secret json: %w", err)
	}
	result := make(map[string]string, len(data))
	for k, v := range data {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			result[k] = t
		case float64, bool:
			result[k] = fmt.Sprint(t)
		default:
			b, err := json.Marshal(t)
			if err != nil {
				continue
			}
			result[k] = string(b)
		}
	}
	return result, nil
}

// WithoutExisting returns a copy of data omitting KALKE_SECRETS_LOADED and any
// key that already has a non-empty value in the process environment.
func WithoutExisting(data map[string]string) map[string]string {
	out := make(map[string]string, len(data))
	for k, v := range data {
		if k == loadedEnv {
			continue
		}
		if cur, ok := os.LookupEnv(k); ok && cur != "" {
			continue
		}
		out[k] = v
	}
	return out
}

// ApplyMap sets missing env keys (does not overwrite non-empty values).
func ApplyMap(data map[string]string) {
	for k, v := range WithoutExisting(data) {
		_ = os.Setenv(k, v)
	}
}

// LoadIntoEnv fetches SECRET_ID and merges into the environment.
// No-op when SECRET_ID empty or KALKE_SECRETS_LOADED is set (entrypoint already loaded).
func LoadIntoEnv(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv(loadedEnv)) != "" {
		return nil
	}
	data, err := FetchMap(ctx, "")
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	ApplyMap(data)
	_ = os.Setenv(loadedEnv, "1")
	return nil
}

// MustLoad is LoadIntoEnv with a short timeout for process startup.
func MustLoad() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return LoadIntoEnv(ctx)
}

// MarkLoaded sets the sentinel so a later MustLoad does not re-fetch.
func MarkLoaded() {
	_ = os.Setenv(loadedEnv, "1")
}
