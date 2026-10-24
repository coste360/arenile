// Public surface of the test-mode feature (spec 15.4, ADR 0019): the router
// and the shell import ONLY these names. The test-mode builder owns
// everything inside features/test-mode/. The shell banner is imported from
// ./TestModeBanner directly so that the lazily loaded page stays out of the
// main chunk.
export { TestModePage } from './TestModePage';
