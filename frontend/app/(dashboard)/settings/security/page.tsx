"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import {
  Shield,
  Smartphone,
  Key,
  Copy,
  X,
  Loader2,
} from "lucide-react";
import { api, MFASetupResponse } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";

function MFASection() {
  const [showSetup, setShowSetup] = useState(false);
  const [showDisable, setShowDisable] = useState(false);
  const [setupData, setSetupData] = useState<MFASetupResponse | null>(null);
  const [verifyCode, setVerifyCode] = useState("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [disablePassword, setDisablePassword] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  const { data: currentUser, refetch: refetchUser } = useQuery({
    queryKey: ["currentUser"],
    queryFn: () => api.getCurrentUser(),
  });

  const mfaEnabled = currentUser?.mfa_enabled || false;

  const setupMutation = useMutation({
    mutationFn: () => api.setupMFA(),
    onSuccess: (data) => {
      setSetupData(data);
      setShowSetup(true);
      setError("");
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const confirmMutation = useMutation({
    mutationFn: (code: string) => api.confirmMFA(code),
    onSuccess: (data) => {
      setBackupCodes(data.backup_codes);
      refetchUser();
      setVerifyCode("");
      setSetupData(null);
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const disableMutation = useMutation({
    mutationFn: ({ password, code }: { password: string; code: string }) =>
      api.disableMFA(password, code),
    onSuccess: () => {
      setShowDisable(false);
      setDisablePassword("");
      setDisableCode("");
      refetchUser();
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const handleCopySecret = () => {
    if (setupData) {
      navigator.clipboard.writeText(setupData.secret);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleCopyBackupCodes = () => {
    navigator.clipboard.writeText(backupCodes.join("\n"));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className="p-2 bg-green-500/10 rounded-lg">
            <Shield className="h-5 w-5 text-green-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Two-Factor Authentication</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Add an extra layer of security to your account
            </p>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        {error && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
            {error}
          </div>
        )}

        {backupCodes.length > 0 && (
          <div className="mb-6 p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
            <div className="flex items-start gap-3 mb-3">
              <Key className="h-5 w-5 text-yellow-400 mt-0.5" />
              <div>
                <h3 className="font-medium text-yellow-300">Save Your Backup Codes</h3>
                <p className="text-sm text-yellow-400/80 mt-1">
                  These codes can be used to access your account if you lose your authenticator.
                  Each code can only be used once.
                </p>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2 mt-4 p-3 bg-gray-900 rounded font-mono text-sm">
              {backupCodes.map((code, i) => (
                <div key={i} className="text-gray-300">{code}</div>
              ))}
            </div>
            <button
              onClick={handleCopyBackupCodes}
              className="mt-3 flex items-center gap-2 text-sm text-yellow-400 hover:text-yellow-300"
            >
              <Copy className="h-4 w-4" />
              {copied ? "Copied!" : "Copy all codes"}
            </button>
            <button
              onClick={() => setBackupCodes([])}
              className="mt-3 ml-4 text-sm text-gray-400 hover:text-gray-300"
            >
              I've saved these codes
            </button>
          </div>
        )}

        {!showSetup && !showDisable && (
          <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
            <div className="flex items-center gap-3">
              <Smartphone className="h-5 w-5 text-gray-500" />
              <div>
                <p className="font-medium text-gray-900 dark:text-white">
                  Authenticator App
                </p>
                <p className="text-sm text-gray-500">
                  {mfaEnabled
                    ? "Two-factor authentication is enabled"
                    : "Not configured"}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {mfaEnabled ? (
                <>
                  <Badge className="bg-green-500/10 text-green-400 border-green-500/30">
                    Enabled
                  </Badge>
                  <button
                    onClick={() => setShowDisable(true)}
                    className="px-3 py-1.5 text-sm text-red-400 hover:text-red-300 hover:bg-red-500/10 rounded transition-colors"
                  >
                    Disable
                  </button>
                </>
              ) : (
                <button
                  onClick={() => setupMutation.mutate()}
                  disabled={setupMutation.isPending}
                  className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors text-sm"
                >
                  {setupMutation.isPending ? "Setting up..." : "Set Up MFA"}
                </button>
              )}
            </div>
          </div>
        )}

        {showSetup && setupData && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <h3 className="font-medium text-gray-900 dark:text-white">Set Up Authenticator</h3>
              <button
                onClick={() => {
                  setShowSetup(false);
                  setSetupData(null);
                  setVerifyCode("");
                }}
                className="p-1 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
              >
                <X className="h-5 w-5 text-gray-500" />
              </button>
            </div>

            <div className="grid md:grid-cols-2 gap-6">
              <div className="p-4 bg-white rounded-lg border border-gray-200 dark:border-gray-700 text-center">
                <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
                  Scan this QR code with your authenticator app
                </p>
                <div className="inline-block p-4 bg-white rounded-lg">
                  <img
                    src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(setupData.otpauth)}`}
                    alt="QR Code"
                    className="w-48 h-48"
                  />
                </div>
                <p className="text-xs text-gray-500 mt-3">
                  Works with Google Authenticator, Authy, 1Password, etc.
                </p>
              </div>

              <div className="space-y-4">
                <div>
                  <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
                    Or enter this code manually:
                  </p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 px-3 py-2 bg-gray-100 dark:bg-gray-800 rounded font-mono text-sm text-gray-900 dark:text-white break-all">
                      {setupData.secret}
                    </code>
                    <button
                      onClick={handleCopySecret}
                      className="p-2 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
                    >
                      <Copy className="h-4 w-4 text-gray-500" />
                    </button>
                  </div>
                  {copied && (
                    <p className="text-xs text-green-400 mt-1">Copied to clipboard!</p>
                  )}
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                    Enter verification code
                  </label>
                  <input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]*"
                    maxLength={6}
                    value={verifyCode}
                    onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, ""))}
                    placeholder="000000"
                    className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-center text-xl font-mono tracking-widest text-gray-900 dark:text-white"
                  />
                </div>

                <button
                  onClick={() => confirmMutation.mutate(verifyCode)}
                  disabled={verifyCode.length !== 6 || confirmMutation.isPending}
                  className="w-full px-4 py-3 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                >
                  {confirmMutation.isPending ? "Verifying..." : "Verify and Enable MFA"}
                </button>
              </div>
            </div>
          </div>
        )}

        {showDisable && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="font-medium text-gray-900 dark:text-white">Disable Two-Factor Authentication</h3>
              <button
                onClick={() => {
                  setShowDisable(false);
                  setDisablePassword("");
                  setDisableCode("");
                }}
                className="p-1 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
              >
                <X className="h-5 w-5 text-gray-500" />
              </button>
            </div>

            <div className="p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
              <p className="text-sm text-yellow-400">
                This will remove two-factor authentication from your account. You'll need to set it up again if you want to re-enable it.
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Your Password
              </label>
              <input
                type="password"
                value={disablePassword}
                onChange={(e) => setDisablePassword(e.target.value)}
                className="w-full px-4 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Verification Code
              </label>
              <input
                type="text"
                inputMode="numeric"
                pattern="[0-9]*"
                maxLength={6}
                value={disableCode}
                onChange={(e) => setDisableCode(e.target.value.replace(/\D/g, ""))}
                placeholder="000000"
                className="w-full px-4 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-center font-mono tracking-widest text-gray-900 dark:text-white"
              />
            </div>

            <button
              onClick={() => disableMutation.mutate({ password: disablePassword, code: disableCode })}
              disabled={!disablePassword || disableCode.length < 6 || disableMutation.isPending}
              className="w-full px-4 py-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
            >
              {disableMutation.isPending ? "Disabling..." : "Disable MFA"}
            </button>
          </div>
        )}
      </Card.Body>
    </Card>
  );
}

export default function SettingsSecurityPage() {
  return <MFASection />;
}
