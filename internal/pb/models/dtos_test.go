package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemberResponse_ToMember(t *testing.T) {
	t.Run("ToMember_ConvertsCorrectly", func(t *testing.T) {
		response := MemberResponse{
			MemberID:    "mem_123",
			Name:        "Test User",
			Email:       "test@example.com",
			PhoneNumber: "1234567890",
			IdpUserID:   "idp-user-123",
		}

		member := response.ToMember()

		assert.Equal(t, response.MemberID, member.MemberID)
		assert.Equal(t, response.Name, member.Name)
		assert.Equal(t, response.Email, member.Email)
		assert.Equal(t, response.PhoneNumber, member.PhoneNumber)
		assert.Equal(t, response.IdpUserID, member.IdpUserID)
	})
}
