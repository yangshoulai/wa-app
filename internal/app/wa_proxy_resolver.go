package app

import (
	"context"
	"strings"
	"time"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
)

type waProxyStage string

const (
	waProxyStageProbe        waProxyStage = "probe"
	waProxyStageRegistration waProxyStage = "registration"

	waProxySourceRequestOverride = "REQUEST_OVERRIDE"
	waProxySourceRequestPolicy   = "REQUEST_POLICY"
	waProxySourceAccountStage    = "ACCOUNT_STAGE"
	waProxySourceAccountDefault  = "ACCOUNT_DEFAULT"
	waProxySourceSystemStage     = "SYSTEM_STAGE"
	waProxySourceSystemCommon    = "SYSTEM_COMMON"
	waProxySourceDirect          = "DIRECT"

	waProxyModeRequestOverride = "REQUEST_PROXY"
	waProxyModeDirect          = "DIRECT"
	waProxyModeCommon          = "COMMON_PROXY"
)

type waProxyResolveRequest struct {
	Stage         waProxyStage
	Payload       map[string]any
	WAAccountID   string
	CountryCode   string
	Purpose       string
	CorrelationID string
}

func (s *Server) resolveWAProxyRoute(ctx context.Context, req waProxyResolveRequest) (DynamicProxyRoute, bool, error) {
	countryCode := proxyRuntimeCountryCode(firstNonEmpty(req.CountryCode, proxyCountryCodeFromPayload(req.Payload)))
	if proxyURL := actionProxyURL(req.Payload); proxyURL != "" {
		route := staticProxyRoute("request-override", proxyURL, waProxyModeRequestOverride)
		route.CountryCode = countryCode
		route.Source = waProxySourceRequestOverride
		route.PolicyMode = waProxyModeRequestOverride
		return route, true, nil
	}
	if policy, err := waAccountProxyPolicyFromPayload(req.Payload); err != nil {
		return DynamicProxyRoute{}, false, err
	} else if policy != nil {
		if stagePolicy, _ := waProxyStagePolicy(policy, req.Stage); stagePolicy != nil {
			route, useProxy, err := s.resolveWAProxyStagePolicy(ctx, stagePolicy, req, waProxySourceRequestPolicy, countryCode)
			return route, useProxy, err
		}
	}
	accountID := firstNonEmpty(req.WAAccountID, textField(req.Payload, "wa_account_id"))
	if accountID != "" {
		route, useProxy, handled, err := s.resolveWAAccountProxyRoute(ctx, accountID, req, countryCode)
		if handled || err != nil {
			return route, useProxy, err
		}
	}
	if route, ok := s.resolveSystemStageProxyRoute(ctx, req, countryCode); ok {
		return route, true, nil
	}
	if route, ok := s.resolveSystemCommonProxyRoute(countryCode); ok {
		return route, true, nil
	}
	return DynamicProxyRoute{ProxyMode: waProxyModeDirect, CountryCode: "LOCAL", Source: waProxySourceDirect, PolicyMode: waProxyModeDirect}, false, nil
}

func (s *Server) resolveWAAccountProxyRoute(ctx context.Context, accountID string, req waProxyResolveRequest, countryCode string) (DynamicProxyRoute, bool, bool, error) {
	normalizedID, err := requireWAAccountID(accountID)
	if err != nil {
		return DynamicProxyRoute{}, false, true, err
	}
	account, err := s.getWAAccount(ctx, normalizedID)
	if err != nil {
		return DynamicProxyRoute{}, false, true, err
	}
	policy, source := waProxyStagePolicy(account.GetProxyPolicy(), req.Stage)
	if policy == nil {
		return DynamicProxyRoute{}, false, false, nil
	}
	route, useProxy, err := s.resolveWAProxyStagePolicy(ctx, policy, req, source, countryCode)
	return route, useProxy, true, err
}

func waProxyStagePolicy(policy *waappv1.WAAccountProxyPolicy, stage waProxyStage) (*waappv1.WAProxyStagePolicy, string) {
	if policy == nil {
		return nil, ""
	}
	stagePolicy := policy.GetRegistrationPolicy()
	if stage == waProxyStageProbe {
		stagePolicy = policy.GetProbePolicy()
	}
	if !emptyWAProxyStagePolicy(stagePolicy) {
		return stagePolicy, waProxySourceAccountStage
	}
	if !emptyWAProxyStagePolicy(policy.GetDefaultPolicy()) {
		return policy.GetDefaultPolicy(), waProxySourceAccountDefault
	}
	return nil, ""
}

func (s *Server) resolveWAProxyStagePolicy(ctx context.Context, policy *waappv1.WAProxyStagePolicy, req waProxyResolveRequest, source string, countryCode string) (DynamicProxyRoute, bool, error) {
	mode := normalizeWAProxyPolicyMode(policy.GetMode())
	switch mode {
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_INHERIT:
		return DynamicProxyRoute{}, false, nil
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_DIRECT:
		return DynamicProxyRoute{ProxyMode: waProxyModeDirect, CountryCode: "LOCAL", Source: source, PolicyMode: mode.String()}, false, nil
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_COMMON_PROXY:
		route, ok := s.resolveSystemCommonProxyRoute(countryCode)
		if !ok {
			return DynamicProxyRoute{}, false, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "WA common proxy is not configured", true)
		}
		route.Source = source
		route.PolicyMode = mode.String()
		return route, true, nil
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_PROXY_RUNTIME_INGRESS_RULE:
		ruleID := strings.TrimSpace(policy.GetProxyRuntimeIngressRuleId())
		if ruleID == "" {
			return DynamicProxyRoute{}, false, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "proxy_runtime_ingress_rule_id is required", false)
		}
		if s == nil || s.proxyRuntime == nil {
			return DynamicProxyRoute{}, false, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "WA proxy runtime is not configured", true)
		}
		route, err := s.proxyRuntime.GatewayProxyRouteByRuleID(ctx, ruleID, waDynamicProxyRouteRequest(req, countryCode))
		if err != nil {
			return DynamicProxyRoute{}, false, err
		}
		route.Source = source
		route.PolicyMode = mode.String()
		return route, true, nil
	default:
		return DynamicProxyRoute{}, false, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "WA proxy policy mode is unsupported", false)
	}
}

func (s *Server) resolveSystemStageProxyRoute(ctx context.Context, req waProxyResolveRequest, countryCode string) (DynamicProxyRoute, bool) {
	if s == nil {
		return DynamicProxyRoute{}, false
	}
	if username := s.systemStageProxyUsername(req.Stage); username != "" && s.proxyRuntime != nil {
		if route, err := s.proxyRuntime.GatewayProxyRoute(ctx, username, waDynamicProxyRouteRequest(req, countryCode)); err == nil {
			route.Source = waProxySourceSystemStage
			route.PolicyMode = "PROXY_RUNTIME_INGRESS_RULE"
			return route, true
		}
	}
	if proxyURL := s.systemStageProxyURL(req.Stage); proxyURL != "" {
		name := string(req.Stage)
		mode := staticNumberProbeProxyMode
		if req.Stage == waProxyStageRegistration {
			name = "registration"
			mode = staticRegistrationProxyMode
		}
		route := staticProxyRoute(name, proxyURL, mode)
		route.CountryCode = countryCode
		route.Source = waProxySourceSystemStage
		route.PolicyMode = mode
		return route, true
	}
	return DynamicProxyRoute{}, false
}

func (s *Server) resolveSystemCommonProxyRoute(countryCode string) (DynamicProxyRoute, bool) {
	if s == nil || strings.TrimSpace(s.commonProxyURL) == "" {
		return DynamicProxyRoute{}, false
	}
	route := staticProxyRoute("common", s.commonProxyURL, staticCommonProxyMode)
	route.CountryCode = countryCode
	route.Source = waProxySourceSystemCommon
	route.PolicyMode = waProxyModeCommon
	return route, true
}

func (s *Server) systemStageProxyUsername(stage waProxyStage) string {
	if s == nil {
		return ""
	}
	switch stage {
	case waProxyStageProbe:
		return strings.TrimSpace(s.numberProbeProxyUsername)
	case waProxyStageRegistration:
		return strings.TrimSpace(s.registrationProxyUsername)
	default:
		return ""
	}
}

func (s *Server) systemStageProxyURL(stage waProxyStage) string {
	if s == nil {
		return ""
	}
	switch stage {
	case waProxyStageProbe:
		return strings.TrimSpace(s.numberProbeProxyURL)
	case waProxyStageRegistration:
		return strings.TrimSpace(s.registrationProxyURL)
	default:
		return ""
	}
}

func waDynamicProxyRouteRequest(req waProxyResolveRequest, countryCode string) DynamicProxyRouteRequest {
	mode := DynamicProxySessionModeSticky
	if req.Stage == waProxyStageProbe {
		mode = DynamicProxySessionModeRotating
	}
	return DynamicProxyRouteRequest{
		Purpose:       firstNonEmpty(strings.TrimSpace(req.Purpose), "WA_"+strings.ToUpper(string(req.Stage))),
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		CountryCode:   countryCode,
		TTL:           waProxyRouteTTL(req.Stage),
		Mode:          mode,
	}
}

func waProxyRouteTTL(stage waProxyStage) time.Duration {
	if stage == waProxyStageProbe {
		return numberProbeProxyRouteTTL
	}
	return registrationProxyRouteTTL
}

func waProxySummary(route DynamicProxyRoute, useProxy bool) map[string]any {
	if !useProxy {
		return map[string]any{"success": true, "accepted": true, "proxy_mode": waProxyModeDirect, "country_code": "LOCAL", "source": waProxySourceDirect}
	}
	result := map[string]any{
		"success":      true,
		"accepted":     true,
		"proxy_mode":   firstNonEmpty(route.ProxyMode, "PROXY"),
		"country_code": firstNonEmpty(route.CountryCode, "UNKNOWN"),
	}
	if route.Source != "" {
		result["source"] = route.Source
	}
	if route.PolicyMode != "" {
		result["policy_mode"] = route.PolicyMode
	}
	if route.AccountID != "" {
		result["account_id"] = route.AccountID
	}
	if route.RouteID != "" {
		result["route_id"] = route.RouteID
	}
	if route.RuleID != "" {
		result["rule_id"] = route.RuleID
	}
	if route.ProfileID != "" {
		result["profile_id"] = route.ProfileID
	}
	if route.Username != "" {
		result["proxy_username"] = route.Username
	}
	return result
}
