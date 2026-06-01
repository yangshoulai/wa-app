import { AccountAddDialog, AccountPhoneFieldList, accountPhoneSubmitDisabled } from '@byte-v-forge/common-ui';
import { createWaAccount } from './wa-api';

type WaAddAccountValues = { phone: string; country_calling_code: string };

export function WaAccountAdd({ disabled, onCreated, onError }: {
  disabled?: boolean;
  onCreated: () => void | Promise<void>;
  onError: (message: string) => void;
}) {
  return (
    <AccountAddDialog<WaAddAccountValues>
      formId="wa-add-account-form"
      title="添加 WAAccount"
      description="输入手机号和国家拨号码；服务端归一化为 WAAccount。"
      defaultValues={{ phone: '', country_calling_code: '' }}
      disabled={disabled}
      submitDisabled={accountPhoneSubmitDisabled}
      onError={onError}
      onDone={onCreated}
      onSubmit={(values) => createWaAccount({ phone: values.phone, country_calling_code: values.country_calling_code })}
    >
      {(form) => <AccountPhoneFieldList control={form.control} idPrefix="wa-add" countryPlaceholder="+1" phonePlaceholder="4155550123" />}
    </AccountAddDialog>
  );
}
