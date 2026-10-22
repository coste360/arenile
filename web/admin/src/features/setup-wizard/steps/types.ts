import type { Dispatch } from 'react';
import type { ConfigContext } from '../../configuration/api/useConfigContext';
import type { SetupStatus } from '../api';
import type { WizardAction, WizardProgress } from '../progress';

export interface StepProps {
  ctx: ConfigContext;
  progress: WizardProgress;
  dispatch: Dispatch<WizardAction>;
  status: SetupStatus | undefined;
  /** If-Match base for establishment updates made by the steps. */
  version: number;
  onVersion: (version: number) => void;
}
