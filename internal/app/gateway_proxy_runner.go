package app

import (
	"context"
	"strings"
	"time"
)

type gatewayProxyEngineRequest struct {
	Username      string
	Purpose       string
	CorrelationID string
	TTL           time.Duration
	Mode          DynamicProxySessionMode
}

func (s *Server) optionalGatewayProxyEngine(ctx context.Context, native *NativeEngine, req gatewayProxyEngineRequest) (*NativeEngine, func(), bool) {
	if native == nil || strings.TrimSpace(native.activeProxyURL) != "" || s == nil || s.proxyRuntime == nil {
		return native, func() {}, false
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return native, func() {}, false
	}
	route, err := s.proxyRuntime.GatewayProxyRoute(ctx, username, DynamicProxyRouteRequest{
		Purpose:       req.Purpose,
		CorrelationID: req.CorrelationID,
		TTL:           req.TTL,
		Mode:          req.Mode,
	})
	if err != nil {
		return native, func() {}, false
	}
	proxied, err := native.WithProxyURL(route.ProxyURL)
	if err != nil {
		_ = s.proxyRuntime.ReleaseProxyRoute(context.Background(), route)
		return native, func() {}, false
	}
	return proxied, func() { _ = s.proxyRuntime.ReleaseProxyRoute(context.Background(), route) }, true
}
