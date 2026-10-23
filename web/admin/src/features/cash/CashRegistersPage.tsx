import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Configurazione → Casse: CRUD of the cash registers (listCashRegisters,
 * createCashRegister, updateCashRegister, deleteCashRegister). Scaffold
 * until the cash builder implements it.
 */
export function CashRegistersPage() {
  const { t } = useTranslation('cash');
  return (
    <ScaffoldPage title={t('registersPage.title')} description={t('registersPage.description')} />
  );
}
