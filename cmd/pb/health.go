package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/openndx/openndx-core/internal/utils"
	"gorm.io/gorm"
)

// debugHandler reports the build PB is running.
func debugHandler(w http.ResponseWriter, r *http.Request) {
	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"service":    "portal-backend",
		"version":    Version,
		"build_time": BuildTime,
		"git_commit": GitCommit,
		"path":       r.URL.Path,
		"method":     r.Method,
	})
}

// debugDBHandler reports the database connection and the members table.
func debugDBHandler(gormDB *gorm.DB, dbName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		debugInfo := map[string]interface{}{
			"v1": map[string]interface{}{},
		}

		// Test V1 GORM database connection
		if gormDB == nil {
			debugInfo["v1"] = map[string]interface{}{
				"error": "GORM connection is nil",
			}
		} else {
			sqlDB, err := gormDB.DB()
			if err != nil {
				debugInfo["v1"] = map[string]interface{}{
					"error": fmt.Sprintf("failed to get sql.DB: %v", err),
				}
			} else if err := sqlDB.PingContext(ctx); err != nil {
				debugInfo["v1"] = map[string]interface{}{
					"error": fmt.Sprintf("V1 database ping failed: %v", err),
				}
			} else {
				v1Info := map[string]interface{}{
					"status":   "connected",
					"database": dbName,
				}

				// Check if members table exists in V1 DB
				var membersExists bool
				checkMembersQuery := `SELECT EXISTS (
                       SELECT FROM information_schema.tables
                       WHERE table_schema = 'public'
                       AND table_name = 'members'
                   )`

				if err := sqlDB.QueryRowContext(ctx, checkMembersQuery).Scan(&membersExists); err != nil {
					v1Info["table_check_error"] = fmt.Sprintf("failed to check members table: %v", err)
				} else {
					v1Info["members_table_exists"] = membersExists
					if membersExists {
						var memberCount int
						countMembersQuery := `SELECT COUNT(*) FROM members`
						if err := sqlDB.QueryRowContext(ctx, countMembersQuery).Scan(&memberCount); err != nil {
							v1Info["count_error"] = fmt.Sprintf("failed to count members: %v", err)
						} else {
							v1Info["members_count"] = memberCount
						}
					}
				}
				debugInfo["v1"] = v1Info
			}
		}

		utils.RespondWithJSON(w, http.StatusOK, debugInfo)
	}
}
