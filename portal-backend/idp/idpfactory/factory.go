package idpfactory

import (
	"context"
	"errors"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
	"github.com/OpenNDX/openndx-core/portal-backend/idp/asgardeo"
)

type FactoryConfig struct {
	ProviderType idp.ProviderType
	BaseURL      string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// asgardeoAdapter adapts *asgardeo.Client to match the idp.IdentityProviderAPI
// method signatures (specifically AddMemberToGroup which expects a string (non-pointer), but the underlying asgardeo.Client expects a *string (pointer), so the adapter converts from string to *string).
type asgardeoAdapter struct {
	*asgardeo.Client
}

func (a *asgardeoAdapter) AddMemberToGroup(ctx context.Context, groupID string, member *idp.GroupMember) error {
	// forward to the underlying client which expects *string for groupID
	return a.Client.AddMemberToGroup(ctx, &groupID, member)
}

func NewIdpAPIProvider(cfg FactoryConfig) (idp.IdentityProviderAPI, error) {
	switch cfg.ProviderType {
	case idp.ProviderAsgardeo:
		return &asgardeoAdapter{asgardeo.NewClient(cfg.BaseURL, cfg.ClientID, cfg.ClientSecret, cfg.Scopes)}, nil
	default:
		return nil, errors.New("unsupported provider type")
	}
}
