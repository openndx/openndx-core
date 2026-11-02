// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"fmt"

	"github.com/OpenNSW/core/shared/validation"
)

// Config carries yaml struct tags, so it can be embedded in a larger
// application config struct and populated generically (e.g. via
// yaml.Unmarshal).
type Config struct {
	JWKSURL               string `yaml:"jwksURL"`
	Issuer                string `yaml:"issuer"`
	Audience              string `yaml:"audience"`
	InsecureSkipTLSVerify bool   `yaml:"insecureSkipTLSVerify"`

	// Claims configures the standard identity claims. See WithClaims. Zero
	// value = every claim optional.
	Claims ClaimsConfig `yaml:"claims"`
}

// ClaimsConfig configures each standard identity claim — "email", "ouId" and
// "ouHandle" — available through Principal accessors and Claims().
//
// ClaimsConfig carries yaml struct tags, so it can be populated generically
// (e.g. via yaml.Unmarshal) as part of Config.
type ClaimsConfig struct {
	Email    ClaimConfig `yaml:"email"`
	OUID     ClaimConfig `yaml:"ouId"`
	OUHandle ClaimConfig `yaml:"ouHandle"`
}

// ClaimConfig configures one standard identity claim.
type ClaimConfig struct {
	// Required rejects a token whose claim is absent or blank. The
	// requirement applies to every token, including client-credentials
	// tokens issued to systems, which typically carry no email or
	// organization unit. Default false: the string accessor returns "" instead.
	Required bool `yaml:"required"`
}

func (c Config) Validate() error {
	if c.JWKSURL == "" {
		return fmt.Errorf("jwksURL is required")
	}
	if err := validation.HTTPURL("jwksURL", c.JWKSURL); err != nil {
		return err
	}
	if c.Issuer == "" {
		return fmt.Errorf("issuer is required")
	}
	if err := validation.HTTPURL("issuer", c.Issuer); err != nil {
		return err
	}
	if c.Audience == "" {
		return fmt.Errorf("audience is required")
	}
	return nil
}
