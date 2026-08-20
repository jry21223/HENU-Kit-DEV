package contract

// DisplayNamesRoute is generated from platform-core.yaml
// (resolveUserDisplayNames). The request and response payload types below are
// hand-written because cmd/contractgen only renders the route constant for
// this read-only boundary; their shapes mirror the OpenAPI schema
// ResolveUserDisplayNamesRequest and the 200 response data object.

// ResolveUserDisplayNamesRequest carries a bounded, de-duplicated batch of
// user ids (1..100, uuid) for display-name resolution.
type ResolveUserDisplayNamesRequest struct {
	UserIDs []string `json:"user_ids"`
}

// DisplayNameMap maps each requested user id to its display name, or null
// when the name is unset or the id is unknown. Missing ids are never omitted;
// every requested id appears exactly once so callers can batch-degrade.
type DisplayNameMap map[string]*string
