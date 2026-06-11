import { Cpu, Fingerprint, Loader2, Smartphone } from 'lucide-react';
import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import type { ClientProfile, DeviceFingerprint } from '../proto/byte/v/forge/waapp/v1/profile';
import { Badge, Item, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from './ui';

export function WaDeviceFingerprintPanel({ profiles, loading }: { profiles: ClientProfile[]; loading: boolean }) {
  if (loading) return <p className="inline-flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />加载设备指纹...</p>;
  if (profiles.length === 0) return <p className="text-sm text-muted-foreground">暂无客户端 Profile。</p>;
  return <div className="grid gap-6">{profiles.map((profile) => <ProfileBlock key={profile.client_profile_id} profile={profile} />)}</div>;
}

function ProfileBlock({ profile }: { profile: ClientProfile }) {
  const fp = profile.device_fingerprint;
  return (
    <section className="grid gap-3">
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h4 className="text-sm font-semibold">{deviceTitle(fp)}</h4>
          <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{profile.client_profile_id}</p>
        </div>
        <Badge variant="outline">{profile.status || 'UNKNOWN'}</Badge>
      </header>
      {fp ? <FingerprintGrid fingerprint={fp} /> : <p className="text-sm text-muted-foreground">没有可展示的设备指纹。</p>}
    </section>
  );
}

function FingerprintGrid({ fingerprint }: { fingerprint: DeviceFingerprint }) {
  const rows: Array<{ label: string; value: string; icon: LucideIcon }> = [
    { label: '指纹 ID', value: fingerprint.fingerprint_id, icon: Fingerprint },
    { label: 'FDID', value: fingerprint.fdid, icon: Fingerprint },
    { label: 'User-Agent', value: fingerprint.user_agent, icon: Smartphone },
    { label: 'App / Android', value: [fingerprint.app_version, fingerprint.android_version].filter(Boolean).join(' / '), icon: Smartphone },
    { label: 'RAM / Radio', value: [ramLabel(fingerprint.device_ram_gib), radioLabel(fingerprint.network_radio_type)].filter(Boolean).join(' / '), icon: Cpu },
    { label: 'MCC/MNC', value: pairLabel(fingerprint.mcc, fingerprint.mnc), icon: Smartphone },
    { label: 'SIM MCC/MNC', value: pairLabel(fingerprint.sim_mcc, fingerprint.sim_mnc), icon: Smartphone },
    { label: 'Phone Hash', value: fingerprint.phone_sha256_prefix ? `${fingerprint.phone_sha256_prefix}...` : '', icon: Fingerprint },
    { label: '生成时间', value: formatTime(fingerprint.created_at), icon: Smartphone },
  ];
  return <ItemGroup className="grid gap-2 md:grid-cols-2">{rows.map(({ label, value, icon: Icon }) => <FingerprintItem key={label} icon={<Icon size={13} />} label={label} value={value || '-'} />)}</ItemGroup>;
}

function FingerprintItem({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <Item variant="muted" size="sm" className="min-w-0"><ItemMedia variant="icon">{icon}</ItemMedia><ItemContent className="min-w-0"><ItemTitle className="text-xs text-muted-foreground">{label}</ItemTitle><ItemDescription className="truncate font-mono text-xs text-foreground">{value}</ItemDescription></ItemContent></Item>;
}

function deviceTitle(fingerprint?: DeviceFingerprint) {
  if (!fingerprint) return '未知设备';
  return [fingerprint.device_vendor, fingerprint.device_model].filter(Boolean).join(' ') || '未知设备';
}

function pairLabel(a?: string, b?: string) {
  return [a, b].filter(Boolean).join('/');
}

function ramLabel(value?: string) {
  return value ? `${value} GiB` : '';
}

function radioLabel(value?: string) {
  const labels: Record<string, string> = { '1': 'GPRS', '2': 'EDGE', '3': 'UMTS', '9': 'HSDPA', '13': 'LTE', '20': 'NR' };
  return value ? labels[value] || value : '';
}

function formatTime(value?: string) {
  if (!value) return '';
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString();
}
