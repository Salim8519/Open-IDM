package main

// The host keeps its own copies of the few message types it handles instead of importing
// the engine, so peregrine-host.exe stays small (ADR 0015). TestWireTypesMatchApp checks
// that every field here still exists in the app's types with the same JSON name and type.

// wireRequest is the part of engine.Request the extension may set.
type wireRequest struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Paused   bool              `json:"paused,omitempty"`
	Kind     string            `json:"kind,omitempty"`
}

// wireCapture is the part of api.Capture the extension may set.
type wireCapture struct {
	wireRequest
	Source   string `json:"source"`
	PageURL  string `json:"pageUrl,omitempty"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}
