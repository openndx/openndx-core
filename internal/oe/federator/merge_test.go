// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"fmt"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The merge tests pass fetch results in directly: merge depends only on the
// operation, the plan and the results, not on how the results were fetched.

// results returns one successful result per fetch, keyed by schema ID.
func results(p *plan, data map[string]map[string]interface{}) map[*fetch]fetchResult {
	out := map[*fetch]fetchResult{}
	for _, f := range p.fetches {
		out[f] = fetchResult{response: graphql.Response{Data: data[f.schemaID]}}
	}
	return out
}

func providerErr(message string, path ...string) map[string]interface{} {
	err := map[string]interface{}{"message": message, "extensions": map[string]interface{}{"code": errors.CodeProviderError}}
	if len(path) > 0 {
		segments := make([]interface{}, len(path))
		for i, s := range path {
			segments[i] = s
		}
		err["path"] = segments
	}
	return err
}

func TestMerge(t *testing.T) {
	identityRegistryData := map[string]interface{}{"person": map[string]interface{}{"fullName": "Jane", "tags": []interface{}{"a", "b"}}}
	vehicleRegistryData := map[string]interface{}{"vehicle": map[string]interface{}{"list": map[string]interface{}{"data": []interface{}{
		map[string]interface{}{"registrationNumber": "CAB-1", "make": "Toyota", "owner": map[string]interface{}{"name": "Jane"}},
		map[string]interface{}{"registrationNumber": "CAB-2", "make": "Honda"},
	}}}}

	t.Run("places values by response key, including list items", func(t *testing.T) {
		op, p := planFor(t, `{ who: personInfo(nic: "1") { name: fullName tags cars: ownedVehicles { reg: regNo make owner { name } } untracked __typename } }`, nil)

		resp := merge(op, p, results(p, map[string]map[string]interface{}{"identity-registry-v1": identityRegistryData, "vehicle-registry-v1": vehicleRegistryData}))

		assert.Empty(t, resp.Errors)
		assert.Equal(t, map[string]interface{}{"who": map[string]interface{}{
			"name": "Jane",
			"tags": []interface{}{"a", "b"},
			"cars": []interface{}{
				map[string]interface{}{"reg": "CAB-1", "make": "Toyota", "owner": map[string]interface{}{"name": "Jane"}},
				map[string]interface{}{"reg": "CAB-2", "make": "Honda", "owner": map[string]interface{}{"name": nil}},
			},
			"untracked":  nil,
			"__typename": "PersonInfo",
		}}, resp.Data)
	})

	t.Run("reports each field of a failed fetch as null with an error", func(t *testing.T) {
		op, p := planFor(t, `{ personInfo(nic: "1") { fullName ownedVehicles { regNo } } }`, nil)
		res := results(p, map[string]map[string]interface{}{"identity-registry-v1": identityRegistryData})
		for f := range res {
			if f.schemaID == "vehicle-registry-v1" {
				res[f] = fetchResult{err: fmt.Errorf("connection refused")}
			}
		}

		resp := merge(op, p, res)

		assert.Equal(t, map[string]interface{}{"personInfo": map[string]interface{}{"fullName": "Jane", "ownedVehicles": nil}}, resp.Data)
		assert.Equal(t, []interface{}{providerErr("Failed to fetch data from provider for schema vehicle-registry-v1", "personInfo", "ownedVehicles")}, resp.Errors)
	})

	t.Run("propagates a null in a non-null field to the nearest nullable parent", func(t *testing.T) {
		op, p := planFor(t, `{ personInfo(nic: "1") { fullName ownedVehicles { regNo } } }`, nil)
		missingRegNo := map[string]interface{}{"vehicle": map[string]interface{}{"list": map[string]interface{}{"data": []interface{}{
			map[string]interface{}{"registrationNumber": "CAB-1"},
			map[string]interface{}{"make": "Honda"},
		}}}}

		resp := merge(op, p, results(p, map[string]map[string]interface{}{"identity-registry-v1": identityRegistryData, "vehicle-registry-v1": missingRegNo}))

		// regNo is String! and the items are Vehicle!, so the null reaches the nullable list.
		assert.Equal(t, map[string]interface{}{"personInfo": map[string]interface{}{"fullName": "Jane", "ownedVehicles": nil}}, resp.Data)
		assert.Equal(t, []interface{}{map[string]interface{}{
			"message": "Cannot return null for non-nullable field Vehicle.regNo.",
			"path":    []any{"personInfo", "ownedVehicles", 1, "regNo"},
		}}, resp.Errors)
	})

	t.Run("nulls data when a non-null root field is null", func(t *testing.T) {
		op, p := planFor(t, `{ required(nic: "1") { value optional } }`, nil)

		resp := merge(op, p, results(p, map[string]map[string]interface{}{"identity-registry-v1": {"required": map[string]interface{}{"optional": "x"}}}))

		assert.Nil(t, resp.Data)
		assert.Equal(t, []interface{}{map[string]interface{}{
			"message": "Cannot return null for non-nullable field Required.value.",
			"path":    []any{"required", "value"},
		}}, resp.Errors)
	})

	t.Run("reports a provider value that should be a list", func(t *testing.T) {
		op, p := planFor(t, `{ personInfo(nic: "1") { ownedVehicles { regNo } } }`, nil)
		notAList := map[string]interface{}{"vehicle": map[string]interface{}{"list": map[string]interface{}{"data": "oops"}}}

		resp := merge(op, p, results(p, map[string]map[string]interface{}{"vehicle-registry-v1": notAList}))

		assert.Equal(t, map[string]interface{}{"personInfo": map[string]interface{}{"ownedVehicles": nil}}, resp.Data)
		assert.Equal(t, []interface{}{providerErr("Expected a list from the provider, got string", "personInfo", "ownedVehicles")}, resp.Errors)
	})

	t.Run("passes on provider errors at the overlapping consumer field", func(t *testing.T) {
		op, p := planFor(t, `{ personInfo(nic: "1") { fullName tags } }`, nil)
		res := results(p, nil)
		for f := range res {
			res[f] = fetchResult{response: graphql.Response{
				Data: map[string]interface{}{"person": map[string]interface{}{"fullName": "Jane", "tags": nil}},
				Errors: []interface{}{
					map[string]interface{}{"message": "tags unavailable", "path": []interface{}{"person", "tags"}},
					map[string]interface{}{"message": "rate limited"},
				},
			}}
		}

		resp := merge(op, p, res)

		assert.Equal(t, map[string]interface{}{"personInfo": map[string]interface{}{"fullName": "Jane", "tags": nil}}, resp.Data)
		assert.Equal(t, []interface{}{
			providerErr("Provider for schema identity-registry-v1: tags unavailable", "personInfo", "tags"),
			providerErr("Provider for schema identity-registry-v1: rate limited"),
		}, resp.Errors)
	})

	t.Run("reads missing provider fields as null", func(t *testing.T) {
		op, p := planFor(t, `{ personInfo(nic: "1") { fullName tags } }`, nil)

		resp := merge(op, p, results(p, map[string]map[string]interface{}{"identity-registry-v1": {}}))

		require.Empty(t, resp.Errors)
		assert.Equal(t, map[string]interface{}{"personInfo": map[string]interface{}{"fullName": nil, "tags": nil}}, resp.Data)
	})
}
