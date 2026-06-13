package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

var waAccountIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{0,127}$`)

func newWAAccount(id string, displayName string, phone *waappv1.PhoneTarget, status waappv1.WAAccountStatus, audit *waappv1.AuditStamp) *waappv1.WAAccount {
	phone = normalizePhone(phone)
	return &waappv1.WAAccount{
		WaAccountId: strings.TrimSpace(id),
		DisplayName: strings.TrimSpace(displayName),
		Phone:       phone,
		Status:      normalizeWAAccountStatus(status),
		Audit:       audit,
	}
}

func withWAAccountStatus(account *waappv1.WAAccount, status waappv1.WAAccountStatus, updatedAt time.Time) *waappv1.WAAccount {
	createdAt := waAccountCreatedAt(account)
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	next := newWAAccount(waAccountID(account), account.GetDisplayName(), account.GetPhone(), status, audit(createdAt, updatedAt))
	next.TwoFactorAuth = cloneTwoFactorAuthStatus(account.GetTwoFactorAuth())
	next.ProxyPolicy = cloneWAAccountProxyPolicy(account.GetProxyPolicy())
	return next
}

func withWAAccountDisplayName(account *waappv1.WAAccount, displayName string, updatedAt time.Time) *waappv1.WAAccount {
	createdAt := waAccountCreatedAt(account)
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	next := newWAAccount(waAccountID(account), displayName, account.GetPhone(), waAccountStatus(account), audit(createdAt, updatedAt))
	next.TwoFactorAuth = cloneTwoFactorAuthStatus(account.GetTwoFactorAuth())
	next.ProxyPolicy = cloneWAAccountProxyPolicy(account.GetProxyPolicy())
	return next
}

func withWAAccountTwoFactorAuthStatus(account *waappv1.WAAccount, status *waappv1.TwoFactorAuthStatus, updatedAt time.Time) *waappv1.WAAccount {
	createdAt := waAccountCreatedAt(account)
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	next := newWAAccount(waAccountID(account), account.GetDisplayName(), account.GetPhone(), waAccountStatus(account), audit(createdAt, updatedAt))
	next.TwoFactorAuth = cloneTwoFactorAuthStatus(status)
	next.ProxyPolicy = cloneWAAccountProxyPolicy(account.GetProxyPolicy())
	return next
}

func withWAAccountProxyPolicy(account *waappv1.WAAccount, policy *waappv1.WAAccountProxyPolicy, updatedAt time.Time) *waappv1.WAAccount {
	createdAt := waAccountCreatedAt(account)
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	next := newWAAccount(waAccountID(account), account.GetDisplayName(), account.GetPhone(), waAccountStatus(account), audit(createdAt, updatedAt))
	next.TwoFactorAuth = cloneTwoFactorAuthStatus(account.GetTwoFactorAuth())
	next.ProxyPolicy = cloneWAAccountProxyPolicy(policy)
	return next
}

func cloneTwoFactorAuthStatus(status *waappv1.TwoFactorAuthStatus) *waappv1.TwoFactorAuthStatus {
	if status == nil {
		return nil
	}
	return &waappv1.TwoFactorAuthStatus{
		Configured:      status.GetConfigured(),
		EmailConfigured: status.GetEmailConfigured(),
		EmailAddress:    strings.TrimSpace(status.GetEmailAddress()),
		EmailVerified:   status.GetEmailVerified(),
		EmailConfirmed:  status.GetEmailConfirmed(),
	}
}

func cloneWAAccountProxyPolicy(policy *waappv1.WAAccountProxyPolicy) *waappv1.WAAccountProxyPolicy {
	if policy == nil {
		return nil
	}
	normalized := &waappv1.WAAccountProxyPolicy{
		DefaultPolicy:      cloneWAProxyStagePolicy(policy.GetDefaultPolicy()),
		ProbePolicy:        cloneWAProxyStagePolicy(policy.GetProbePolicy()),
		RegistrationPolicy: cloneWAProxyStagePolicy(policy.GetRegistrationPolicy()),
	}
	if emptyWAAccountProxyPolicy(normalized) {
		return nil
	}
	return normalized
}

func cloneWAProxyStagePolicy(policy *waappv1.WAProxyStagePolicy) *waappv1.WAProxyStagePolicy {
	if policy == nil {
		return nil
	}
	mode := normalizeWAProxyPolicyMode(policy.GetMode())
	ruleID := strings.TrimSpace(policy.GetProxyRuntimeIngressRuleId())
	if mode != waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_PROXY_RUNTIME_INGRESS_RULE {
		ruleID = ""
	}
	if mode == waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_INHERIT && ruleID == "" {
		return nil
	}
	return &waappv1.WAProxyStagePolicy{
		Mode:                      mode,
		ProxyRuntimeIngressRuleId: ruleID,
	}
}

func normalizeWAProxyPolicyMode(mode waappv1.WAProxyPolicyMode) waappv1.WAProxyPolicyMode {
	if mode == waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_UNSPECIFIED {
		return waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_INHERIT
	}
	return mode
}

func emptyWAAccountProxyPolicy(policy *waappv1.WAAccountProxyPolicy) bool {
	if policy == nil {
		return true
	}
	return emptyWAProxyStagePolicy(policy.GetDefaultPolicy()) &&
		emptyWAProxyStagePolicy(policy.GetProbePolicy()) &&
		emptyWAProxyStagePolicy(policy.GetRegistrationPolicy())
}

func emptyWAProxyStagePolicy(policy *waappv1.WAProxyStagePolicy) bool {
	return policy == nil ||
		(normalizeWAProxyPolicyMode(policy.GetMode()) == waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_INHERIT &&
			strings.TrimSpace(policy.GetProxyRuntimeIngressRuleId()) == "")
}

func waAccountProxyPolicyJSON(policy *waappv1.WAAccountProxyPolicy) string {
	normalized := cloneWAAccountProxyPolicy(policy)
	if normalized == nil {
		return "{}"
	}
	marshaler := protojson.MarshalOptions{UseProtoNames: true}
	data, err := marshaler.Marshal(normalized)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func waAccountProxyPolicyFromJSON(value string) *waappv1.WAAccountProxyPolicy {
	value = strings.TrimSpace(value)
	if value == "" || value == "{}" || value == "null" {
		return nil
	}
	policy := &waappv1.WAAccountProxyPolicy{}
	unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshaler.Unmarshal([]byte(value), policy); err != nil {
		return nil
	}
	return cloneWAAccountProxyPolicy(policy)
}

func validateWAAccountProxyPolicy(policy *waappv1.WAAccountProxyPolicy) error {
	if policy == nil {
		return nil
	}
	for name, stage := range map[string]*waappv1.WAProxyStagePolicy{
		"default_policy":      policy.GetDefaultPolicy(),
		"probe_policy":        policy.GetProbePolicy(),
		"registration_policy": policy.GetRegistrationPolicy(),
	} {
		if err := validateWAProxyStagePolicy(name, stage); err != nil {
			return err
		}
	}
	return nil
}

func validateWAProxyStagePolicy(name string, policy *waappv1.WAProxyStagePolicy) error {
	if policy == nil {
		return nil
	}
	switch normalizeWAProxyPolicyMode(policy.GetMode()) {
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_INHERIT,
		waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_DIRECT,
		waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_COMMON_PROXY:
		return nil
	case waappv1.WAProxyPolicyMode_WA_PROXY_POLICY_MODE_PROXY_RUNTIME_INGRESS_RULE:
		if strings.TrimSpace(policy.GetProxyRuntimeIngressRuleId()) == "" {
			return NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, name+" proxy_runtime_ingress_rule_id is required", false)
		}
		return nil
	default:
		return NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, name+" proxy mode is unsupported", false)
	}
}

func waAccountID(account *waappv1.WAAccount) string {
	return strings.TrimSpace(account.GetWaAccountId())
}

func waAccountStatus(account *waappv1.WAAccount) waappv1.WAAccountStatus {
	if account == nil {
		return waappv1.WAAccountStatus_WA_ACCOUNT_STATUS_UNSPECIFIED
	}
	return normalizeWAAccountStatus(account.GetStatus())
}

func normalizeWAAccountStatus(status waappv1.WAAccountStatus) waappv1.WAAccountStatus {
	if status != waappv1.WAAccountStatus_WA_ACCOUNT_STATUS_UNSPECIFIED {
		return status
	}
	return waappv1.WAAccountStatus_WA_ACCOUNT_STATUS_PENDING_REGISTRATION
}

func parseWAAccountStatus(value string) waappv1.WAAccountStatus {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return waappv1.WAAccountStatus_WA_ACCOUNT_STATUS_UNSPECIFIED
	}
	if !strings.HasPrefix(value, "WA_ACCOUNT_STATUS_") {
		value = "WA_ACCOUNT_STATUS_" + value
	}
	return waappv1.WAAccountStatus(waappv1.WAAccountStatus_value[value])
}

func waAccountStatusStorageValue(account *waappv1.WAAccount) string {
	return waAccountStatus(account).String()
}

func waAccountCreatedAt(account *waappv1.WAAccount) time.Time {
	return timeFromProto(account.GetAudit().GetCreatedAt())
}

func waAccountUpdatedAt(account *waappv1.WAAccount) time.Time {
	return timeFromProto(account.GetAudit().GetUpdatedAt())
}

func requireWAAccountID(value string) (string, error) {
	accountID := strings.TrimSpace(value)
	if accountID == "" {
		return "", NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "wa_account_id is required", false)
	}
	if !waAccountIDPattern.MatchString(accountID) {
		return "", NewError(waappv1.WaErrorCode_WA_ERROR_CODE_VALIDATION_FAILED, "wa_account_id must use letters, digits, colon, underscore or dash", false)
	}
	return accountID, nil
}

func requireWAAccountIDValue(value string) (string, error) {
	accountID, err := requireWAAccountID(value)
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}
	return accountID, nil
}
