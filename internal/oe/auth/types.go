package auth

// ConsumerAssertion represents the parsed and validated M2M token claims.
type ConsumerAssertion struct {
	ApplicationID string   // Mapped from validated 'client_id' (application identity)
	ClientID      string   // Mapped from 'client_id'
	Subscriber    string   // Mapped from 'sub' or 'azp'
	Iss           string   // Mapped from 'iss'
	Aud           []string // Mapped from 'aud'
	Exp           int64    // Mapped from 'exp'
	Iat           int64    // Mapped from 'iat'
}
