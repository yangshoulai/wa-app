import type { ReactNode } from 'react';
import { AccountPhoneProbeForm } from '@byte-v-forge/common-ui';
import { resolveWaPhoneTarget, type WaResolvedPhone } from './wa-utils';

export function WaPhoneSMSProbeForm({ disabled, resultSlot, onCheck, onError }: {
  disabled?: boolean;
  resultSlot?: ReactNode;
  onCheck: (target: WaResolvedPhone) => void | Promise<void>;
  onError: (message: string) => void;
}) {
  return (
    <AccountPhoneProbeForm
      title="手机号/SMS 探测"
      disabled={disabled}
      resultSlot={resultSlot}
      emptyResultText="结果：注册 / SMS / Blocked"
      countryPlaceholder="+992"
      phonePlaceholder="007886231"
      actionLabel="探测手机号和 SMS 状态"
      resolve={(values) => resolveWaPhoneTarget(values.phone, values.country_calling_code)}
      onSubmit={onCheck}
      onError={onError}
    />
  );
}
