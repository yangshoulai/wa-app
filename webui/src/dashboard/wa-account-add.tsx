import { useEffect, useState } from 'react';
import { CheckCircle2, KeyRound, Search, ShieldAlert } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { probeWaPhoneSMS, registerWaPhone, submitWaRegistrationOTP, type WaWorkflowResponse } from './wa-api';
import { WhatsAppIcon } from './wa-brand-icon';
import { accountReasonLabel } from './wa-result-labels';
import { waProbeStatus } from './wa-result-model';
import { WaRegistrationChannelButtons } from './wa-registration-channel-buttons';
import { WaRegistrationOtpCard } from './wa-registration-otp-card';
import { registrationAnyMethodAvailable, registrationChannelsHardBlocked, type SelectableRegistrationMethodOption } from './wa-registration-methods';
import { WaResultPanel } from './wa-result-panel';
import { resolveWaPhoneTarget, type WaResolvedPhone } from './wa-utils';
type ProbeState = { target: WaResolvedPhone; result: WaWorkflowResponse } | null;
type PendingRegistration = { target: WaResolvedPhone; accountID: string; verificationRequestID: string };
type Props = { disabled?: boolean; onChanged: () => void | Promise<void>; onDone: (message: string) => void; onError: (message: string) => void };
export function WaAccountAdd({ disabled, onChanged, onDone, onError }: Props) {
  const [phone, setPhone] = useState('');
  const [countryCallingCode, setCountryCallingCode] = useState('');
  const [probe, setProbe] = useState<ProbeState>(null);
  const [pending, setPending] = useState<PendingRegistration | null>(null);
  const [registrationResult, setRegistrationResult] = useState<WaWorkflowResponse | null>(null);
  const [registrationTarget, setRegistrationTarget] = useState<WaResolvedPhone | null>(null);
  const [cooldownStartedAt, setCooldownStartedAt] = useState(Date.now());
  const [clockNow, setClockNow] = useState(Date.now());
  const [otp, setOtp] = useState('');
  const [busy, setBusy] = useState(false);
  const samePhone = probeMatchesValues(probe, phone, countryCallingCode);
  const currentTarget = resolveWaPhoneTarget(phone, countryCallingCode).target;
  const registrationSamePhone = Boolean(registrationTarget && currentTarget?.e164 === registrationTarget.e164);
  const activeRegistrationResult = registrationSamePhone ? registrationResult : null;
  const status = waProbeStatus(activeRegistrationResult || (samePhone ? probe?.result : null));
  const channelStatus = samePhone ? waProbeStatus(activeRegistrationResult || probe?.result) : null;
  const cooldownElapsedSeconds = Math.max(0, (clockNow - cooldownStartedAt) / 1000);
  const blocked = status.blocked === true;
  const showChannels = Boolean(channelStatus && !pending);
  const channelsHardBlocked = registrationChannelsHardBlocked(channelStatus);
  const canRegister = samePhone && registrationAnyMethodAvailable(channelStatus, cooldownElapsedSeconds) && !channelsHardBlocked;
  const showHelp = pending || blocked || canRegister || Boolean(activeRegistrationResult);
  const badgeVariant = pending ? 'default' : blocked ? 'destructive' : canRegister ? 'default' : 'outline';
  const badgeLabel = pending ? '等待 OTP' : blocked ? '已封禁' : canRegister ? '可注册' : '待检测';

  useEffect(() => {
    const activeResult = activeRegistrationResult || (samePhone ? probe?.result : null);
    if (!activeResult) return undefined;
    const timer = window.setInterval(() => setClockNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [activeRegistrationResult, probe, samePhone]);

  async function runProbe() {
    const resolved = resolveWaPhoneTarget(phone, countryCallingCode);
    if (!resolved.target) return onError(resolved.error || '请输入手机号和国家拨号码。');
    setBusy(true);
    try {
      setRegistrationResult(null);
      setRegistrationTarget(null);
      setPending(null);
      const result = await probeWaPhoneSMS(resolved.target.input);
      resetCooldownClock();
      setProbe({ target: resolved.target, result });
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  async function submitOTP() {
    if (!pending) return onError('没有等待中的注册 OTP。');
    const code = otp.trim();
    if (!code) return onError('请输入 OTP。');
    setBusy(true);
    try {
      const result = await submitWaRegistrationOTP(pending.accountID, code);
      if (result.success === false || result.error_message) throw new Error(accountReasonLabel(result.error_message, result.status) || 'OTP 提交失败');
      setOtp('');
      setPending(null);
      onDone('OTP 已提交');
      await onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  async function startRegistration(method: SelectableRegistrationMethodOption) {
    const resolved = resolveWaPhoneTarget(phone, countryCallingCode);
    if (!resolved.target) return onError(resolved.error || '请输入手机号和国家拨号码。');
    if (!samePhone || !channelStatus) return onError('请先完成检测，再选择可用通道发起注册。');
    setBusy(true);
    try {
      const result = await registerWaPhone(resolved.target.input, method.value);
      const resultStatus = waProbeStatus(result);
      resetCooldownClock();
      setRegistrationResult(result);
      setRegistrationTarget(resolved.target);
      if (result.success === false || result.error_message || resultStatus.blocked === true || resultStatus.requestFailed) {
        onError(registrationFailureMessage(result, resultStatus));
        return;
      }
      const accountID = workflowText(result, 'wa_account_id');
      if (accountID) setPending({ target: resolved.target, accountID, verificationRequestID: workflowText(result, 'verification_request_id') });
      setProbe(null);
      setOtp('');
      onDone(accountID ? `${method.label} OTP 已发送，请输入验证码` : `${method.label} 注册流程已发起`);
      await onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  function resetCooldownClock() {
    const now = Date.now();
    setCooldownStartedAt(now);
    setClockNow(now);
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="grid gap-1">
          <CardTitle className="inline-flex items-center gap-2 text-base"><WhatsAppIcon className="size-5" />添加并注册 WAAccount</CardTitle>
          <CardDescription>先检测手机号/SMS 状态；检测通过后才发起注册并持久化账号。</CardDescription>
        </div>
        <Badge variant={badgeVariant}>
          {pending ? <KeyRound size={12} /> : canRegister ? <CheckCircle2 size={12} /> : null}
          {badgeLabel}
        </Badge>
      </CardHeader>
      <CardContent className="grid gap-3">
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-[160px_1fr]">
            <Field><FieldLabel>国家拨号码</FieldLabel><Input placeholder="+1" value={countryCallingCode} onChange={(event) => setCountryCallingCode(event.target.value)} disabled={busy || disabled} /></Field>
            <Field>
              <FieldLabel>手机号</FieldLabel>
              <div className="flex gap-2">
                <Input placeholder="4155550123" value={phone} onChange={(event) => setPhone(event.target.value)} disabled={busy || disabled} />
                <Button type="button" size="icon" variant="outline" disabled={busy || disabled} title="检测手机号" aria-label="检测手机号" onClick={() => void runProbe()}><Search size={14} /></Button>
              </div>
            </Field>
          </div>
          {probe && !samePhone && <Badge variant="outline">号码已变化，请重新检测</Badge>}
        </FieldGroup>
        {showChannels && (
          <Field>
            <FieldLabel>注册 / 登录通道</FieldLabel>
            <WaRegistrationChannelButtons status={channelStatus} elapsedSeconds={cooldownElapsedSeconds} disabled={busy || disabled || Boolean(pending) || channelsHardBlocked} onStart={(method) => void startRegistration(method)} />
            <FieldDescription>检测后只显示当前可用通道；冷却中的通道会显示倒计时。</FieldDescription>
          </Field>
        )}
        {showHelp && <Alert variant={blocked ? 'destructive' : 'default'}>
          {blocked && <ShieldAlert className="size-4" />}
          {blocked && <AlertTitle>号码被拒绝/封禁</AlertTitle>}
          <AlertDescription>{registrationHelp(Boolean(pending), canRegister, Boolean(activeRegistrationResult), blocked)}</AlertDescription>
        </Alert>}
        {pending && <WaRegistrationOtpCard phone={pending.target.e164} verificationRequestID={pending.verificationRequestID} value={otp} busy={busy} onChange={setOtp} onSubmit={() => void submitOTP()} />}
        {(activeRegistrationResult || probe || busy) && (
          <Card className="p-3">
            <WaResultPanel title={activeRegistrationResult ? '注册结果' : '检测结果'} phone={registrationSamePhone ? registrationTarget?.e164 || '' : samePhone ? probe?.target.e164 || '' : ''} result={activeRegistrationResult || (samePhone ? probe?.result || null : null)} loading={busy} showMethods={!showChannels} />
          </Card>
        )}
      </CardContent>
    </Card>
  );
}
function registrationHelp(pending: boolean, canRegister: boolean, hasRegistrationResult: boolean, blocked: boolean) {
  if (blocked) return 'WA 已拒绝该号码；请停止重复请求，建议更换号码或注册通道。';
  if (pending) return 'OTP 已发送，请在本页输入验证码完成注册。';
  if (canRegister && hasRegistrationResult) return '当前通道已返回结果；仍有其他可用通道，可直接继续尝试。';
  if (canRegister) return '检测通过，可以点击可用通道发起注册。';
  if (hasRegistrationResult) return '本次注册请求已返回结果，可重新检测后再继续。';
  return '检测通过前不会持久化 WAAccount。';
}
function probeMatchesValues(probe: ProbeState, phone: string, countryCallingCode: string) {
  if (!probe) return false;
  return resolveWaPhoneTarget(phone, countryCallingCode).target?.e164 === probe.target.e164;
}
function workflowText(result: WaWorkflowResponse, key: keyof WaWorkflowResponse) {
  const value = result[key];
  return typeof value === 'string' ? value.trim() : '';
}
function registrationFailureMessage(result: WaWorkflowResponse, status: ReturnType<typeof waProbeStatus>) {
  const detail = status.failureReason || result.error_message || result.status || '';
  const reason = accountReasonLabel(detail);
  if (status.blocked) return '号码被 WA 拒绝/封禁，先停止重试，建议更换号码或注册通道。';
  if (status.accountFlow === 'invalid_number') return reason || '号码格式被 WA 拒绝，请检查国家拨号码和手机号。';
  if (status.accountFlow === 'rate_limited') return reason || 'WA 注册请求处于冷却，请稍后再试。';
  return reason || 'WA 注册流程发起失败';
}
