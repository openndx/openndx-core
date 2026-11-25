// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package auth

import (
	"context"

	"github.com/openndx/openndx-core/internal/authn"
)

// ConsumerAssertion identifies the data consumer that sent a query.
type ConsumerAssertion struct {
	// ApplicationID is the consumer application the PDP authorizes. It is the
	// token's client_id: the PDP allow-list is keyed by client_id (see issue #447).
	ApplicationID string
	ClientID      string // Mapped from 'client_id'
	Subscriber    string // Mapped from 'sub'
}

// ConsumerFromContext returns the consumer authenticated by internal/authn's
// middleware, or nil when the request was not authenticated.
func ConsumerFromContext(ctx context.Context) *ConsumerAssertion {
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil {
		return nil
	}
	return &ConsumerAssertion{
		ApplicationID: authCtx.ClientID(),
		ClientID:      authCtx.ClientID(),
		Subscriber:    authCtx.Subject(),
	}
}
