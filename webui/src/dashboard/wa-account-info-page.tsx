import { ArrowLeft, Trash2 } from 'lucide-react';
import { Link } from 'react-router';
import type { ClientProfile, WAAccount } from '../proto/byte/v/forge/waapp/v1/profile';
import { waAccountID, waAccountTitle } from './wa-api';
import { WaAccountAvatar } from './wa-account-avatar';
import { WaAccountDetail } from './wa-account-detail';
import { waChatsPath } from './wa-route-paths';
import { Button } from './ui';

export function WaAccountInfoPage({ account, profiles, profilesLoading, busy, onDelete, onDone, onError, onAvatarChanged }: { account: WAAccount; profiles: ClientProfile[]; profilesLoading: boolean; busy: boolean; onDelete: (account: WAAccount) => void; onDone: (message: string) => void; onError: (message: string) => void; onAvatarChanged: () => void }) {
  return (
    <section className="grid h-dvh min-h-0 grid-rows-[auto_1fr] bg-background">
      <header className="flex h-16 items-center justify-between border-b border-border bg-card px-5">
        <div className="flex min-w-0 items-center gap-3">
          <Link className="inline-flex size-9 items-center justify-center rounded-lg transition hover:bg-muted" to={waChatsPath(waAccountID(account))} title="返回消息" aria-label="返回消息"><ArrowLeft size={16} /></Link>
          <WaAccountAvatar account={account} version={account.audit?.updated_at || 'latest'} size="md" />
          <div className="min-w-0"><h1 className="truncate text-base font-semibold">{waAccountTitle(account)}</h1><p className="truncate font-mono text-xs text-muted-foreground">{waAccountID(account)}</p></div>
        </div>
        <Button variant="destructive" size="icon" disabled={busy} title="删除账号" aria-label="删除账号" onClick={() => onDelete(account)}><Trash2 size={16} /></Button>
      </header>
      <main className="min-h-0 overflow-y-auto p-6">
        <div className="mx-auto max-w-5xl">
          <WaAccountDetail account={account} profiles={profiles} profilesLoading={profilesLoading} busy={busy} onDone={onDone} onError={onError} onAvatarChanged={onAvatarChanged} />
        </div>
      </main>
    </section>
  );
}
