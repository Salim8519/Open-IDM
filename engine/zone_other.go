//go:build !windows && !darwin

package engine

func markFromInternet(path, sourceURL, referrer string) {}
