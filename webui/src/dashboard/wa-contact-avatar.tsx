import { useEffect, useState } from 'react';
import type { WaContact } from './wa-chat-model';
import { WhatsAppIcon } from './wa-brand-icon';

type ContactAvatarSize = 'sm' | 'md';

export function WaContactAvatar({ contact, size = 'md' }: { contact?: WaContact; size?: ContactAvatarSize }) {
  const [failedURL, setFailedURL] = useState('');
  const pictureURL = contact?.profilePictureURL || '';

  useEffect(() => {
    setFailedURL('');
  }, [pictureURL]);

  const sizeClass = size === 'sm' ? 'size-9' : 'size-10';
  const iconClass = size === 'sm' ? 'size-6' : 'size-7';
  const title = contact?.title || '联系人';
  if (pictureURL && failedURL !== pictureURL) {
    return <img className={`${sizeClass} rounded-full object-cover`} src={pictureURL} alt={title} loading="lazy" onError={() => setFailedURL(pictureURL)} />;
  }
  return <span className={`grid ${sizeClass} place-items-center rounded-full bg-emerald-50`}><WhatsAppIcon className={iconClass} title={title} /></span>;
}
