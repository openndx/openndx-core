// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
)

// tokenKid names the signing key in the JWKS and in issued tokens' headers.
const tokenKid = "integration-test"

func main() {
	// The mock provider also stands in for the identity provider: the Orchestration
	// Engine verifies consumer tokens against /jwks, and tests get signed tokens
	// from /token. The key is generated at startup and never leaves the process.
	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("generate signing key: %v", err)
	}

	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"person": map[string]interface{}{
					"email":      "test@example.com",
					"fullName":   "John Doe",
					"address":    "123 Test St",
					"profession": "Tester",
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kid": tokenKid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(signingKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(signingKey.E)).Bytes()),
			}},
		})
	})

	// POST /token signs the JSON claims in the request body as an RS256 JWT.
	http.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		var claims map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&claims); err != nil {
			http.Error(w, "claims must be a JSON object", http.StatusBadRequest)
			return
		}
		token, err := signRS256(signingKey, claims)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"access_token": token})
	})

	log.Println("Mock Provider listening on :8083")
	if err := http.ListenAndServe(":8083", nil); err != nil {
		log.Fatal(err)
	}
}

// signRS256 returns claims as a JWT signed with key.
func signRS256(key *rsa.PrivateKey, claims map[string]interface{}) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": tokenKid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
