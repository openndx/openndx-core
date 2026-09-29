package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBaseModel_BeforeCreate(t *testing.T) {
	t.Run("BeforeCreate_SetsTimestamps", func(t *testing.T) {
		// Use a test database connection
		db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
		if err != nil {
			t.Skipf("Skipping test: could not connect to test database: %v", err)
			return
		}

		// Create a test model that embeds BaseModel
		type TestModel struct {
			ID string `gorm:"primarykey"`
			BaseModel
			Name string
		}

		require.NoError(t, db.AutoMigrate(&TestModel{}))
		t.Cleanup(func() {
			require.NoError(t, db.Migrator().DropTable(&TestModel{}))
		})

		// Create a record
		model := TestModel{
			ID:   "test-create-123",
			Name: "Test",
		}

		err = db.Create(&model).Error
		assert.NoError(t, err)

		// Verify timestamps were set
		assert.False(t, model.CreatedAt.IsZero())
		assert.False(t, model.UpdatedAt.IsZero())
		assert.WithinDuration(t, time.Now(), model.CreatedAt, 5*time.Second)
		assert.WithinDuration(t, time.Now(), model.UpdatedAt, 5*time.Second)
	})
}

func TestBaseModel_BeforeUpdate(t *testing.T) {
	t.Run("BeforeUpdate_UpdatesTimestamp", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
		if err != nil {
			t.Skipf("Skipping test: could not connect to test database: %v", err)
			return
		}

		type TestModel struct {
			ID string `gorm:"primarykey"`
			BaseModel
			Name string
		}

		require.NoError(t, db.AutoMigrate(&TestModel{}))
		t.Cleanup(func() {
			require.NoError(t, db.Migrator().DropTable(&TestModel{}))
		})

		// Create a record - timestamps will be set by BeforeCreate hook
		model := TestModel{
			ID:   "test-update-123",
			Name: "Original",
		}
		err = db.Create(&model).Error
		assert.NoError(t, err)

		// Plant a stale UpdatedAt. BeforeUpdate must overwrite it with time.Now();
		// if the hook does not run, this ancient value would persist and fail below.
		model.Name = "Updated"
		model.UpdatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		err = db.Save(&model).Error
		assert.NoError(t, err)

		// Reload the model to get the updated timestamp from database
		var updatedModel TestModel
		err = db.First(&updatedModel, "id = ?", model.ID).Error
		assert.NoError(t, err)

		assert.WithinDuration(t, time.Now(), updatedModel.UpdatedAt, 5*time.Second)
		assert.True(t, updatedModel.UpdatedAt.After(time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)),
			"planted year-2000 UpdatedAt must be overwritten by BeforeUpdate")
	})
}
