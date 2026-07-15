package asgardeo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
)

type CreateGroupRequestBody struct {
	DisplayName string                   `json:"displayName"`
	Members     []GroupMemberRequestBody `json:"members,omitempty"`
	Schemas     []string                 `json:"schemas"`
}

type GetGroupResponseBody struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Members     []struct {
		Value   string `json:"value"`
		Display string `json:"display"`
	} `json:"members"`
	Schemas []string `json:"schemas"`
}

type CreateGroupResponseBody struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Members     []struct {
		Value   string `json:"value"`
		Display string `json:"display"`
	} `json:"members"`
	Schemas []string `json:"schemas"`
}

type GroupMemberRequestBody struct {
	Value   string `json:"value"`
	Display string `json:"display"`
}

type PatchGroupOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

type PatchGroupRequestBody struct {
	Schemas    []string              `json:"schemas"`
	Operations []PatchGroupOperation `json:"Operations"`
}

func (a *Client) CreateGroup(ctx context.Context, group *idp.Group) (*idp.GroupInfo, error) {
	url := fmt.Sprintf("%s/scim2/Groups", a.BaseURL)

	body := CreateGroupRequestBody{
		DisplayName: fmt.Sprintf("DEFAULT/%s", group.DisplayName),
		Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
	}

	// Add members if provided
	if len(group.Members) > 0 {
		members := make([]GroupMemberRequestBody, len(group.Members))
		for i, member := range group.Members {
			members[i] = GroupMemberRequestBody{
				Value:   member.Value,
				Display: member.Display,
			}
		}
		body.Members = members
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/scim+json")

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create group, status code: %d", res.StatusCode)
	}

	var response CreateGroupResponseBody
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	groupInfo := &idp.GroupInfo{
		Id:          response.ID,
		DisplayName: response.DisplayName,
	}

	if len(response.Members) > 0 {
		members := make([]idp.GroupMember, len(response.Members))
		for i, member := range response.Members {
			members[i] = idp.GroupMember{
				Value:   member.Value,
				Display: member.Display,
			}
		}
		groupInfo.Members = members
	}

	return groupInfo, nil
}

func (a *Client) GetGroup(ctx context.Context, groupId string) (*idp.GroupInfo, error) {
	url := fmt.Sprintf("%s/scim2/Groups/%s", a.BaseURL, groupId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get group, status code: %d", res.StatusCode)
	}

	var response GetGroupResponseBody
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	groupInfo := &idp.GroupInfo{
		Id:          response.ID,
		DisplayName: strings.TrimPrefix(response.DisplayName, "DEFAULT/"),
	}

	if len(response.Members) > 0 {
		members := make([]idp.GroupMember, len(response.Members))
		for i, member := range response.Members {
			members[i] = idp.GroupMember{
				Value:   member.Value,
				Display: member.Display,
			}
		}
		groupInfo.Members = members
	}

	return groupInfo, nil
}

func (a *Client) GetGroupByName(ctx context.Context, groupName string) (*string, error) {
	displayName := fmt.Sprintf("DEFAULT/%s", groupName)
	url := fmt.Sprintf("%s/scim2/Groups/.search", a.BaseURL)

	searchRequest := struct {
		Schemas    []string `json:"schemas"`
		StartIndex int      `json:"startIndex"`
		Filter     string   `json:"filter"`
	}{
		Schemas:    []string{"urn:ietf:params:scim:api:messages:2.0:SearchRequest"},
		StartIndex: 1,
		Filter:     fmt.Sprintf("displayName eq \"%s\"", displayName),
	}

	payload, err := json.Marshal(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/scim+json")

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to search group by name, status code: %d", res.StatusCode)
	}

	var response struct {
		TotalResults int                    `json:"totalResults"`
		Resources    []GetGroupResponseBody `json:"Resources"`
	}
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.TotalResults == 0 || len(response.Resources) == 0 {
		return nil, fmt.Errorf("group with name %s not found", groupName)
	}

	groupId := &response.Resources[0].ID
	return groupId, nil
}

func (a *Client) UpdateGroup(ctx context.Context, groupId string, group *idp.Group) (*idp.GroupInfo, error) {
	url := fmt.Sprintf("%s/scim2/Groups/%s", a.BaseURL, groupId)

	body := CreateGroupRequestBody{
		DisplayName: fmt.Sprintf("DEFAULT/%s", group.DisplayName),
		Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
	}

	// Add members if provided
	if len(group.Members) > 0 {
		members := make([]GroupMemberRequestBody, len(group.Members))
		for i, member := range group.Members {
			members[i] = GroupMemberRequestBody{
				Value:   member.Value,
				Display: member.Display,
			}
		}
		body.Members = members
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/scim+json")

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update group, status code: %d", res.StatusCode)
	}

	var response CreateGroupResponseBody
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	groupInfo := &idp.GroupInfo{
		Id:          response.ID,
		DisplayName: strings.TrimPrefix(response.DisplayName, "DEFAULT/"),
	}

	if len(response.Members) > 0 {
		members := make([]idp.GroupMember, len(response.Members))
		for i, member := range response.Members {
			members[i] = idp.GroupMember{
				Value:   member.Value,
				Display: member.Display,
			}
		}
		groupInfo.Members = members
	}

	return groupInfo, nil
}

func (a *Client) DeleteGroup(ctx context.Context, groupId string) error {
	url := fmt.Sprintf("%s/scim2/Groups/%s", a.BaseURL, groupId)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete group, status code: %d", res.StatusCode)
	}

	return nil
}

func (a *Client) AddMemberToGroup(ctx context.Context, groupId *string, memberInfo *idp.GroupMember) error {
	url := fmt.Sprintf("%s/scim2/Groups/%s", a.BaseURL, *groupId)

	body := PatchGroupRequestBody{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []PatchGroupOperation{
			{
				Op: "add",
				Value: map[string]interface{}{
					"members": []GroupMemberRequestBody{
						{
							Value:   memberInfo.Value,
							Display: memberInfo.Display,
						},
					},
				},
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/scim+json")

	res, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to add member to group, status code: %d", res.StatusCode)
	}

	return nil
}

func (a *Client) AddMemberToGroupByGroupName(ctx context.Context, groupName string, memberInfo *idp.GroupMember) (*string, error) {
	groupId, err := a.GetGroupByName(ctx, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get group by name: %w", err)
	}

	return groupId, a.AddMemberToGroup(ctx, groupId, memberInfo)
}

func (a *Client) RemoveMemberFromGroup(ctx context.Context, groupId string, userId string) error {
	url := fmt.Sprintf("%s/scim2/Groups/%s", a.BaseURL, groupId)

	body := PatchGroupRequestBody{
		Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		Operations: []PatchGroupOperation{
			{
				Op:   "remove",
				Path: fmt.Sprintf("members[value eq \"%s\"]", userId),
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/scim+json")

	res, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to remove member from group, status code: %d", res.StatusCode)
	}

	return nil
}
