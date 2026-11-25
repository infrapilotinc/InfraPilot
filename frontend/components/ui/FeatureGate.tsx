'use client';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, ReactNode, useState } from 'react';
import { Lock, CheckCircle2 } from 'lucide-react';
import { api } from '@/lib/api';

interface FeatureGateProps {
  feature: string;
  tier?: string;
  featureLabel: string;
  children: ReactNode;
  [key: string]: any;
}

// Features a free Community key unlocks (v3/35 "connected" bundle). Keep in sync with
// CONNECTED_FREE in infrapilot.org packages/license and ConnectedFreeFeatures() in
// the backend. For these the gate offers the in-app email sign-up instead of a link.
const CONNECTED_FEATURES = ['webhooks', 'chat_notifications', 'email_alerts'];

/**
 * CE FeatureGate — checks the license features returned by infrapilot.org.
 * Keyless CE is the full single-node platform; a free Community key adds the
 * connected features (alert channels), which this gate unlocks in place.
 */
export function FeatureGate({ feature, featureLabel, children }: FeatureGateProps) {
  const { data } = useQuery({
    queryKey: ['licenseTierInfo'],
    queryFn: () => api.getLicenseTierInfo(),
    staleTime: 60 * 1000,
  });

  // Still loading or errored — render nothing to avoid flashing gated content
  if (!data) return null;

  if (data.features?.includes(feature)) return <>{children}</>;

  if (CONNECTED_FEATURES.includes(feature)) {
    return <ConnectAccountPrompt featureLabel={featureLabel} />;
  }

  return (
    <div className="flex flex-col items-center justify-center flex-1 p-12 text-center">
      <div className="w-14 h-14 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center mb-4">
        <Lock className="w-6 h-6 text-gray-400" />
      </div>
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
        {featureLabel} needs a license key
      </h2>
      <p className="text-sm text-gray-500 dark:text-gray-400 max-w-sm mb-6">
        Enter a key in Settings → License to unlock this feature.
      </p>
      <a
        href="/settings/license"
        className="inline-flex items-center gap-2 px-5 py-2.5 bg-primary-600 hover:bg-primary-700 text-white rounded-xl font-semibold text-sm transition-colors"
      >
        Go to Settings → License
      </a>
    </div>
  );
}

const inputClass =
  'w-full px-4 py-2.5 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500';
const buttonClass =
  'w-full py-2.5 px-4 bg-primary-600 hover:bg-primary-700 disabled:opacity-60 disabled:cursor-not-allowed text-white font-semibold rounded-lg text-sm transition-colors';

/**
 * In-app free account connect: email, then the 6-digit code, never leaving the page.
 * On success the backend swaps the key in live, so refetching the license unlocks
 * the gated page immediately.
 */
function ConnectAccountPrompt({ featureLabel }: { featureLabel: string }) {
  const queryClient = useQueryClient();
  const [stage, setStage] = useState<'email' | 'code' | 'done'>('email');
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const run = async (fn: () => Promise<void>) => {
    setError('');
    setLoading(true);
    try {
      await fn();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Something went wrong');
    } finally {
      setLoading(false);
    }
  };

  const sendCode = (e?: FormEvent) => {
    e?.preventDefault();
    return run(async () => {
      await api.connectAccount(email.trim());
      setCode('');
      setStage('code');
    });
  };

  const verify = (e: FormEvent) => {
    e.preventDefault();
    return run(async () => {
      await api.connectAccountVerify(email.trim(), code.trim());
      setStage('done');
      await queryClient.invalidateQueries({ queryKey: ['licenseTierInfo'] });
      await queryClient.invalidateQueries({ queryKey: ['licenseSettings'] });
    });
  };

  return (
    <div className="flex flex-col items-center justify-center flex-1 p-12 text-center">
      <div className="w-full max-w-sm">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          Connect a free account to use {featureLabel}
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">
          Sending alerts by email, Slack, Discord, Teams or webhook needs a free
          InfraPilot account. No credit card, just your email.
        </p>

        {error && (
          <div className="mb-4 bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm text-left">
            {error}
          </div>
        )}

        {stage === 'email' && (
          <form onSubmit={sendCode} className="space-y-3 text-left">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
              className={inputClass}
              placeholder="you@example.com"
              aria-label="Email address"
            />
            <button type="submit" disabled={loading} className={buttonClass}>
              {loading ? 'Sending...' : 'Email me a code'}
            </button>
          </form>
        )}

        {stage === 'code' && (
          <form onSubmit={verify} className="space-y-3 text-left">
            <input
              type="text"
              inputMode="numeric"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
              required
              autoFocus
              className={`${inputClass} font-mono text-center text-2xl tracking-[0.3em]`}
              placeholder="000000"
              aria-label="Verification code"
            />
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Sent to <strong>{email}</strong>. Expires in 10 minutes.
            </p>
            <button type="submit" disabled={loading || code.length !== 6} className={buttonClass}>
              {loading ? 'Verifying...' : 'Verify and connect'}
            </button>
            <div className="flex justify-between text-sm">
              <button
                type="button"
                onClick={() => setStage('email')}
                className="text-gray-500 dark:text-gray-400 hover:underline"
              >
                Change email
              </button>
              <button
                type="button"
                onClick={() => sendCode()}
                disabled={loading}
                className="text-primary-600 dark:text-primary-400 hover:underline disabled:opacity-50"
              >
                Resend code
              </button>
            </div>
          </form>
        )}

        {stage === 'done' && (
          <div className="flex items-center justify-center gap-2 bg-green-500/10 border border-green-500/50 text-green-700 dark:text-green-400 px-4 py-3 rounded text-sm">
            <CheckCircle2 className="w-4 h-4" />
            Connected. Loading {featureLabel}...
          </div>
        )}
      </div>
    </div>
  );
}
