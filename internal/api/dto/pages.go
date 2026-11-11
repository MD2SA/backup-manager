package dto

// List endpoints return a keyset page. next_cursor is empty on the last page.

type ExecutionPage struct {
	Items      []ExecutionResponse `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
	HasMore    bool                `json:"has_more"`
}

type RestorePage struct {
	Items      []RestoreResponse `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

type AuditPage struct {
	Items      []AuditEntryResponse `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
	HasMore    bool                 `json:"has_more"`
}

type UserPage struct {
	Items      []UserResponse `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

type ProfilePage struct {
	Items      []ProfileResponse `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

type StorageProviderPage struct {
	Items      []StorageProviderResponse `json:"items"`
	NextCursor string                    `json:"next_cursor,omitempty"`
	HasMore    bool                      `json:"has_more"`
}

type NotificationProviderPage struct {
	Items      []NotificationProviderResponse `json:"items"`
	NextCursor string                         `json:"next_cursor,omitempty"`
	HasMore    bool                           `json:"has_more"`
}

type RetentionPolicyPage struct {
	Items      []RetentionPolicyResponse `json:"items"`
	NextCursor string                    `json:"next_cursor,omitempty"`
	HasMore    bool                      `json:"has_more"`
}

type APIKeyPage struct {
	Items      []APIKeyResponse `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
}
