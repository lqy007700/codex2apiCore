package admin

import (
	"net/http"

	"strings"
	"testing"
)

func stringPtr(value string) *string { return &value }

func TestParseProxyRiskScoringResponseExtractsReferenceFields(t *testing.T) {
	body := []byte(`{"scamalytics":{"scamalytics_score":78,"scamalytics_risk":"high","scamalytics_proxy":{"is_vpn":true,"is_datacenter":true},"credits":{"remaining":120,"used":30}},"external_datasources":{"ip2proxy_lite":{"proxy_type":"DCH","ip_country_code":"DE"},"firehol":{"is_proxy":true}}}`)
	result, credits, err := parseProxyRiskScoringResponse(body, 42)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score == nil || *result.Score != 78 || result.RiskLevel != "high" || !result.IsVPN || !result.IsDatacenter || result.ProxyType != "DCH" || result.Country != "DE" || result.LatencyMS != 42 {
		t.Fatalf("parsed result = %+v", result)
	}
	if credits == nil || credits.Remaining == nil || *credits.Remaining != 120 || credits.Used == nil || *credits.Used != 30 {
		t.Fatalf("parsed credits = %+v", credits)
	}
}

func TestParseProxyRiskScoringResponseAcceptsIntegralFloatScore(t *testing.T) {
	result, _, err := parseProxyRiskScoringResponse([]byte(`{"score":78.0,"risk":"medium"}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score == nil || *result.Score != 78 {
		t.Fatalf("parsed float score = %+v", result.Score)
	}
}

func TestParseProxyRiskScoringResponseRedactsFeatureSecrets(t *testing.T) {
	result, _, err := parseProxyRiskScoringResponse([]byte(`{"score":12,"external_datasources":{"provider_key":"secret-value","name":"safe"}}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.FeaturesJSON, "secret-value") || !strings.Contains(result.FeaturesJSON, "[redacted]") {
		t.Fatalf("feature JSON leaked a secret: %s", result.FeaturesJSON)
	}
}

func TestParseProxyRiskScoringResponseRejectsNestedProviderError(t *testing.T) {
	_, _, err := parseProxyRiskScoringResponse([]byte(`{"scamalytics":{"error":"invalid API key"}}`), 1)
	if err == nil || !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("nested provider error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
