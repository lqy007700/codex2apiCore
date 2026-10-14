package admin

// Save each output before requesting the next. Neither input Base64 nor output
// buffers from previous outputs survive a wait on another upstream request.

// Read compressed image bytes only if a requested transformation is necessary.
// Normal saves stream the file to local/S3 storage without pixel decoding.
