// Public surface of the test-mode feature (spec 15.4, ADR 0019): the router
// and the shell import ONLY these names. The test-mode builder owns
// everything inside features/test-mode/ (including the shell banner).
export { TestModePage } from './TestModePage';
