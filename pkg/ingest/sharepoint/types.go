package sharepoint

import (
	"encoding/json"
	"time"
)

// spFilesResponse is the JSON envelope for a Files collection endpoint.
// NextLink carries the odata.nextLink continuation URL for pagination.
type spFilesResponse struct {
	Value    []spFile `json:"value"`
	NextLink string   `json:"odata.nextLink"`
}

// spFoldersResponse is the JSON envelope for a Folders collection endpoint.
// NextLink carries the odata.nextLink continuation URL for pagination.
type spFoldersResponse struct {
	Value    []spFolder `json:"value"`
	NextLink string     `json:"odata.nextLink"`
}

// spFile represents a single file from the SharePoint REST API.
type spFile struct {
	Name              string                     `json:"Name"`
	ServerRelativeURL string                     `json:"ServerRelativeUrl"`
	TimeLastModified  time.Time                  `json:"TimeLastModified"`
	UniqueID          string                     `json:"UniqueId"`
	ETag              string                     `json:"ETag"`
	ListItemAllFields map[string]json.RawMessage `json:"ListItemAllFields"`
}

// spFolder represents a single folder from the SharePoint REST API.
type spFolder struct {
	Name              string `json:"Name"`
	ServerRelativeURL string `json:"ServerRelativeUrl"`
}
