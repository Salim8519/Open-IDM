//go:build !windows && !darwin && !linux

package autostart

import "errors"

var errUnsupported = errors.New("autostart: not supported on this platform")

func enable(Entry) error          { return errUnsupported }
func disable(Entry) error         { return nil }
func enabled(Entry) (bool, error) { return false, nil }
