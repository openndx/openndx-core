package handlers

import (
	"net/http"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/models"
	"gorm.io/gorm"
)

// setupSQLiteTestDB creates an in-memory SQLite database with all Portal
// Backend models migrated.
func setupSQLiteTestDB(t *testing.T) *gorm.DB {
	return dbtest.SetupSQLiteDB(t,
		&models.Member{},
		&models.Application{},
		&models.ApplicationSubmission{},
		&models.Schema{},
		&models.SchemaSubmission{},
	)
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(req *http.Request) (*http.Response, error)

// RoundTrip calls f(req).
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
