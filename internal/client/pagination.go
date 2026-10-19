package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type collectionPolicy int

const (
	pagedCollection collectionPolicy = iota
	// WLAN and profile-group endpoints on 6.1 return complete data envelopes.
	completeDataCollection
)

const maxCollectionPages = 10000
const maxCollectionBytes = 64 << 20

// listAll returns a complete collection or no rows and an error. A short page
// does not prove completion: controllers may cap the requested page size.
func listAll[T any](ctx context.Context, c *Client, requestURL, identityKey string) ([]T, error) {
	return listCollection[T](ctx, c, requestURL, identityKey, pagedCollection)
}

func listCollection[T any](ctx context.Context, c *Client, requestURL, identityKey string, policy collectionPolicy) ([]T, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return nil, &TransportError{operation: "creating collection request", cause: err}
	}
	query := u.Query()
	query.Set("currentPageSize", "100")
	all := make([]T, 0)
	seen := make(map[string]bool)
	bytesRead := 0
	var total *int
	for page := 1; page <= maxCollectionPages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query.Set("currentPage", strconv.Itoa(page))
		u.RawQuery = query.Encode()
		resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		if len(resp.Result) > maxCollectionBytes-bytesRead {
			return nil, &ProtocolError{reason: "collection exceeds 64 MiB limit"}
		}
		bytesRead += len(resp.Result)
		rows, count, direct, err := decodeCollectionPage(resp.Result, page)
		if err != nil {
			return nil, err
		}
		if policy == completeDataCollection && !direct {
			fields, err := collectionObject(resp.Result)
			if err != nil {
				return nil, err
			}
			_, hasTotal := fields["totalRows"]
			_, hasPage := fields["currentPage"]
			_, hasSize := fields["currentSize"]
			// Metadata, when present, retains the normal strict paged contract.
			direct = !hasTotal && !hasPage && !hasSize
		}
		if page == 1 {
			total = count
		} else if direct || (count == nil) != (total == nil) || (count != nil && *count != *total) {
			return nil, &ProtocolError{reason: "collection pagination changed during listing"}
		}
		if total != nil && (len(rows) > *total-len(all) || (len(rows) == 0 && len(all) < *total)) {
			return nil, &ProtocolError{reason: "collection rows disagree with totalRows"}
		}
		for _, row := range rows {
			fields, err := collectionObject(row)
			if err != nil {
				return nil, err
			}
			var id string
			if err := json.Unmarshal(fields[identityKey], &id); err != nil || id == "" || seen[id] {
				return nil, &ProtocolError{reason: "collection has missing, invalid or duplicate identities"}
			}
			var item T
			if err := json.Unmarshal(row, &item); err != nil {
				return nil, &ProtocolError{reason: "collection row has invalid fields"}
			}
			seen[id] = true
			all = append(all, item)
		}
		if direct || (total != nil && len(all) == *total) || (total == nil && len(rows) == 0) {
			return all, nil
		}
	}
	return nil, &ProtocolError{reason: "collection exceeds 10000 page limit"}
}

func decodeCollectionPage(raw json.RawMessage, page int) ([]json.RawMessage, *int, bool, error) {
	raw = bytes.TrimSpace(raw)
	direct := len(raw) > 0 && raw[0] == '['
	var total *int
	data := raw
	if !direct {
		fields, err := collectionObject(raw)
		if err != nil {
			return nil, nil, false, err
		}
		for _, key := range []string{"totalRows", "currentPage", "currentSize"} {
			value, present := fields[key]
			if !present {
				continue
			}
			var n int
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &n) != nil || n < 0 || (key == "currentPage" && n != page) {
				return nil, nil, false, &ProtocolError{reason: "collection has invalid pagination metadata"}
			}
			if key == "totalRows" {
				total = &n
			}
		}
		data = bytes.TrimSpace(fields["data"])
	}
	var rows []json.RawMessage
	if len(data) == 0 || data[0] != '[' || json.Unmarshal(data, &rows) != nil {
		return nil, nil, false, &ProtocolError{reason: "collection data must be an array"}
	}
	return rows, total, direct, nil
}

// Reject duplicate keys rather than allowing JSON's last value to conceal an
// inconsistent total, identity or data field. Diagnostics never include values.
func collectionObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	invalid := func() (map[string]json.RawMessage, error) {
		return nil, &ProtocolError{reason: "collection requires JSON objects with unique fields"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return invalid()
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return invalid()
		}
		key, ok := token.(string)
		if !ok {
			return invalid()
		}
		if _, exists := fields[key]; exists {
			return invalid()
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return invalid()
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return invalid()
	}
	// raw is a single value taken from F's validated envelope or a decoded array.
	return fields, nil
}
