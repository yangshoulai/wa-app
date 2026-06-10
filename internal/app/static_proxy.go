package app

import "strings"

const (
	staticCommonProxyMode       = "COMMON_PROXY"
	staticNumberProbeProxyMode  = "STATIC_NUMBER_PROBE_PROXY"
	staticRegistrationProxyMode = "STATIC_REGISTRATION_PROXY"
)

func staticProxyRoute(name string, proxyURL string, mode string) DynamicProxyRoute {
	return DynamicProxyRoute{
		AccountID:   "static-" + name + "-proxy",
		RouteID:     "static-" + name + "-proxy",
		ProxyURL:    strings.TrimSpace(proxyURL),
		ProxyMode:   mode,
		CountryCode: "UNKNOWN",
	}
}

func staticProxyResult(mode string) map[string]any {
	return map[string]any{
		"success":      true,
		"accepted":     true,
		"proxy_mode":   mode,
		"country_code": "UNKNOWN",
	}
}

func isStaticProxyRoute(route DynamicProxyRoute) bool {
	return strings.HasPrefix(strings.TrimSpace(route.RouteID), "static-")
}
