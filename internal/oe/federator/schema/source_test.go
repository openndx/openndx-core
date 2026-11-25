// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Run("loads the schema file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "consumer.graphql")
		require.NoError(t, os.WriteFile(path, []byte(`type Query { fromFile: String }`), 0o600))

		src, err := Load(path)
		require.NoError(t, err)

		compiled, err := src.Active(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, compiled.Schema.Query.Fields.ForName("fromFile"))
	})

	t.Run("serves the same schema for every request", func(t *testing.T) {
		src, err := FromSDL(`type Query { a: String }`)
		require.NoError(t, err)

		first, err := src.Active(context.Background())
		require.NoError(t, err)
		second, err := src.Active(context.Background())
		require.NoError(t, err)
		assert.Same(t, first, second)
	})

	t.Run("fails when the file is missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.graphql")

		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read schema file")
		assert.Contains(t, err.Error(), path)
	})

	t.Run("fails when the file doesn't parse", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.graphql")
		require.NoError(t, os.WriteFile(path, []byte(`type Query {`), 0o600))

		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compile schema file "+path)
	})

	t.Run("fails when the schema is invalid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.graphql")
		require.NoError(t, os.WriteFile(path, []byte(`type Query { person: Person }`), 0o600))

		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compile schema file "+path)
		assert.Contains(t, err.Error(), "Person")
	})

	t.Run("loads the repository's sample schemas", func(t *testing.T) {
		for _, path := range []string{"../../../../cmd/oe/schema.graphql", "../../../../tests/integration/schema.graphql"} {
			_, err := Load(path)
			require.NoError(t, err, path)
		}
	})
}
