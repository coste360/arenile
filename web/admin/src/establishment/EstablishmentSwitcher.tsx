import { useNavigate, useParams } from '@tanstack/react-router';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useIsOwner } from './context';

const NEW_VALUE = '__new__';

/** Header select listing the accessible establishments (spec 28 M1, multi-establishment). */
export function EstablishmentSwitcher() {
  const { t } = useTranslation();
  const id = useId();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { establishmentId?: string; mapId?: string };
  const isOwner = useIsOwner();
  const current = useCurrentEstablishment();

  if (!current.isLoading && current.items.length === 0 && !isOwner) return null;

  const onChange = (value: string) => {
    if (value === NEW_VALUE) {
      void navigate({ to: '/establishments/new' });
      return;
    }
    if (params.mapId) {
      // A map document belongs to one establishment: go back to the maps list.
      void navigate({ to: '/e/$establishmentId/maps', params: { establishmentId: value } });
    } else if (params.establishmentId) {
      // Same section, other establishment.
      void navigate({ to: '.', params: { establishmentId: value } as never });
    } else {
      void navigate({
        to: '/e/$establishmentId/config/establishment',
        params: { establishmentId: value },
      });
    }
  };

  return (
    <div className="flex items-center gap-2">
      <label htmlFor={id} className="sr-only sm:not-sr-only sm:text-sm sm:text-slate-600">
        {t('admin.switcher.label')}
      </label>
      <select
        id={id}
        className="h-9 max-w-[12rem] rounded-md border border-slate-300 bg-white px-2 text-sm"
        value={current.id ?? ''}
        disabled={current.isLoading}
        onChange={(e) => onChange(e.target.value)}
      >
        {!current.id && (
          <option value="" disabled>
            {current.items.length === 0
              ? t('admin.switcher.none')
              : t('admin.switcher.placeholder')}
          </option>
        )}
        {current.items.map((e) => (
          <option key={e.id} value={e.id}>
            {e.name}
          </option>
        ))}
        {isOwner && <option value={NEW_VALUE}>{t('admin.switcher.new')}</option>}
      </select>
    </div>
  );
}
