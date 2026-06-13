package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
)

type DynamicProxyRoute struct {
	AccountID   string
	RouteID     string
	LeaseID     string
	Username    string
	RuleID      string
	ProfileID   string
	ProxyURL    string
	ProxyMode   string
	CountryCode string
	Source      string
	PolicyMode  string
	ExpiresAt   time.Time
}

type DynamicProxySessionMode string

const (
	DynamicProxySessionModeRotating DynamicProxySessionMode = "rotating"
	DynamicProxySessionModeSticky   DynamicProxySessionMode = "sticky"
)

type DynamicProxyRouteRequest struct {
	Purpose       string
	CorrelationID string
	SessionID     string
	CountryCode   string
	TTL           time.Duration
	Mode          DynamicProxySessionMode
}

type DynamicProxyRuntime struct {
	baseURL       string
	gatewayScheme string
	client        *http.Client
	mu            sync.Mutex
	rules         []proxyIngressRuleSettings
	rulesExpireAt time.Time
}

type gatewayProxyRule struct {
	RuleID    string
	Username  string
	Password  string
	ProfileID string
}

type releaseProxyLeaseRequest struct {
	LeaseID string `json:"lease_id"`
}

type proxyRuntimeSettingsResponse struct {
	Settings proxyRuntimeSettings `json:"settings"`
}

type proxyRuntimeSettings struct {
	IngressRules []proxyIngressRuleSettings `json:"ingress_rules"`
}

type proxyIngressRuleSettings struct {
	Enabled       bool   `json:"enabled"`
	RuleID        string `json:"rule_id"`
	Username      string `json:"username"`
	PasswordValue string `json:"password_value"`
	ProfileID     string `json:"profile_id"`
}

const (
	proxyRuntimeGatewayPort      = "10810"
	proxyRuntimeRequestTimeout   = 5 * time.Second
	proxyRuntimeSettingsCacheTTL = 5 * time.Minute
)

func NewDynamicProxyRuntime(baseURL string, gatewayProtocol string) *DynamicProxyRuntime {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &DynamicProxyRuntime{baseURL: baseURL, gatewayScheme: gatewayProxyScheme(gatewayProtocol), client: &http.Client{Timeout: 20 * time.Second}}
}

func (p *DynamicProxyRuntime) GatewayProxyRoute(ctx context.Context, username string, routeReq DynamicProxyRouteRequest) (DynamicProxyRoute, error) {
	rule, err := p.gatewayProxyRule(ctx, username)
	if err != nil {
		return DynamicProxyRoute{}, err
	}
	proxyURL, err := p.gatewayProxyURL(rule.Username, rule.Password)
	if err != nil {
		return DynamicProxyRoute{}, err
	}
	now := time.Now().UTC()
	expiresAt := time.Time{}
	if routeReq.TTL > 0 {
		expiresAt = now.Add(routeReq.TTL)
	}
	routeID := proxyRouteID(rule.Username, routeReq)
	accountID := routeID
	countryCode := proxyRuntimeCountryCode(routeReq.CountryCode)
	return DynamicProxyRoute{
		AccountID:   accountID,
		RouteID:     routeID,
		Username:    rule.Username,
		RuleID:      rule.RuleID,
		ProfileID:   rule.ProfileID,
		ProxyURL:    proxyURL,
		ProxyMode:   proxyRouteModeForCountry(routeReq.Mode, countryCode),
		CountryCode: countryCode,
		ExpiresAt:   expiresAt,
	}, nil
}

func (p *DynamicProxyRuntime) GatewayProxyURL(ctx context.Context, username string) (string, error) {
	rule, err := p.gatewayProxyRule(ctx, username)
	if err != nil {
		return "", err
	}
	return p.gatewayProxyURL(rule.Username, rule.Password)
}

func (p *DynamicProxyRuntime) GatewayProxyRouteByRuleID(ctx context.Context, ruleID string, routeReq DynamicProxyRouteRequest) (DynamicProxyRoute, error) {
	rule, err := p.gatewayProxyRuleByID(ctx, ruleID)
	if err != nil {
		return DynamicProxyRoute{}, err
	}
	proxyURL, err := p.gatewayProxyURL(rule.Username, rule.Password)
	if err != nil {
		return DynamicProxyRoute{}, err
	}
	countryCode := proxyRuntimeCountryCode(routeReq.CountryCode)
	return DynamicProxyRoute{
		AccountID:   "proxy-runtime-rule-" + safeProxyRouteToken(rule.RuleID),
		RouteID:     "gateway-rule-" + safeProxyRouteToken(rule.RuleID),
		Username:    rule.Username,
		RuleID:      rule.RuleID,
		ProfileID:   rule.ProfileID,
		ProxyURL:    proxyURL,
		ProxyMode:   "PROXY_RUNTIME_INGRESS_RULE",
		CountryCode: countryCode,
		ExpiresAt:   time.Time{},
	}, nil
}

func (p *DynamicProxyRuntime) ReleaseProxyRoute(ctx context.Context, route DynamicProxyRoute) error {
	leaseID := strings.TrimSpace(route.LeaseID)
	if p == nil || p.baseURL == "" || leaseID == "" {
		return nil
	}
	endpoint, err := p.endpoint("/leases/release")
	if err != nil {
		return err
	}
	payload := releaseProxyLeaseRequest{LeaseID: leaseID}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxyRuntimeRouteError("gateway profile release", resp.StatusCode, body)
	}
	return nil
}

func (p *DynamicProxyRuntime) gatewayProxyRule(ctx context.Context, username string) (gatewayProxyRule, error) {
	if p == nil || p.baseURL == "" {
		return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "WA proxy runtime is not configured", false)
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "gateway username is required", false)
	}
	if rule, ok := p.cachedGatewayProxyRule(username, time.Now().UTC()); ok {
		return rule, nil
	}
	rules, err := p.fetchGatewayProxyRules(ctx)
	if err != nil {
		if rule, ok := p.cachedGatewayProxyRule(username, time.Now().UTC()); ok {
			return rule, nil
		}
		return gatewayProxyRule{}, err
	}
	p.cacheGatewayProxyRules(rules, time.Now().UTC().Add(proxyRuntimeSettingsCacheTTL))
	if rule, ok := gatewayProxyRuleFromSettings(username, rules); ok {
		return rule, nil
	}
	return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, fmt.Sprintf("proxy-runtime gateway user %q is unavailable", username), true)
}

func (p *DynamicProxyRuntime) gatewayProxyRuleByID(ctx context.Context, ruleID string) (gatewayProxyRule, error) {
	if p == nil || p.baseURL == "" {
		return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "WA proxy runtime is not configured", false)
	}
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "gateway rule_id is required", false)
	}
	if rule, ok := p.cachedGatewayProxyRuleByID(ruleID, time.Now().UTC()); ok {
		return rule, nil
	}
	rules, err := p.fetchGatewayProxyRules(ctx)
	if err != nil {
		if rule, ok := p.cachedGatewayProxyRuleByID(ruleID, time.Now().UTC()); ok {
			return rule, nil
		}
		return gatewayProxyRule{}, err
	}
	p.cacheGatewayProxyRules(rules, time.Now().UTC().Add(proxyRuntimeSettingsCacheTTL))
	if rule, ok := gatewayProxyRuleByIDFromSettings(ruleID, rules); ok {
		return rule, nil
	}
	return gatewayProxyRule{}, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, fmt.Sprintf("proxy-runtime gateway rule %q is unavailable", ruleID), true)
}

func (p *DynamicProxyRuntime) fetchGatewayProxyRules(ctx context.Context) ([]proxyIngressRuleSettings, error) {
	endpoint, err := p.endpoint("/settings/in-user-rules")
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, proxyRuntimeRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "proxy-runtime gateway ingress unavailable", true)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, proxyRuntimeRouteError("gateway ingress", resp.StatusCode, body)
	}
	var settings proxyRuntimeSettingsResponse
	if err := json.Unmarshal(body, &settings); err != nil {
		return nil, NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, "proxy-runtime gateway ingress response is invalid", true)
	}
	return settings.Settings.IngressRules, nil
}

func gatewayProxyRuleFromSettings(username string, rules []proxyIngressRuleSettings) (gatewayProxyRule, bool) {
	username = strings.TrimSpace(username)
	for _, rule := range rules {
		if !rule.Enabled || strings.TrimSpace(rule.Username) != username {
			continue
		}
		return gatewayProxyRule{RuleID: strings.TrimSpace(rule.RuleID), Username: username, Password: rule.PasswordValue, ProfileID: strings.TrimSpace(rule.ProfileID)}, true
	}
	return gatewayProxyRule{}, false
}

func gatewayProxyRuleByIDFromSettings(ruleID string, rules []proxyIngressRuleSettings) (gatewayProxyRule, bool) {
	ruleID = strings.TrimSpace(ruleID)
	for _, rule := range rules {
		if !rule.Enabled || strings.TrimSpace(rule.RuleID) != ruleID {
			continue
		}
		username := strings.TrimSpace(rule.Username)
		if username == "" {
			continue
		}
		return gatewayProxyRule{RuleID: ruleID, Username: username, Password: rule.PasswordValue, ProfileID: strings.TrimSpace(rule.ProfileID)}, true
	}
	return gatewayProxyRule{}, false
}

func (p *DynamicProxyRuntime) cachedGatewayProxyRule(username string, now time.Time) (gatewayProxyRule, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if now.IsZero() || p.rulesExpireAt.IsZero() || !now.Before(p.rulesExpireAt) {
		return gatewayProxyRule{}, false
	}
	return gatewayProxyRuleFromSettings(username, p.rules)
}

func (p *DynamicProxyRuntime) cachedGatewayProxyRuleByID(ruleID string, now time.Time) (gatewayProxyRule, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if now.IsZero() || p.rulesExpireAt.IsZero() || !now.Before(p.rulesExpireAt) {
		return gatewayProxyRule{}, false
	}
	return gatewayProxyRuleByIDFromSettings(ruleID, p.rules)
}

func (p *DynamicProxyRuntime) cacheGatewayProxyRules(rules []proxyIngressRuleSettings, expiresAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rules = append([]proxyIngressRuleSettings{}, rules...)
	p.rulesExpireAt = expiresAt
}

func (p *DynamicProxyRuntime) endpoint(path string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(p.baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid WA proxy runtime API URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (p *DynamicProxyRuntime) gatewayProxyURL(username string, password string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(p.baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid WA proxy runtime API URL")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid WA proxy runtime API URL")
	}
	gateway := &url.URL{
		Scheme: p.gatewayScheme,
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, proxyRuntimeGatewayPort),
	}
	return gateway.String(), nil
}

func gatewayProxyScheme(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	protocol = strings.TrimSuffix(protocol, "://")
	switch protocol {
	case "http", "https", "socks5", "socks5h":
		return protocol
	case "mixed":
		return "http"
	case "socks", "socks5-proxy":
		return "socks5"
	default:
		return "socks5"
	}
}

func proxyRouteID(username string, req DynamicProxyRouteRequest) string {
	seed := strings.Join([]string{username, req.Purpose, req.CorrelationID, req.SessionID, proxyRouteMode(req.Mode)}, ":")
	return "gateway-" + safeProxyRouteToken(username) + "-" + stableID(seed)
}

func proxyRouteMode(mode DynamicProxySessionMode) string {
	return proxyRouteModeForCountry(mode, "US")
}

func proxyRouteModeForCountry(mode DynamicProxySessionMode, countryCode string) string {
	countryCode = proxyRuntimeCountryCode(countryCode)
	switch mode {
	case DynamicProxySessionModeRotating:
		return countryCode + "_ROTATING_DYNAMIC_IP"
	case DynamicProxySessionModeSticky:
		return countryCode + "_STICKY_DYNAMIC_IP"
	default:
		return "GATEWAY_PROFILE"
	}
}

func proxyCountryCodeFromPayload(payload map[string]any) string {
	phone := objectField(payload, "phone")
	proxy := objectField(payload, "proxy")
	value := firstNonEmpty(
		textField(payload, "proxy_country_code"),
		textField(proxy, "country_code"),
		textField(proxy, "proxy_country_code"),
		textField(payload, "country_iso2"),
		textField(payload, "country_region"),
		textField(payload, "region"),
		textField(phone, "country_iso2"),
	)
	if value != "" {
		return proxyRuntimeCountryCode(value)
	}
	callingCode := firstNonEmpty(
		textField(payload, "country_calling_code"),
		textField(payload, "cc"),
		textField(payload, "country_code"),
		textField(phone, "country_calling_code"),
	)
	return proxyRuntimeCountryCode(proxyCountryCodeFromCallingCode(callingCode))
}

func proxyCountryCodeFromCallingCode(value string) string {
	switch strings.TrimPrefix(strings.TrimSpace(value), "+") {
	case "1":
		return "US"
	case "48":
		return "PL"
	case "57":
		return "CO"
	case "63":
		return "PH"
	case "84":
		return "VN"
	case "86":
		return "CN"
	default:
		return ""
	}
}

func proxyRuntimeCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "+")
	switch value {
	case "", "1", "USA", "UNITEDSTATES", "UNITED_STATES":
		return "US"
	default:
		return value
	}
}

func safeProxyRouteToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			out.WriteRune(char)
		case char >= '0' && char <= '9':
			out.WriteRune(char)
		case char == '-' || char == '_':
			out.WriteByte('-')
		}
	}
	token := strings.Trim(out.String(), "-")
	if token == "" {
		return "proxy"
	}
	if len(token) > 48 {
		return token[:48]
	}
	return token
}

func proxyRuntimeRouteError(resource string, statusCode int, body []byte) error {
	message := fmt.Sprintf("proxy-runtime %s unavailable: HTTP %d", strings.TrimSpace(resource), statusCode)
	if detail := proxyRuntimeErrorDetail(body); detail != "" {
		message += ": " + detail
	}
	return NewError(waappv1.WaErrorCode_WA_ERROR_CODE_ROUTE_UNAVAILABLE, message, true)
}

func proxyRuntimeErrorDetail(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	detail := strings.Join(strings.Fields(payload.Message), " ")
	if detail == "" || strings.Contains(detail, "://") {
		return ""
	}
	const maxDetailLength = 180
	if len(detail) > maxDetailLength {
		return detail[:maxDetailLength]
	}
	return detail
}
