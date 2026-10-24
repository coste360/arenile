import { Button } from '@arenile/shared/ui/button';
import { SectionCard } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { ArrowRightIcon, PrinterIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';

/**
 * Pointer from the desktop settings to Configurazione → Stampanti: the
 * thermal network printers are configured per establishment (ADR 0021).
 */
export function PrintersHint() {
  const { t } = useTranslation('printers');
  const establishmentId = useRequiredEstablishmentId();
  return (
    <SectionCard
      title={t('hint.title')}
      titleId="printers-hint-title"
      icon={PrinterIcon}
      description={t('hint.description')}
      actions={
        <Button variant="outline" size="sm" asChild>
          <Link to="/e/$establishmentId/config/printers" params={{ establishmentId }}>
            {t('hint.action')}
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        </Button>
      }
    />
  );
}
