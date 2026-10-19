package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetSiteSettingsRaw retains section fields so updates can detect unknown
// siblings before replacing a section. Passwords must not be copied into state.
func (c *Client) GetSiteSettingsRaw(ctx context.Context, siteID string) (map[string]any, error) {
	resp, err := c.doSiteRequest(ctx, siteID, http.MethodGet, "/setting", nil)
	if err != nil {
		return nil, err
	}
	var settings map[string]any
	if err := json.Unmarshal(resp.Result, &settings); err != nil {
		return nil, fmt.Errorf("decoding site settings: %w", err)
	}
	if settings == nil {
		return nil, fmt.Errorf("site settings response is missing")
	}
	return settings, nil
}

// PatchSiteSettings acknowledges the write separately from a subsequent read.
// A successful credential rotation must survive an unrelated readback failure.
func (c *Client) PatchSiteSettings(ctx context.Context, siteID string, settings map[string]any) error {
	_, err := c.doSiteRequest(ctx, siteID, http.MethodPatch, "/setting", settings)
	return err
}
