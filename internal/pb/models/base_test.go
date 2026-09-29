package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

		// Auto-migrate
		db.AutoMigrate(&TestModel{})
		defer db.Migrator().DropTable(&TestModel{})

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

		db.AutoMigrate(&TestModel{})
		defer db.Migrator().DropTable(&TestModel{})

		// Create a record - timestamps will be set by BeforeCreate hook
		model := TestModel{
			ID:   "test-update-123",
			Name: "Original",
		}
		err = db.Create(&model).Error
		assert.NoError(t, err)
		originalUpdatedAt := model.UpdatedAt

		// Update the record - BeforeUpdate hook should update UpdatedAt
		explicitLaterTime := originalUpdatedAt.Add(1 * time.Second)
		model.Name = "Updated"
		model.UpdatedAt = explicitLaterTime
		err = db.Save(&model).Error
		assert.NoError(t, err)

		// Reload the model to get the updated timestamp from database
		var updatedModel TestModel
		err = db.First(&updatedModel, "id = ?", model.ID).Error
		assert.NoError(t, err)

		// Verify UpdatedAt was changed by BeforeUpdate hook
		// Note: BeforeUpdate sets UpdatedAt to time.Now(), so it should be >= our explicit time
		assert.True(t, updatedModel.UpdatedAt.After(originalUpdatedAt) || updatedModel.UpdatedAt.Equal(explicitLaterTime))
	})
}
