import { describe, expect, it } from 'vitest';
import { ApiProblem } from './client';
import { problemMessageKey } from './problem';

describe('problemMessageKey', () => {
  it.each([
    ['conflict', 'common.errors.conflict'],
    ['concurrent_update', 'common.errors.concurrentUpdate'],
    ['approval_required', 'common.errors.approvalRequired'],
    ['internal_error', 'common.errors.generic'],
  ] as const)('%s → %s', (type, key) => {
    expect(problemMessageKey(new ApiProblem({ type, status: 409, title: 'T' }))).toBe(key);
  });
});
