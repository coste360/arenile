import { Button } from '@arenile/shared/ui/button';
import { ButtonGroup } from '@arenile/shared/ui/button-group';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ChevronDownIcon, ExternalLinkIcon, FileDownIcon, ReceiptTextIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { printZReport, zReportUrl, type ZPaper } from '../lib/zreport';

/**
 * "PDF chiusura Z": prints the server-rendered Z closing (A4 by default,
 * 80 mm receipt from the menu) through the browser print dialog, where the
 * user picks "Save as PDF"; the menu also opens the document in a new tab.
 */
export function ZPdfButton({
  establishmentId,
  sessionId,
  size = 'default',
}: {
  establishmentId: string;
  sessionId: string;
  size?: 'default' | 'sm';
}) {
  const { t } = useTranslation('cash');
  const [busy, setBusy] = useState(false);

  const openTab = (paper: ZPaper) => {
    window.open(zReportUrl(establishmentId, sessionId, paper), '_blank', 'noopener');
  };
  const print = async (paper: ZPaper) => {
    setBusy(true);
    try {
      await printZReport(zReportUrl(establishmentId, sessionId, paper));
    } catch {
      toast.error(t('zpdf.failed'), {
        action: { label: t('zpdf.openTab'), onClick: () => openTab(paper) },
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <ButtonGroup aria-label={t('zpdf.group')}>
      <Button variant="outline" size={size} onClick={() => void print('a4')} disabled={busy}>
        {busy ? <Spinner /> : <FileDownIcon aria-hidden="true" />}
        {t('zpdf.button')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size={size === 'sm' ? 'icon-sm' : 'icon'}
            aria-label={t('zpdf.more')}
          >
            <ChevronDownIcon aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => void print('a4')}>
            <FileDownIcon aria-hidden="true" />
            {t('zpdf.a4')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void print('receipt80')}>
            <ReceiptTextIcon aria-hidden="true" />
            {t('zpdf.receipt')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => openTab('a4')}>
            <ExternalLinkIcon aria-hidden="true" />
            {t('zpdf.openTab')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </ButtonGroup>
  );
}
