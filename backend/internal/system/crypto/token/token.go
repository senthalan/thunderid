/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package token provides utilities for generating and validating secure tokens.
package token

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"

	"github.com/senthalan/thunder/backend/internal/system/crypto/hash"
)

// GenerateSecureToken generates a cryptographically random 32-byte token, hex-encoded (64 chars).
func GenerateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of the given token.
// The token itself carries 256 bits of entropy, so no salt is needed.
func HashToken(rawToken string) string {
	h, _ := hash.Hash([]byte(rawToken), hash.GenericSHA256)
	return hex.EncodeToString(h)
}

// ValidateTokenHash checks whether rawToken hashes to storedHash using constant-time comparison.
func ValidateTokenHash(rawToken, storedHash string) bool {
	expected := HashToken(rawToken)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(storedHash)) == 1
}
