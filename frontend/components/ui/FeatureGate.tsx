'use client';

import { useQuery } from '@tanstack/react-query';
import { ReactNode } from 'react';
import { Lock } from 'lucide-react';
import { api } from '@/lib/api';

interface FeatureGateProps {
  feature: string;
  tier?: string;
  featureLabel: string;
  children: ReactNode;
  [key: string]: any;
}

/**
 * CE FeatureGate — checks the license features returned by infrapilot.org.
 * Features unlocked by a free Community Edition key will render children.
 * Locked features show a prompt to get a free CE key.
 */
export function FeatureGate({ feature, featureLabel, children }: FeatureGateProps) {
  const { data } = useQuery({
    queryKey: ['licenseTierInfo'],
    queryFn: () => api.getLicenseTierInfo(),
    staleTime: 60 * 1000,
  });

  // Still loading — render nothing to avoid flashing gated content
  if (!data) return null;

  if (!data.features.includes(feature)) {
    return (
      <div className="flex flex-col items-center justify-center flex-1 p-12 text-center">
        <div className="w-14 h-14 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center mb-4">
          <Lock className="w-6 h-6 text-gray-400" />
        </div>
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          {featureLabel} requires a Community Edition key
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 max-w-sm mb-6">
          This feature is included in the free Community Edition license.
          Get your key and enter it in Settings → License.
        </p>
        <a
          href="https://infrapilot.org/signup"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-2 px-5 py-2.5 bg-primary-600 hover:bg-primary-700 text-white rounded-xl font-semibold text-sm transition-colors"
        >
          Get a free CE key at infrapilot.org
        </a>
      </div>
    );
  }

  return <>{children}</>;
}
