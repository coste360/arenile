import { EmptyState, SectionCard } from '@arenile/shared/ui/patterns';
import { ConstructionIcon, WalletIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export interface CustomerCreditsSectionProps {
  establishmentId: string;
  customerId: string;
}

/**
 * Credit ledger of a customer (spec 13.4): balance, expiring credits,
 * movements and "Riconosci credito" (GET/POST /customers/{id}/credits).
 * Scaffold: the credits frontend builder implements it and mounts it in
 * features/customers/CustomerDetailPage.tsx in place of the M3 credits block.
 */
export function CustomerCreditsSection(_props: CustomerCreditsSectionProps) {
  const { t } = useTranslation('credits');
  return (
    <SectionCard title={t('title')} description={t('description')} icon={WalletIcon}>
      <EmptyState
        bordered={false}
        icon={ConstructionIcon}
        title={t('title')}
        description={t('description')}
      />
    </SectionCard>
  );
}
