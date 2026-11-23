// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package kernel

// CollectionResponse Generic collection response
type CollectionResponse struct {
	Items interface{} `json:"items"`
	Count int         `json:"count"`
}
