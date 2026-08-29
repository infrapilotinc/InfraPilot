"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";

const inputClass =
  "w-full px-4 py-3 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500";
const labelClass = "block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2";
const buttonClass =
  "w-full py-3 px-4 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-800 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors";
const errorBoxClass =
  "bg-red-500/10 border border-red-500/50 text-red-600 dark:text-red-400 px-4 py-3 rounded text-sm";

export default function SetupPage() {
  const router = useRouter();
  const { setTokens } = useAuthStore();

  const [ready, setReady] = useState(false);
  // A license is required to complete setup (v3/36), and the setup token is required
  // before anything else on this page can run at all -- so the wizard is now three
  // steps, always in order: token, license, admin account.
  const [step, setStep] = useState<1 | 2 | 3>(1);

  // Step 1: setup token — closes the "whoever visits /setup first becomes admin" race.
  // Collected once here and carried through every later call.
  const [setupToken, setSetupToken] = useState("");
  const [tokenInput, setTokenInput] = useState("");
  const [tokenError, setTokenError] = useState("");
  const [tokenLoading, setTokenLoading] = useState(false);

  // Step 2: license — two ways in, toggled with licenseMode.
  const [licenseMode, setLicenseMode] = useState<"signup" | "paste">("signup");

  // Step 2a: paste an existing key
  const [licenseKey, setLicenseKey] = useState("");
  const [licenseError, setLicenseError] = useState("");
  const [licenseLoading, setLicenseLoading] = useState(false);

  // Step 2b: in-app email sign-up for a free key — enter email, get a 6-digit code,
  // type it in. No tab-switching, no polling.
  const [otpStage, setOtpStage] = useState<"email" | "code">("email");
  const [signupEmail, setSignupEmail] = useState("");
  const [signupError, setSignupError] = useState("");
  const [signupLoading, setSignupLoading] = useState(false);
  const [otpCode, setOtpCode] = useState("");
  const [otpError, setOtpError] = useState("");
  const [otpLoading, setOtpLoading] = useState(false);
  const [otpVerified, setOtpVerified] = useState(false);
  const [resendLoading, setResendLoading] = useState(false);

  // Step 3: admin account
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [adminError, setAdminError] = useState("");
  const [adminLoading, setAdminLoading] = useState(false);

  // On mount: redirect away if setup already done. A license already configured (e.g.
  // LICENSE_KEY env var, or a prior partial setup) skips straight to admin creation --
  // that step will still ask for the token, since it's not carried across a refresh.
  useEffect(() => {
    api.getSetupStatus().then((status) => {
      if (!status.setup_required) {
        router.replace("/login");
        return;
      }
      if (status.license_error) setLicenseError(status.license_error);
      setStep(status.license_configured ? 3 : 1);
      setReady(true);
    }).catch(() => {
      setReady(true);
    });
  }, [router]);

  const handleTokenSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setTokenError("");
    setTokenLoading(true);
    try {
      await api.verifySetupToken(tokenInput.trim());
      setSetupToken(tokenInput.trim());
      setStep(2);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Invalid setup token";
      setTokenError(message);
    } finally {
      setTokenLoading(false);
    }
  };

  const handleLicenseSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLicenseError("");
    setLicenseLoading(true);
    try {
      await api.setupLicense(licenseKey.trim(), setupToken);
      setStep(3);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Invalid license key";
      setLicenseError(message);
    } finally {
      setLicenseLoading(false);
    }
  };

  const sendCode = async () => {
    await api.communitySignup(signupEmail.trim(), setupToken);
    setOtpStage("code");
    setOtpCode("");
    setOtpError("");
  };

  const handleSendCode = async (e: React.FormEvent) => {
    e.preventDefault();
    setSignupError("");
    setSignupLoading(true);
    try {
      await sendCode();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to send code";
      setSignupError(message);
    } finally {
      setSignupLoading(false);
    }
  };

  const handleResendCode = async () => {
    setOtpError("");
    setResendLoading(true);
    try {
      await sendCode();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to resend code";
      setOtpError(message);
    } finally {
      setResendLoading(false);
    }
  };

  const handleVerifyCode = async (e: React.FormEvent) => {
    e.preventDefault();
    setOtpError("");
    setOtpLoading(true);
    try {
      await api.communitySignupVerify(signupEmail.trim(), otpCode.trim(), setupToken);
      setOtpVerified(true);
      setTimeout(() => setStep(3), 1000);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Incorrect code";
      setOtpError(message);
    } finally {
      setOtpLoading(false);
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
      const result = await api.createInitialAdmin(email, password, setupToken);
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

  const stepTitles: Record<1 | 2 | 3, string> = {
    1: "Enter the setup token from your console",
    2: "Activate a free Community key to get started",
    3: "Create your admin account",
  };

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
            <p className="text-gray-600 dark:text-gray-400 mt-2">{stepTitles[step]}</p>
          </div>

          {/* Step 1: Setup Token */}
          {step === 1 && (
            <form onSubmit={handleTokenSubmit} className="space-y-5">
              {tokenError && <div className={errorBoxClass}>{tokenError}</div>}

              <div>
                <label htmlFor="setupToken" className={labelClass}>
                  Setup Token
                </label>
                <input
                  id="setupToken"
                  type="text"
                  value={tokenInput}
                  onChange={(e) => setTokenInput(e.target.value)}
                  required
                  autoFocus
                  className={`${inputClass} font-mono text-sm`}
                  placeholder="Paste the token"
                />
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Find it with{" "}
                  <code className="font-mono">docker exec &lt;container&gt; cat /data/setup_token</code>
                </p>
              </div>

              <button type="submit" disabled={tokenLoading} className={buttonClass}>
                {tokenLoading ? "Checking..." : "Continue →"}
              </button>
            </form>
          )}

          {/* Step 2: License */}
          {step === 2 && (
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
                otpStage === "email" ? (
                  <form onSubmit={handleSendCode} className="space-y-5">
                    {signupError && <div className={errorBoxClass}>{signupError}</div>}

                    <div>
                      <label htmlFor="signupEmail" className={labelClass}>
                        Email Address
                      </label>
                      <input
                        id="signupEmail"
                        type="email"
                        value={signupEmail}
                        onChange={(e) => setSignupEmail(e.target.value)}
                        required
                        autoFocus
                        className={inputClass}
                        placeholder="you@example.com"
                      />
                      <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        No credit card. We&apos;ll email you a 6-digit code to activate a
                        free Community key.
                      </p>
                    </div>
                    <button type="submit" disabled={signupLoading} className={buttonClass}>
                      {signupLoading ? "Sending..." : "Send Code →"}
                    </button>
                  </form>
                ) : (
                  <form onSubmit={handleVerifyCode} className="space-y-5">
                    {otpError && <div className={errorBoxClass}>{otpError}</div>}

                    {otpVerified ? (
                      <div className="bg-green-500/10 border border-green-500/50 text-green-700 dark:text-green-400 px-4 py-3 rounded text-sm text-center">
                        Verified! Your Community key is active — continuing...
                      </div>
                    ) : (
                      <>
                        <div>
                          <label htmlFor="otpCode" className={labelClass}>
                            Verification Code
                          </label>
                          <input
                            id="otpCode"
                            type="text"
                            inputMode="numeric"
                            maxLength={6}
                            value={otpCode}
                            onChange={(e) => setOtpCode(e.target.value.replace(/\D/g, ""))}
                            required
                            autoFocus
                            className={`${inputClass} font-mono text-center text-2xl tracking-[0.3em]`}
                            placeholder="000000"
                          />
                          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                            Sent to <strong>{signupEmail}</strong>. Expires in 10 minutes.
                          </p>
                        </div>
                        <button
                          type="submit"
                          disabled={otpLoading || otpCode.length !== 6}
                          className={buttonClass}
                        >
                          {otpLoading ? "Verifying..." : "Verify Code →"}
                        </button>
                        <button
                          type="button"
                          onClick={handleResendCode}
                          disabled={resendLoading}
                          className="w-full text-sm text-primary-600 dark:text-primary-400 hover:underline disabled:opacity-50"
                        >
                          {resendLoading ? "Resending..." : "Resend code"}
                        </button>
                      </>
                    )}
                  </form>
                )
              ) : (
                <form onSubmit={handleLicenseSubmit} className="space-y-5">
                  {licenseError && <div className={errorBoxClass}>{licenseError}</div>}

                  <div>
                    <label htmlFor="licenseKey" className={labelClass}>
                      License Key
                    </label>
                    <input
                      id="licenseKey"
                      type="text"
                      value={licenseKey}
                      onChange={(e) => setLicenseKey(e.target.value)}
                      required
                      autoFocus
                      className={`${inputClass} font-mono`}
                      placeholder="IP-CE-XXXX-XXXX-XXXX"
                    />
                  </div>

                  <button type="submit" disabled={licenseLoading} className={buttonClass}>
                    {licenseLoading ? "Validating..." : "Activate License →"}
                  </button>
                </form>
              )}
            </div>
          )}

          {/* Step 3: Admin Account */}
          {step === 3 && (
            <form onSubmit={handleAdminSubmit} className="space-y-5">
              {adminError && <div className={errorBoxClass}>{adminError}</div>}

              <div>
                <label htmlFor="email" className={labelClass}>
                  Email Address
                </label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  autoFocus
                  className={inputClass}
                  placeholder="admin@example.com"
                />
              </div>

              <div>
                <label htmlFor="password" className={labelClass}>
                  Password
                </label>
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                  className={inputClass}
                  placeholder="At least 8 characters"
                />
              </div>

              <div>
                <label htmlFor="confirmPassword" className={labelClass}>
                  Confirm Password
                </label>
                <input
                  id="confirmPassword"
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  minLength={8}
                  className={inputClass}
                  placeholder="Confirm your password"
                />
              </div>

              <button type="submit" disabled={adminLoading} className={buttonClass}>
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
