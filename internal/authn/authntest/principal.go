// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package authntest builds authenticated fixtures through the real token parser.
// Import it only from tests and test helper packages.
package authntest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openndx/openndx-core/internal/authn"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var fixture = sync.OnceValue(func() func(map[string]any) authn.Principal {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	jwks, err := json.Marshal(map[string]any{"keys": []map[string]any{{
		"kid": "test", "kty": "RSA", "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}})
	if err != nil {
		panic(err)
	}
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(jwks))),
			Header:     make(http.Header),
		}, nil
	})}
	extractor, err := authn.NewTokenExtractorWithClient("https://authn.test/jwks", "https://authn.test", "test", client)
	if err != nil {
		panic(err)
	}
	return func(values map[string]any) authn.Principal {
		claims := jwt.MapClaims{
			"iss": "https://authn.test", "aud": "test",
			"exp": time.Now().Add(time.Hour).Unix(),
			"sub": "test-subject", "client_id": "test-client",
		}
		for name, value := range values {
			claims[name] = value
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = "test"
		signed, err := token.SignedString(key)
		if err != nil {
			panic(err)
		}
		principal, err := extractor.ExtractPrincipalFromHeader("Bearer " + signed)
		if err != nil {
			panic(err)
		}
		return *principal
	}
})

// MustPrincipal signs and validates a test token, overriding default test claims
// with values. It panics if a fixture is invalid. JWKS is served in memory and no
// network requests are made; production validation is never bypassed.
func MustPrincipal(values map[string]any) authn.Principal { return fixture()(values) }
