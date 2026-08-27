"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";

export default function SetupPage() {
  const router = useRouter();
  const { setTokens } = useAuthStore();

  const [ready, setReady] = useState(false);
  // A license is now required to complete setup (v3/36) — step 1 (license) always
  // comes first, and there's no way to skip to step 2 without one.
  const [step, setStep] = useState<1 | 2>(1);

  // Required by both steps below — closes the "whoever visits /setup first becomes
  // admin" race. Printed to the container's logs at startup, only someone with actual
  // access to the host (not a remote attacker) can read it.
  const [setupToken, setSetupToken] = useState("");

  // Step 1: license — two ways in, toggled with licenseMode.
  const [licenseMode, setLicenseMode] = useState<"signup" | "paste">("signup");

  // Step 1a: paste an existing key
  const [licenseKey, setLicenseKey] = useState("");
  const [licenseError, setLicenseError] = useState("");
  const [licenseLoading, setLicenseLoading] = useState(false);

  // Step 1b: in-app email sign-up for a free key — no tab-switching, no copy-paste.
  // "idle" = enter email; "pending" = check your inbox, polling in the background;
  // "verified" = key issued and already saved, about to move on to step 2.
  const [signupEmail, setSignupEmail] = useState("");
  const [signupStatus, setSignupStatus] = useState<"idle" | "pending" | "verified">("idle");
  const [signupError, setSignupError] = useState("");
  const [signupLoading, setSignupLoading] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Step 2: admin account
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [adminError, setAdminError] = useState("");
  const [adminLoading, setAdminLoading] = useState(false);

  // On mount: redirect away if setup already done. A license already configured (e.g.
  // LICENSE_KEY env var, or a prior partial setup) skips straight to admin creation.
  useEffect(() => {
    api.getSetupStatus().then((status) => {
      if (!status.setup_required) {
        router.replace("/login");
        return;
      }
      if (status.license_error) setLicenseError(status.license_error);
      setStep(status.license_configured ? 2 : 1);
      setReady(true);
    }).catch(() => {
      setReady(true);
    });
  }, [router]);

  // Stop polling if the component unmounts mid-signup.
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

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

  const handleSignupSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSignupError("");
    setSignupLoading(true);
    try {
      await api.communitySignup(signupEmail.trim());
      setSignupStatus("pending");
      pollRef.current = setInterval(async () => {
        try {
          const result = await api.communitySignupStatus(signupEmail.trim());
          if (result.status === "verified") {
            if (pollRef.current) clearInterval(pollRef.current);
            setSignupStatus("verified");
            setTimeout(() => setStep(2), 1200);
          }
        } catch {
          // Transient poll failure — keep trying, the interval will retry in 3s.
        }
      }, 3000);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to start sign-up";
      setSignupError(message);
    } finally {
      setSignupLoading(false);
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
      const result = await api.createInitialAdmin(email, password, setupToken.trim());
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
                ? "Activate a free Community key to get started"
                : "Create your admin account"}
            </p>
          </div>

          {/* Step 1: License */}
          {step === 1 && (
            <div className="space-y-5">
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setLicenseMode("signup")}
                  className={`flex-1 py-2 px-3 rounded-lg text-sm font-medium transition-colors ${
                    licenseMode === "signup"
                      ? "bg-primary-600 text-white"
                      : "bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400"
                  }`}
                >
                  Get a free key
                </button>
                <button
                  type="button"
                  onClick={() => setLicenseMode("paste")}
                  className={`flex-1 py-2 px-3 rounded-lg text-sm font-medium transition-colors ${
                    licenseMode === "paste"
                      ? "bg-primary-600 text-white"
                      : "bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400"
                  }`}
                >
                  I have a key
                </button>
              </div>

              {licenseMode === "signup" ? (
                <form onSubmit={handleSignupSubmit} className="space-y-5">
                  {signupError && (
                    <div className="bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm">
                      {signupError}
                    </div>
                  )}

                  {signupStatus === "idle" && (
                    <>
                      <div>
                        <label
                          htmlFor="signupEmail"
                          className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                        >
                          Email Address
                        </label>
                        <input
                          id="signupEmail"
                          type="email"
                          value={signupEmail}
                          onChange={(e) => setSignupEmail(e.target.value)}
                          required
                          autoFocus
                          className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500"
                          placeholder="you@example.com"
                        />
                        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                          No credit card. We&apos;ll email you a free Community key and check
                          back here automatically once it&apos;s ready.
                        </p>
                      </div>
                      <button
                        type="submit"
                        disabled={signupLoading}
                        className="w-full py-3 px-4 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-800 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
                      >
                        {signupLoading ? "Sending..." : "Send Verification Email →"}
                      </button>
                    </>
                  )}

                  {signupStatus === "pending" && (
                    <div className="text-center space-y-3 py-4">
                      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500 mx-auto" />
                      <p className="text-sm text-gray-700 dark:text-gray-300">
                        Check <strong>{signupEmail}</strong> and click the verification link.
                      </p>
                      <p className="text-xs text-gray-500 dark:text-gray-400">
                        This page will continue automatically once verified — no need to
                        come back here.
                      </p>
                    </div>
                  )}

                  {signupStatus === "verified" && (
                    <div className="bg-green-500/10 border border-green-500/50 text-green-700 dark:text-green-400 px-4 py-3 rounded text-sm text-center">
                      Verified! Your Community key is active — continuing...
                    </div>
                  )}
                </form>
              ) : (
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
                </form>
              )}
            </div>
          )}

          {/* Step 2: Admin Account */}
          {step === 2 && (
            <form onSubmit={handleAdminSubmit} className="space-y-5">
              {adminError && (
                <div className="bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm">
                  {adminError}
                </div>
              )}

              {/* Setup token — closes the remote "whoever visits /setup first becomes
                  admin" race. Only needed here, since creating the admin account is the
                  actual privilege boundary; activating a license grants no access. */}
              <div>
                <label
                  htmlFor="setupToken"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
                >
                  Setup Token
                </label>
                <input
                  id="setupToken"
                  type="text"
                  value={setupToken}
                  onChange={(e) => setSetupToken(e.target.value)}
                  required
                  className="w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 font-mono text-sm"
                  placeholder="Paste the token"
                />
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Find it with{" "}
                  <code className="font-mono">docker exec &lt;container&gt; cat /data/setup_token</code>
                  {" "}(not <code className="font-mono">docker logs</code> — the backend&apos;s
                  own output goes to a log file, not the container&apos;s stdout).
                </p>
              </div>

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

              {/* First-run telemetry disclosure (v3/40 §2 privacy contract) */}
              <p className="text-center text-xs text-gray-500 dark:text-gray-400">
                InfraPilot sends anonymous, non-identifying usage telemetry by default — never
                app names, repo URLs, env vars, or your infrastructure&apos;s content. Turn it
                off anytime in Settings → Privacy.
              </p>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
