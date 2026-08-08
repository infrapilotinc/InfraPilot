"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";

export default function SetupPage() {
  const router = useRouter();
  const { setTokens } = useAuthStore();

  const [ready, setReady] = useState(false);
  // Keyless by default (doc 36 §7): 2 = admin account is the primary step; 1 = optional license activation.
  const [step, setStep] = useState<1 | 2>(2);

  // Step 1: license
  const [licenseKey, setLicenseKey] = useState("");
  const [licenseError, setLicenseError] = useState("");
  const [licenseLoading, setLicenseLoading] = useState(false);

  // Step 2: admin account
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [adminError, setAdminError] = useState("");
  const [adminLoading, setAdminLoading] = useState(false);

  // On mount: redirect away if setup already done; skip license step if key already active
  useEffect(() => {
    api.getSetupStatus().then((status) => {
      if (!status.setup_required) {
        router.replace("/login");
        return;
      }
      // Keyless by default: always land on admin-account creation. A license is
      // optional — activate it here (link) or later in Settings → License.
      if (status.license_error) setLicenseError(status.license_error);
      setStep(2);
      setReady(true);
    }).catch(() => {
      setReady(true);
    });
  }, [router]);

  const handleLicenseSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLicenseError("");
    setLicenseLoading(true);
    try {
      await api.setupLicense(licenseKey.trim());
      setStep(2);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Invalid license key";
      setLicenseError(message);
    } finally {
      setLicenseLoading(false);
    }
  };

  const handleAdminSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAdminError("");

    if (password !== confirmPassword) {
      setAdminError("Passwords do not match");
      return;
    }
    if (password.length < 8) {
      setAdminError("Password must be at least 8 characters");
      return;
    }

    setAdminLoading(true);
    try {
      const result = await api.createInitialAdmin(email, password);
      if (result.access_token && result.refresh_token) {
        setTokens(result.access_token, result.refresh_token);
        router.push("/");
      } else {
        router.push("/login");
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to create account";
      setAdminError(message);
    } finally {
      setAdminLoading(false);
    }
  };

  if (!ready) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-100 dark:bg-gray-950">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-100 dark:bg-gray-950">
      <div className="w-full max-w-md">
        <div className="bg-white dark:bg-gray-900 rounded-lg shadow-xl p-8">
          {/* Header */}
          <div className="text-center mb-8">
            <div className="mx-auto w-16 h-16 bg-primary-100 dark:bg-primary-900/30 rounded-full flex items-center justify-center mb-4">
              <img src="/logo.svg" alt="InfraPilot" className="h-10 w-10" />
            </div>
            <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
              Welcome to InfraPilot
            </h1>
            <span className="inline-block mt-1 px-2 py-0.5 text-xs font-medium bg-primary-100 dark:bg-primary-900/40 text-primary-700 dark:text-primary-300 rounded-full">
              Community Edition
            </span>
            <p className="text-gray-600 dark:text-gray-400 mt-2">
              {step === 1
                ? "Activate a license (optional)"
                : "Create your admin account"}
            </p>
          </div>

          {/* Step 1: License Key */}
          {step === 1 && (
            <form onSubmit={handleLicenseSubmit} className="space-y-5">
              {licenseError && (
                <div className="bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm">
                  {licenseError}
                </div>
              )}

              <div>
                <label
                  htmlFor="licenseKey"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                >
                  License Key
                </label>
                <input
                  id="licenseKey"
                  type="text"
                  value={licenseKey}
                  onChange={(e) => setLicenseKey(e.target.value)}
                  required
                  autoFocus
                  className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 font-mono"
                  placeholder="IP-CE-XXXX-XXXX-XXXX"
                />
              </div>

              <button
                type="submit"
                disabled={licenseLoading}
                className="w-full py-3 px-4 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-800 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
              >
                {licenseLoading ? "Validating..." : "Activate License →"}
              </button>

              <button
                type="button"
                onClick={() => { setLicenseError(""); setStep(2); }}
                className="w-full text-center text-sm text-gray-500 dark:text-gray-400 hover:text-primary-600 dark:hover:text-primary-400 transition-colors"
              >
                ← Skip — continue with Community Edition
              </button>
            </form>
          )}

          {/* Step 2: Admin Account */}
          {step === 2 && (
            <form onSubmit={handleAdminSubmit} className="space-y-5">
              {adminError && (
                <div className="bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm">
                  {adminError}
                </div>
              )}

              <div>
                <label
                  htmlFor="email"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                >
                  Email Address
                </label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  autoFocus
                  className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500"
                  placeholder="admin@example.com"
                />
              </div>

              <div>
                <label
                  htmlFor="password"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                >
                  Password
                </label>
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                  className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500"
                  placeholder="At least 8 characters"
                />
              </div>

              <div>
                <label
                  htmlFor="confirmPassword"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                >
                  Confirm Password
                </label>
                <input
                  id="confirmPassword"
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  minLength={8}
                  className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500"
                  placeholder="Confirm your password"
                />
              </div>

              <button
                type="submit"
                disabled={adminLoading}
                className="w-full py-3 px-4 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-800 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
              >
                {adminLoading ? "Creating Account..." : "Create Admin Account →"}
              </button>

              <p className="text-center text-xs text-gray-500 dark:text-gray-400">
                Have an Enterprise license?{" "}
                <button
                  type="button"
                  onClick={() => setStep(1)}
                  className="text-primary-600 dark:text-primary-400 hover:underline"
                >
                  Activate it
                </button>
              </p>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
