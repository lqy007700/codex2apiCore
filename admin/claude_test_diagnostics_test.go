package admin

func claudeDiagnosticFrame(value string) string { return "data: " + value + "\n\n" }

// Also redact a known token that straddles the preview boundary.
