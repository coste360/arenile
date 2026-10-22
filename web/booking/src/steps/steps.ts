export const STEPS = ['when', 'where', 'what', 'who'] as const;
export type Step = (typeof STEPS)[number];
