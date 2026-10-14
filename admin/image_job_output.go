package admin

// GetExternalImageJobOutput streams Base64 without aggregating decoded images
// or Base64 JSON in memory. Only this explicit delivery route consumes images
// whose caller opted into delete_after_read; polling/preview routes never do.

// Windows requires closed files before removing them.

// A failed/partial delivery must retain the images for retry until TTL.

// AcknowledgeExternalImageJobOutput lets URL-based clients explicitly confirm
// durable receipt. It is idempotent and never deletes legacy/temporary outputs.
