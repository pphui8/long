package auth

import (
	"testing"
)

func TestJWT(t *testing.T) {
	username := "testuser"

	// Test Access Token
	accessToken, err := GenerateAccessToken(username)
	if err != nil {
		t.Fatalf("Failed to generate access token: %v", err)
	}

	claims, err := ValidateToken(accessToken, AccessAudience)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}

	if claims.Username != username {
		t.Errorf("Expected username %s, got %s", username, claims.Username)
	}

	// Test Refresh Token
	refreshToken, jti, err := GenerateRefreshToken(username)
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	if jti == "" {
		t.Error("Expected JTI to be non-empty")
	}

	refreshClaims, err := ValidateToken(refreshToken, RefreshAudience)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}

	if refreshClaims.Username != username {
		t.Errorf("Expected username %s, got %s", username, refreshClaims.Username)
	}

	if refreshClaims.ID != jti {
		t.Errorf("Expected JTI %s, got %s", jti, refreshClaims.ID)
	}

	// Test Invalid Audience
	_, err = ValidateToken(accessToken, RefreshAudience)
	if err == nil {
		t.Error("Expected error when validating access token with refresh audience, got nil")
	}
}

func TestTokenStore(t *testing.T) {
	username := "testuser"
	jti := "test-jti"

	GlobalTokenStore.RegisterToken(username, jti)
	if GlobalTokenStore.GetActiveToken(username) != jti {
		t.Error("Failed to register/get active token")
	}

	if GlobalTokenStore.IsRevoked(jti) {
		t.Error("Token should not be revoked yet")
	}

	GlobalTokenStore.RevokeToken(jti)
	if !GlobalTokenStore.IsRevoked(jti) {
		t.Error("Failed to revoke token")
	}

	GlobalTokenStore.InvalidateUserSessions(username)
	if GlobalTokenStore.GetActiveToken(username) != "" {
		t.Error("Failed to invalidate user sessions")
	}
}
