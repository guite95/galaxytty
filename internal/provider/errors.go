package provider

import "errors"

var (
	ErrProviderPermissionDenied = errors.New("Android provider permission denied")
	ErrProviderOutput           = errors.New("malformed Android provider output")
)
