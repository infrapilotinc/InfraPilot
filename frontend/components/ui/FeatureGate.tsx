'use client';

import React from 'react';

/**
 * CE stub — all included features are always available.
 * Simply renders children without any license gating.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function FeatureGate({ children }: { children: React.ReactNode; [key: string]: any }) {
  return <>{children}</>;
}
