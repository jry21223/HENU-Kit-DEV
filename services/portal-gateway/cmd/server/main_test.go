package main

import (
	"strings"
	"testing"
)

func TestValidateFoodPostDeploymentSecretsAllowsLoopbackExample(t *testing.T) {
	env := map[string]string{
		"FOOD_POSTS_URL":          "http://food:8096",
		"PORTAL_ORIGIN":           "http://localhost:8088",
		"FOOD_POST_CREATE_SECRET": "replace-food-post-create-secret-32bytes!!",
		"FOOD_POST_READ_SECRET":   "replace-food-post-read-secret-32bytes!!",
	}
	if err := validateFoodPostDeploymentSecrets(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("loopback validation returned error: %v", err)
	}
}

func TestValidateFoodPostDeploymentSecretsRejectsPlaceholderInDeployment(t *testing.T) {
	env := map[string]string{
		"FOOD_POSTS_URL":          "http://food:8096",
		"PORTAL_ORIGIN":           "https://henukit.cn",
		"FOOD_POST_CREATE_SECRET": "replace-food-post-create-secret-32bytes!!",
		"FOOD_POST_READ_SECRET":   "food-read-production-secret-with-entropy-123",
	}
	err := validateFoodPostDeploymentSecrets(func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "FOOD_POST_CREATE_SECRET") {
		t.Fatalf("validation error = %v, want create-secret rejection", err)
	}
}

func TestValidateFoodPostDeploymentSecretsAcceptsExplicitDeploymentSecrets(t *testing.T) {
	env := map[string]string{
		"FOOD_POSTS_URL":          "http://food:8096",
		"PORTAL_ORIGIN":           "https://henukit.cn",
		"FOOD_POST_CREATE_SECRET": "food-create-production-secret-with-entropy-123",
		"FOOD_POST_READ_SECRET":   "food-read-production-secret-with-entropy-4567",
	}
	if err := validateFoodPostDeploymentSecrets(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("deployment validation returned error: %v", err)
	}
}
