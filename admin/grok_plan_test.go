package admin

import (
	"encoding/base64"
	"encoding/json"
)

func testGrokJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return header + "." + payload + ".sig"
}
