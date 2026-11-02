package auth

import (
	"context"
	"testing"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/authn/authntest"
	"github.com/stretchr/testify/assert"
)

func TestConsumerFromContext(t *testing.T) {
	t.Run("maps the authenticated principal", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), authn.AuthContextKey, &authn.AuthContext{
			Principal: authntest.MustPrincipal(map[string]any{"sub": "sub-1", "client_id": "app-1"}),
		})

		assert.Equal(t, &ConsumerAssertion{
			ApplicationID: "app-1",
			ClientID:      "app-1",
			Subscriber:    "sub-1",
		}, ConsumerFromContext(ctx))
	})

	t.Run("returns nil without an authenticated principal", func(t *testing.T) {
		assert.Nil(t, ConsumerFromContext(context.Background()))
	})
}
