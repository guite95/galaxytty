package adb

import "errors"

var (
	ErrADBNotFound  = errors.New("adb executable not found")
	ErrUnauthorized = errors.New("adb device unauthorized")
	ErrOffline      = errors.New("adb device offline")
)
