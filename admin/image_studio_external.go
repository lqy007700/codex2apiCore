package admin

import (
	"context"

	"fmt"

	"net"

	"strings"
	"time"
)

// RegisterExternalImageRoutes registers API-key authenticated async Image Studio
// task endpoints for external clients.

// Remote image fetches and Base64 expansion are intentionally after prompt
// inspection. A blocked prompt must not consume network, image-processing,
// database, concurrency, or upstream resources.

// Preflight the whole batch so one accepted job cannot cross an RPM/RPD
// boundary. Reserve concurrency before accepting a legacy asynchronous job.

const (
	externalInputImageFetchTimeout  = 20 * time.Second
	externalInputImageMaxBytes      = 20 << 20
	externalInputImageMaxEncodedLen = (externalInputImageMaxBytes+2)/3*4 + 128
)

var dialPublicExternalInputImageAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host == "" || strings.EqualFold(host, "localhost") {
		return nil, fmt.Errorf("blocked private or reserved address")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("input_images URL host cannot be resolved")
	}
	for _, ip := range ips {
		if !isPublicImageURLIP(ip) {
			return nil, fmt.Errorf("blocked private or reserved address")
		}
	}
	network = "tcp"
	dialer := &net.Dialer{Timeout: externalInputImageFetchTimeout}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func isPublicImageURLIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return !(ip4[0] == 0 || ip4[0] == 10 || ip4[0] == 127 ||
			(ip4[0] == 169 && ip4[1] == 254) ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168) ||
			(ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127) ||
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0) ||
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) ||
			(ip4[0] == 198 && ip4[1] == 18) ||
			(ip4[0] == 198 && ip4[1] == 19) ||
			(ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) ||
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) ||
			ip4[0] >= 224)
	}
	return !(ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast())
}
