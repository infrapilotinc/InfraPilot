"use client";

import { useState, useEffect } from "react";
import { api, SSLCertificateInfo, SSLStatus, DNSVerifyResult, SSLCertificateRecord, SSLSource } from "@/lib/api";
import { Button, Input } from "@/components/ui/page-layout";
import { cn } from "@/lib/utils";
import {
  Shield,
  ShieldCheck,
  ShieldAlert,
  Globe,
  Mail,
  RefreshCw,
  Check,
  X,
  AlertTriangle,
  ArrowRight,
  ArrowLeft,
  Loader2,
  Copy,
  Lock,
  FileKey,
  Scan,
} from "lucide-react";

interface SSLWizardProps {
  /** Primary domain for the certificate */
  domain: string;
  /** Additional domains (for Traffic Resources with multiple domains) */
  domains?: string[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
  // For regular proxy hosts
  agentId?: string;
  proxyId?: string;
  // For traffic resources
  resourceId?: string;
  // Whether to include www subdomain in SSL cert
  includeWWW?: boolean;
}

/** Resource type for unified handling */
type ResourceType = "proxy" | "resource" | "system";

type WizardStep = "check" | "source" | "wildcard" | "dns" | "email" | "request" | "dns_challenge" | "dns_verify" | "upload" | "complete";

export function SSLWizard({
  domain,
  domains = [],
  open,
  onOpenChange,
  onSuccess,
  agentId,
  proxyId,
  resourceId,
  includeWWW = false,
}: SSLWizardProps) {
  // Determine resource type
  const resourceType: ResourceType = resourceId ? "resource" : (agentId && proxyId) ? "proxy" : "system";
  const isRegularProxy = resourceType === "proxy";
  const isTrafficResource = resourceType === "resource";

  // All domains for this certificate (primary + additional)
  const allDomains = [domain, ...domains.filter(d => d !== domain)];
  const [step, setStep] = useState<WizardStep>("check");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Certificate check state
  const [certInfo, setCertInfo] = useState<SSLCertificateInfo | null>(null);
  const [wildcardCerts, setWildcardCerts] = useState<SSLCertificateInfo[]>([]);

  // SSL source selection
  const [sslSource, setSSLSource] = useState<SSLSource>("letsencrypt");
  const [availableCerts, setAvailableCerts] = useState<SSLCertificateRecord[]>([]);
  const [selectedCertId, setSelectedCertId] = useState<string>("");

  // DNS verification state
  const [dnsResult, setDnsResult] = useState<DNSVerifyResult | null>(null);
  const [serverIP, setServerIP] = useState<string>("");

  // Email/SSL config state
  const [sslStatus, setSSLStatus] = useState<SSLStatus | null>(null);
  const [email, setEmail] = useState("");
  const [staging, setStaging] = useState(false);

  // Request state
  const [requestStatus, setRequestStatus] = useState<"pending" | "success" | "error" | null>(null);
  const [resultMessage, setResultMessage] = useState<string>("");

  // DNS Challenge state (for wildcard certs)
  const [dnsChallenge, setDNSChallenge] = useState<{
    txt_record: string;
    txt_name: string;
    txt_records?: { name: string; value: string }[];
  } | null>(null);
  const [txtVerified, setTxtVerified] = useState(false);

  // Custom certificate upload state
  const [uploadCertPEM, setUploadCertPEM] = useState("");
  const [uploadKeyPEM, setUploadKeyPEM] = useState("");
  const [uploadValidationError, setUploadValidationError] = useState<string | null>(null);

  // Reset state when dialog opens/closes
  useEffect(() => {
    if (open) {
      setStep("check");
      setCertInfo(null);
      setWildcardCerts([]);
      setAvailableCerts([]);
      setSelectedCertId("");
      setSSLSource("letsencrypt");
      setDnsResult(null);
      setServerIP("");
      setSSLStatus(null);
      setEmail("");
      setStaging(false);
      setRequestStatus(null);
      setResultMessage("");
      setError(null);
      setDNSChallenge(null);
      setTxtVerified(false);
      setUploadCertPEM("");
      setUploadKeyPEM("");
      setUploadValidationError(null);
      // Start certificate check
      checkCertificate();
    }
  }, [open, domain]);

  const checkCertificate = async () => {
    if (!domain) return;

    setLoading(true);
    setError(null);

    try {
      // Check both local and remote certificates
      const [remoteCheck, wildcardCheck] = await Promise.all([
        api.checkSSL(domain, true),
        api.checkWildcardSSL(domain),
      ]);

      setCertInfo(remoteCheck);
      setWildcardCerts(wildcardCheck.certificates || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to check certificate");
    } finally {
      setLoading(false);
    }
  };

  const verifyDNS = async () => {
    setLoading(true);
    setError(null);
    try {
      const [dnsCheck, instructions] = await Promise.all([
        api.verifyDNS(domain, includeWWW),
        api.getDNSInstructions(domain),
      ]);
      setDnsResult(dnsCheck);
      setServerIP(instructions.server_ip);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to verify DNS");
    } finally {
      setLoading(false);
    }
  };

  const loadSSLSettings = async () => {
    setLoading(true);
    setError(null);
    try {
      const status = await api.getSSLStatus();
      setSSLStatus(status);
      if (status.letsencrypt_email) {
        setEmail(status.letsencrypt_email);
      }
      setStaging(status.letsencrypt_staging);
    } catch {
      // Settings might not exist yet
      setSSLStatus(null);
    } finally {
      setLoading(false);
    }
  };

  const scanCertificates = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.scanSSLCertificates();
      setAvailableCerts(result.certificates || []);
      // Auto-select a matching wildcard cert if available
      const parentDomain = domain.split('.').slice(1).join('.');
      const matchingCert = result.certificates?.find(c =>
        c.is_wildcard && (c.domain === parentDomain || c.domain === `*.${parentDomain}`)
      );
      if (matchingCert?.id) {
        setSelectedCertId(matchingCert.id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to scan certificates");
    } finally {
      setLoading(false);
    }
  };

  const saveEmail = async () => {
    if (!email) {
      setError("Email is required for Let's Encrypt");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await api.updateSSLSettings({ email, staging });

      if (sslSource === "dns_challenge") {
        // For DNS challenge, start the challenge process
        const parentDomain = domain.split('.').slice(-2).join('.');
        const wildcardDomain = `*.${parentDomain}`;
        await startDNSChallenge(wildcardDomain);
      } else {
        setStep("request");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setLoading(false);
    }
  };

  const requestCertificate = async () => {
    setLoading(true);
    setError(null);
    setRequestStatus("pending");
    setResultMessage("");
    try {
      const response = await api.requestSSLCertificate({
        domain,
        email,
        staging,
      });
      if (response.success) {
        setRequestStatus("success");
        setResultMessage(response.message || "SSL certificate issued successfully");
        setStep("complete");
        onSuccess?.();
      } else {
        setRequestStatus("error");
        setError(response.error || "Failed to request certificate");
      }
    } catch (err: unknown) {
      setRequestStatus("error");
      // Try to extract error message from response
      const error = err as { message?: string; error?: string };
      const errorMessage = error?.error || error?.message || "Failed to request certificate";
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const startDNSChallenge = async (wildcardDomain: string) => {
    setLoading(true);
    setError(null);
    setRequestStatus("pending");
    try {
      const response = await api.startDNSChallenge({
        domain: wildcardDomain,
        email,
        staging,
      });
      if (response.success && response.txt_record && response.txt_name) {
        setDNSChallenge({
          txt_record: response.txt_record,
          txt_name: response.txt_name,
          txt_records: response.txt_records || [],
        });
        setRequestStatus(null);
        setStep("dns_verify");
      } else {
        setRequestStatus("error");
        setError(response.error || "Failed to start DNS challenge");
      }
    } catch (err: unknown) {
      setRequestStatus("error");
      const error = err as { message?: string; error?: string };
      setError(error?.error || error?.message || "Failed to start DNS challenge");
    } finally {
      setLoading(false);
    }
  };

  const verifyTXTRecord = async () => {
    if (!dnsChallenge) return;

    setLoading(true);
    setError(null);
    try {
      // Extract base domain for wildcard
      const baseDomain = domain.startsWith("*.") ? domain.substring(2) : domain;
      const response = await api.verifyDNSTXTRecord(baseDomain, dnsChallenge.txt_record);
      setTxtVerified(response.verified);
      if (!response.verified) {
        setError(`TXT record not found. Found: ${response.found?.join(", ") || "none"}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to verify TXT record");
    } finally {
      setLoading(false);
    }
  };

  const completeDNSChallenge = async (wildcardDomain: string) => {
    setLoading(true);
    setError(null);
    setRequestStatus("pending");
    try {
      const response = await api.completeDNSChallenge({
        domain: wildcardDomain,
        email,
        staging,
      });
      if (response.success) {
        setRequestStatus("success");
        setResultMessage(response.message || "Wildcard SSL certificate issued successfully");
        setStep("complete");
        onSuccess?.();
      } else {
        setRequestStatus("error");
        setError(response.error || "Failed to complete DNS challenge");
      }
    } catch (err: unknown) {
      setRequestStatus("error");
      const error = err as { message?: string; error?: string };
      setError(error?.error || error?.message || "Failed to complete DNS challenge");
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const uploadCertificate = async () => {
    setUploadValidationError(null);

    // Client-side PEM header checks before sending to server
    const trimmedCert = uploadCertPEM.trim();
    const trimmedKey = uploadKeyPEM.trim();

    if (!trimmedCert.startsWith("-----BEGIN CERTIFICATE-----")) {
      setUploadValidationError("Certificate must be in PEM format and start with -----BEGIN CERTIFICATE-----");
      return;
    }
    const validKeyHeaders = ["-----BEGIN PRIVATE KEY-----", "-----BEGIN RSA PRIVATE KEY-----", "-----BEGIN EC PRIVATE KEY-----"];
    if (!validKeyHeaders.some(h => trimmedKey.startsWith(h))) {
      setUploadValidationError("Private key must be in PEM format (PRIVATE KEY, RSA PRIVATE KEY, or EC PRIVATE KEY)");
      return;
    }

    setLoading(true);
    setError(null);
    setRequestStatus("pending");
    try {
      const response = await api.uploadSSLCertificate({
        cert_pem: trimmedCert,
        key_pem: trimmedKey,
        ...(agentId && proxyId ? { agent_id: agentId, proxy_id: proxyId } : {}),
      });
      if (response.error) {
        setRequestStatus("error");
        setError(response.error);
        return;
      }
      setRequestStatus("success");
      setResultMessage(`Certificate for ${response.domain} uploaded successfully (issued by ${response.issuer}, expires ${new Date(response.expires_at).toLocaleDateString()})`);
      setStep("complete");
      onSuccess?.();
    } catch (err: unknown) {
      setRequestStatus("error");
      const e = err as { message?: string; error?: string };
      setError(e?.error || e?.message || "Failed to upload certificate");
    } finally {
      setLoading(false);
    }
  };

  const goToStep = (newStep: WizardStep) => {
    setError(null);
    if (newStep === "wildcard") {
      scanCertificates();
    } else if (newStep === "dns") {
      verifyDNS();
    } else if (newStep === "email") {
      loadSSLSettings();
    }
    setStep(newStep);
  };

  // Apply wildcard certificate directly
  const applyWildcardCert = async () => {
    if (!selectedCertId) {
      setError("Please select a certificate");
      return;
    }

    // Find the selected certificate to get its paths
    const selectedCert = availableCerts.find(c => c.id === selectedCertId);
    if (!selectedCert) {
      setError("Selected certificate not found");
      return;
    }

    setLoading(true);
    setError(null);
    setRequestStatus("pending");
    try {
      if (isRegularProxy && agentId && proxyId) {
        // For regular proxy hosts, use the proxy SSL endpoint
        await api.applyWildcardSSL(agentId, proxyId, {
          ssl_enabled: true,
          force_ssl: true,
          http2_enabled: true,
          ssl_source: "wildcard",
          ssl_cert_path: selectedCert.cert_path,
          ssl_key_path: selectedCert.key_path,
        });
      } else {
        // For system domain (InfraPilot), use the domain settings endpoint
        await api.updateInfraPilotDomain({
          domain,
          ssl_enabled: true,
          force_ssl: true,
          http2_enabled: true,
          ssl_source: "wildcard",
          ssl_certificate_id: selectedCertId,
          ssl_cert_path: selectedCert.cert_path,
          ssl_key_path: selectedCert.key_path,
        });
      }
      setRequestStatus("success");
      setResultMessage("SSL certificate applied successfully using wildcard certificate");
      setStep("complete");
      onSuccess?.();
    } catch (err: unknown) {
      setRequestStatus("error");
      const error = err as { message?: string; error?: string };
      setError(error?.error || error?.message || "Failed to apply certificate");
    } finally {
      setLoading(false);
    }
  };

  if (!open) return null;

  // Steps depend on SSL source selection
  const getSteps = (): { key: WizardStep; label: string; icon: typeof Shield }[] => {
    if (sslSource === "wildcard") {
      return [
        { key: "check", label: "Check", icon: Shield },
        { key: "source", label: "Source", icon: FileKey },
        { key: "wildcard", label: "Select", icon: Scan },
        { key: "request", label: "Apply", icon: Lock },
      ];
    }
    if (sslSource === "dns_challenge") {
      // Wildcard certificate via DNS-01 challenge
      return [
        { key: "check", label: "Check", icon: Shield },
        { key: "source", label: "Source", icon: FileKey },
        { key: "email", label: "Email", icon: Mail },
        { key: "dns_challenge", label: "DNS TXT", icon: Globe },
        { key: "dns_verify", label: "Verify", icon: Check },
      ];
    }
    if (sslSource === "external") {
      return [
        { key: "check", label: "Check", icon: Shield },
        { key: "source", label: "Source", icon: FileKey },
        { key: "upload", label: "Upload", icon: Lock },
      ];
    }
    // Let's Encrypt HTTP-01 flow
    return [
      { key: "check", label: "Check", icon: Shield },
      { key: "source", label: "Source", icon: FileKey },
      { key: "dns", label: "DNS", icon: Globe },
      { key: "email", label: "Email", icon: Mail },
      { key: "request", label: "Request", icon: Lock },
    ];
  };

  const steps = getSteps();
  const currentStepIndex = steps.findIndex((s) => s.key === step);

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center"
      onClick={(e) => e.stopPropagation()}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={(e) => {
          e.stopPropagation();
          onOpenChange(false);
        }}
      />

      {/* Dialog */}
      <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl w-full max-w-lg max-h-[90vh] overflow-hidden mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-800">
          <div className="flex items-center gap-2">
            <Shield className="h-5 w-5 text-primary-600" />
            <div>
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
                SSL Certificate Wizard
              </h2>
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Set up HTTPS for {domain}
              </p>
            </div>
          </div>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onOpenChange(false);
            }}
            className="p-2 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Steps indicator */}
        {step !== "complete" && (
          <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-800">
            <div className="flex items-center justify-center">
              {steps.map((s, index) => {
                const Icon = s.icon;
                const isActive = s.key === step;
                const isCompleted = index < currentStepIndex;

                return (
                  <div key={s.key} className="flex items-center">
                    <div
                      className={cn(
                        "w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-colors",
                        isActive
                          ? "bg-primary-600 text-white"
                          : isCompleted
                          ? "bg-green-500 text-white"
                          : "bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400"
                      )}
                    >
                      {isCompleted ? (
                        <Check className="h-4 w-4" />
                      ) : (
                        <Icon className="h-4 w-4" />
                      )}
                    </div>
                    {index < steps.length - 1 && (
                      <div
                        className={cn(
                          "w-12 h-0.5 mx-1",
                          index < currentStepIndex
                            ? "bg-green-500"
                            : "bg-gray-200 dark:bg-gray-700"
                        )}
                      />
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Content */}
        <div className="px-6 py-6 overflow-y-auto max-h-[50vh]">
          {error && (
            <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
              <AlertTriangle className="h-5 w-5 text-red-500 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-700 dark:text-red-400">{error}</p>
            </div>
          )}

          {/* Step: Check */}
          {step === "check" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Checking SSL Certificate Status
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Verifying existing certificates for {domain}
                </p>
              </div>

              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              ) : certInfo ? (
                <div className="space-y-4">
                  {/* Current certificate status */}
                  <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-start gap-3">
                      {certInfo.exists && certInfo.valid_for_domain ? (
                        <ShieldCheck className="h-6 w-6 text-green-500 flex-shrink-0 mt-0.5" />
                      ) : certInfo.exists ? (
                        <ShieldAlert className="h-6 w-6 text-yellow-500 flex-shrink-0 mt-0.5" />
                      ) : (
                        <ShieldAlert className="h-6 w-6 text-gray-400 flex-shrink-0 mt-0.5" />
                      )}
                      <div className="flex-1">
                        <h4 className="font-medium text-gray-900 dark:text-white">
                          {certInfo.exists
                            ? certInfo.valid_for_domain
                              ? "Valid Certificate Found"
                              : "Certificate Found (Not Valid for Domain)"
                            : "No Certificate Found"}
                        </h4>
                        {certInfo.exists && (
                          <div className="mt-2 text-sm text-gray-500 dark:text-gray-400 space-y-1">
                            <p>
                              <span className="font-medium">Issuer:</span> {certInfo.issuer}
                            </p>
                            {certInfo.expires_at && (
                              <p>
                                <span className="font-medium">Expires:</span>{" "}
                                {new Date(certInfo.expires_at).toLocaleDateString()} ({certInfo.days_left} days left)
                              </p>
                            )}
                            {certInfo.is_wildcard && (
                              <p className="text-blue-600 dark:text-blue-400">
                                This is a wildcard certificate
                              </p>
                            )}
                          </div>
                        )}
                        {!certInfo.exists && !certInfo.error && (
                          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
                            This domain doesn't have an SSL certificate yet. Continue to set one up with Let's Encrypt.
                          </p>
                        )}
                        {certInfo.error && (
                          <p className="mt-2 text-sm text-red-500">{certInfo.error}</p>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Wildcard certificates */}
                  {wildcardCerts.length > 0 && wildcardCerts.some((w) => w.exists && w.valid_for_domain) && (
                    <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                      <div className="flex items-start gap-3">
                        <Shield className="h-5 w-5 text-blue-500 flex-shrink-0 mt-0.5" />
                        <div>
                          <h4 className="font-medium text-blue-700 dark:text-blue-300">
                            Wildcard Certificate Available
                          </h4>
                          <p className="text-sm text-blue-600 dark:text-blue-400 mt-1">
                            A wildcard certificate exists that covers this domain. You may not need a new certificate.
                          </p>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Actions */}
                  <div className="flex justify-between pt-4">
                    <Button variant="secondary" onClick={(e) => { e?.stopPropagation(); onOpenChange(false); }}>
                      Cancel
                    </Button>
                    <div className="flex gap-2">
                      <Button variant="ghost" onClick={checkCertificate} icon={RefreshCw}>
                        Recheck
                      </Button>
                      {(!certInfo.exists || !certInfo.valid_for_domain || (certInfo.days_left && certInfo.days_left < 30)) && (
                        <Button variant="primary" onClick={() => goToStep("source")} icon={ArrowRight}>
                          Get Certificate
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
              ) : null}
            </div>
          )}

          {/* Step: Source Selection */}
          {step === "source" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Choose Certificate Source
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  How would you like to get an SSL certificate?
                </p>
              </div>

              <div className="space-y-3">
                {/* Let's Encrypt option */}
                <button
                  onClick={() => setSSLSource("letsencrypt")}
                  className={cn(
                    "w-full p-4 rounded-lg border text-left transition-colors",
                    sslSource === "letsencrypt"
                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                      : "border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600"
                  )}
                >
                  <div className="flex items-start gap-3">
                    <div className={cn(
                      "w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5",
                      sslSource === "letsencrypt"
                        ? "border-primary-500"
                        : "border-gray-300 dark:border-gray-600"
                    )}>
                      {sslSource === "letsencrypt" && (
                        <div className="w-2.5 h-2.5 rounded-full bg-primary-500" />
                      )}
                    </div>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        Request new certificate (Let&apos;s Encrypt)
                      </p>
                      <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Automatically obtain a free SSL certificate from Let&apos;s Encrypt
                      </p>
                    </div>
                  </div>
                </button>

                {/* Wildcard with DNS-01 challenge option */}
                <button
                  onClick={() => setSSLSource("dns_challenge")}
                  className={cn(
                    "w-full p-4 rounded-lg border text-left transition-colors",
                    sslSource === "dns_challenge"
                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                      : "border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600"
                  )}
                >
                  <div className="flex items-start gap-3">
                    <div className={cn(
                      "w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5",
                      sslSource === "dns_challenge"
                        ? "border-primary-500"
                        : "border-gray-300 dark:border-gray-600"
                    )}>
                      {sslSource === "dns_challenge" && (
                        <div className="w-2.5 h-2.5 rounded-full bg-primary-500" />
                      )}
                    </div>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        Request wildcard certificate (DNS verification)
                      </p>
                      <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Get a wildcard certificate (*.domain.com) via DNS TXT record verification
                      </p>
                    </div>
                  </div>
                </button>

                {/* Use existing wildcard option */}
                <button
                  onClick={() => setSSLSource("wildcard")}
                  className={cn(
                    "w-full p-4 rounded-lg border text-left transition-colors",
                    sslSource === "wildcard"
                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                      : "border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600"
                  )}
                >
                  <div className="flex items-start gap-3">
                    <div className={cn(
                      "w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5",
                      sslSource === "wildcard"
                        ? "border-primary-500"
                        : "border-gray-300 dark:border-gray-600"
                    )}>
                      {sslSource === "wildcard" && (
                        <div className="w-2.5 h-2.5 rounded-full bg-primary-500" />
                      )}
                    </div>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        Use existing wildcard certificate
                      </p>
                      <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Select from wildcard certificates already installed on the server
                      </p>
                    </div>
                  </div>
                </button>

                {/* Upload custom certificate option */}
                <button
                  onClick={() => setSSLSource("external")}
                  className={cn(
                    "w-full p-4 rounded-lg border text-left transition-colors",
                    sslSource === "external"
                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                      : "border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600"
                  )}
                >
                  <div className="flex items-start gap-3">
                    <div className={cn(
                      "w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5",
                      sslSource === "external"
                        ? "border-primary-500"
                        : "border-gray-300 dark:border-gray-600"
                    )}>
                      {sslSource === "external" && (
                        <div className="w-2.5 h-2.5 rounded-full bg-primary-500" />
                      )}
                    </div>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        Upload custom certificate
                      </p>
                      <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                        Paste your own certificate and private key (DigiCert, Sectigo, ZeroSSL, internal CA, etc.)
                      </p>
                    </div>
                  </div>
                </button>
              </div>

              {/* Actions */}
              <div className="flex justify-between pt-4">
                <Button variant="secondary" onClick={() => goToStep("check")} icon={ArrowLeft}>
                  Back
                </Button>
                <Button
                  variant="primary"
                  onClick={() => {
                    if (sslSource === "wildcard") {
                      goToStep("wildcard");
                    } else if (sslSource === "dns_challenge") {
                      goToStep("email");
                    } else if (sslSource === "external") {
                      goToStep("upload");
                    } else {
                      goToStep("dns");
                    }
                  }}
                  icon={ArrowRight}
                >
                  Continue
                </Button>
              </div>
            </div>
          )}

          {/* Step: Upload Custom Certificate */}
          {step === "upload" && (
            <div className="space-y-4">
              <div className="text-center mb-2">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Upload Certificate
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Paste your PEM-encoded certificate and private key below
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Certificate (PEM)
                </label>
                <textarea
                  value={uploadCertPEM}
                  onChange={(e) => { setUploadCertPEM(e.target.value); setUploadValidationError(null); }}
                  placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"}
                  rows={7}
                  spellCheck={false}
                  className="w-full px-3 py-2 font-mono text-xs bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent resize-none"
                />
                <p className="mt-1 text-xs text-gray-500">Include the full chain (cert + intermediates) if applicable</p>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Private Key (PEM)
                </label>
                <textarea
                  value={uploadKeyPEM}
                  onChange={(e) => { setUploadKeyPEM(e.target.value); setUploadValidationError(null); }}
                  placeholder={"-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"}
                  rows={7}
                  spellCheck={false}
                  className="w-full px-3 py-2 font-mono text-xs bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent resize-none"
                />
                <p className="mt-1 text-xs text-gray-500">Unencrypted key only — RSA, EC, and PKCS#8 supported</p>
              </div>

              {uploadValidationError && (
                <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-400">
                  {uploadValidationError}
                </div>
              )}

              {error && (
                <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-400">
                  {error}
                </div>
              )}

              <div className="flex justify-between pt-2">
                <Button variant="secondary" onClick={() => goToStep("source")} icon={ArrowLeft}>
                  Back
                </Button>
                <Button
                  variant="primary"
                  onClick={uploadCertificate}
                  disabled={loading || !uploadCertPEM.trim() || !uploadKeyPEM.trim()}
                  icon={loading ? Loader2 : Lock}
                >
                  {loading ? "Uploading..." : "Upload & Apply"}
                </Button>
              </div>
            </div>
          )}

          {/* Step: Wildcard Certificate Selection */}
          {step === "wildcard" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Select Wildcard Certificate
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Choose a wildcard certificate to use for {domain}
                </p>
              </div>

              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              ) : availableCerts.length > 0 ? (
                <div className="space-y-3">
                  {availableCerts.filter(c => c.is_wildcard).map((cert) => {
                    const parentDomain = domain.split('.').slice(1).join('.');
                    const isMatch = cert.domain === parentDomain || cert.san?.includes(`*.${parentDomain}`);

                    return (
                      <button
                        key={cert.id || cert.cert_path}
                        onClick={() => setSelectedCertId(cert.id)}
                        className={cn(
                          "w-full p-4 rounded-lg border text-left transition-colors",
                          selectedCertId === cert.id
                            ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                            : "border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600"
                        )}
                      >
                        <div className="flex items-start gap-3">
                          <div className={cn(
                            "w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5",
                            selectedCertId === cert.id
                              ? "border-primary-500"
                              : "border-gray-300 dark:border-gray-600"
                          )}>
                            {selectedCertId === cert.id && (
                              <div className="w-2.5 h-2.5 rounded-full bg-primary-500" />
                            )}
                          </div>
                          <div className="flex-1">
                            <div className="flex items-center gap-2">
                              <p className="font-medium text-gray-900 dark:text-white">
                                *.{cert.domain}
                              </p>
                              {isMatch && (
                                <span className="px-2 py-0.5 text-xs bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 rounded">
                                  Matches domain
                                </span>
                              )}
                            </div>
                            <div className="mt-1 text-sm text-gray-500 dark:text-gray-400 space-y-0.5">
                              {cert.issuer && <p>Issuer: {cert.issuer}</p>}
                              {cert.expires_at && (
                                <p>Expires: {new Date(cert.expires_at).toLocaleDateString()}</p>
                              )}
                            </div>
                          </div>
                        </div>
                      </button>
                    );
                  })}
                </div>
              ) : (
                <div className="p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                  <div className="flex items-start gap-3">
                    <AlertTriangle className="h-5 w-5 text-yellow-500 flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="font-medium text-yellow-700 dark:text-yellow-300">
                        No Wildcard Certificates Found
                      </p>
                      <p className="text-sm text-yellow-600 dark:text-yellow-400 mt-1">
                        No wildcard certificates were found in /etc/letsencrypt/live/.
                        You can use Let&apos;s Encrypt instead.
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {/* Actions */}
              <div className="flex justify-between pt-4">
                <Button variant="secondary" onClick={() => goToStep("source")} icon={ArrowLeft}>
                  Back
                </Button>
                <div className="flex gap-2">
                  <Button variant="ghost" onClick={scanCertificates} icon={RefreshCw}>
                    Rescan
                  </Button>
                  <Button
                    variant="primary"
                    onClick={applyWildcardCert}
                    disabled={loading || !selectedCertId}
                    icon={loading ? Loader2 : ShieldCheck}
                  >
                    Apply Certificate
                  </Button>
                </div>
              </div>
            </div>
          )}

          {/* Step: DNS */}
          {step === "dns" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Verify DNS Configuration
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Ensure {domain} points to your server
                </p>
              </div>

              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              ) : dnsResult ? (
                <div className="space-y-4">
                  {/* DNS Status */}
                  <div
                    className={cn(
                      "p-4 rounded-lg border",
                      dnsResult.matches
                        ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800"
                        : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800"
                    )}
                  >
                    <div className="flex items-start gap-3">
                      {dnsResult.matches ? (
                        <Check className="h-6 w-6 text-green-500 flex-shrink-0" />
                      ) : (
                        <AlertTriangle className="h-6 w-6 text-yellow-500 flex-shrink-0" />
                      )}
                      <div className="flex-1">
                        <h4
                          className={cn(
                            "font-medium",
                            dnsResult.matches
                              ? "text-green-700 dark:text-green-400"
                              : "text-yellow-700 dark:text-yellow-400"
                          )}
                        >
                          {dnsResult.matches ? "DNS Configured Correctly" : "DNS Not Configured"}
                        </h4>
                        <div className="mt-2 text-sm space-y-1 text-gray-600 dark:text-gray-400">
                          <p>
                            <span className="font-medium">Domain:</span> {dnsResult.domain}
                          </p>
                          <p>
                            <span className="font-medium">Expected IP:</span> {dnsResult.expected_ip || serverIP}
                          </p>
                          <p>
                            <span className="font-medium">Resolved IPs:</span>{" "}
                            {dnsResult.resolved_ips?.length > 0
                              ? dnsResult.resolved_ips.join(", ")
                              : "None"}
                          </p>
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Show individual domain results when include_www is enabled */}
                  {includeWWW && (dnsResult as any).www_result && (
                    <div className="space-y-2">
                      <div className={cn(
                        "p-3 rounded-lg border flex items-center gap-2",
                        (dnsResult as any).main_result?.matches
                          ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800"
                          : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800"
                      )}>
                        {(dnsResult as any).main_result?.matches ? (
                          <Check className="h-4 w-4 text-green-500" />
                        ) : (
                          <X className="h-4 w-4 text-yellow-500" />
                        )}
                        <span className="text-sm">{domain}: {(dnsResult as any).main_result?.resolved_ips?.join(", ") || "Not resolved"}</span>
                      </div>
                      <div className={cn(
                        "p-3 rounded-lg border flex items-center gap-2",
                        (dnsResult as any).www_result?.matches
                          ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800"
                          : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800"
                      )}>
                        {(dnsResult as any).www_result?.matches ? (
                          <Check className="h-4 w-4 text-green-500" />
                        ) : (
                          <X className="h-4 w-4 text-yellow-500" />
                        )}
                        <span className="text-sm">www.{domain}: {(dnsResult as any).www_result?.resolved_ips?.join(", ") || "Not resolved"}</span>
                      </div>
                    </div>
                  )}

                  {/* DNS Instructions */}
                  {!dnsResult.matches && (
                    <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
                      <h4 className="font-medium text-gray-900 dark:text-white mb-2">
                        Add {includeWWW ? "these DNS records" : "this DNS record"}:
                      </h4>
                      <div className="space-y-2">
                        <div className="bg-white dark:bg-gray-900 p-3 rounded font-mono text-sm flex items-center justify-between border border-gray-200 dark:border-gray-700">
                          <span className="text-gray-900 dark:text-white">
                            A {domain} {serverIP || dnsResult.expected_ip}
                          </span>
                          <button
                            onClick={() => copyToClipboard(`${domain} A ${serverIP || dnsResult.expected_ip}`)}
                            className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                          >
                            <Copy className="h-4 w-4" />
                          </button>
                        </div>
                        {includeWWW && (
                          <div className="bg-white dark:bg-gray-900 p-3 rounded font-mono text-sm flex items-center justify-between border border-gray-200 dark:border-gray-700">
                            <span className="text-gray-900 dark:text-white">
                              A www.{domain} {serverIP || dnsResult.expected_ip}
                            </span>
                            <button
                              onClick={() => copyToClipboard(`www.${domain} A ${serverIP || dnsResult.expected_ip}`)}
                              className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                            >
                              <Copy className="h-4 w-4" />
                            </button>
                          </div>
                        )}
                      </div>
                      <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                        DNS changes can take 1-48 hours to propagate. Usually 1-5 minutes.
                      </p>
                    </div>
                  )}

                  {/* Actions */}
                  <div className="flex justify-between pt-4">
                    <Button variant="secondary" onClick={() => goToStep("source")} icon={ArrowLeft}>
                      Back
                    </Button>
                    <div className="flex gap-2">
                      <Button variant="ghost" onClick={verifyDNS} icon={RefreshCw}>
                        Recheck DNS
                      </Button>
                      <Button
                        variant="primary"
                        onClick={() => goToStep("email")}
                        disabled={!dnsResult.configured && !dnsResult.matches}
                        icon={ArrowRight}
                      >
                        Continue
                      </Button>
                    </div>
                  </div>
                </div>
              ) : null}
            </div>
          )}

          {/* Step: Email */}
          {step === "email" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Let's Encrypt Configuration
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Configure email for certificate notifications
                </p>
              </div>

              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex items-start gap-2">
                    <Mail className="h-5 w-5 mt-8 text-gray-400" />
                    <Input
                      label="Email Address"
                      type="email"
                      placeholder="admin@example.com"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      className="flex-1"
                    />
                  </div>
                  <p className="text-xs text-gray-500 dark:text-gray-400 ml-7">
                    Let's Encrypt will send expiry notifications to this email
                  </p>

                  <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        Use Staging Environment
                      </p>
                      <p className="text-xs text-gray-500 dark:text-gray-400">
                        Test with staging first to avoid rate limits
                      </p>
                    </div>
                    <button
                      onClick={() => setStaging(!staging)}
                      className={cn(
                        "relative inline-flex h-6 w-11 items-center rounded-full transition-colors",
                        staging ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-600"
                      )}
                    >
                      <span
                        className={cn(
                          "inline-block h-4 w-4 transform rounded-full bg-white transition-transform",
                          staging ? "translate-x-6" : "translate-x-1"
                        )}
                      />
                    </button>
                  </div>

                  {staging && (
                    <div className="p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                      <div className="flex items-start gap-2">
                        <AlertTriangle className="h-4 w-4 text-yellow-600 flex-shrink-0 mt-0.5" />
                        <span className="text-sm text-yellow-700 dark:text-yellow-300">
                          Staging certificates are not trusted by browsers. Disable staging for production use.
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Actions */}
                  <div className="flex justify-between pt-4">
                    <Button variant="secondary" onClick={() => goToStep(sslSource === "dns_challenge" ? "source" : "dns")} icon={ArrowLeft}>
                      Back
                    </Button>
                    <Button
                      variant="primary"
                      onClick={saveEmail}
                      disabled={loading || !email}
                      icon={loading ? Loader2 : ArrowRight}
                    >
                      {sslSource === "dns_challenge" ? "Start DNS Challenge" : "Continue"}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Step: DNS Verify (for wildcard with DNS-01) */}
          {step === "dns_verify" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Add DNS TXT Record
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Add the following TXT record to verify domain ownership
                </p>
              </div>

              {dnsChallenge ? (
                <div className="space-y-4">
                  {/* TXT Record Info - Show ALL records for wildcard */}
                  {dnsChallenge.txt_records && dnsChallenge.txt_records.length > 1 ? (
                    <div className="space-y-3">
                      <div className="p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                        <div className="flex items-start gap-2">
                          <AlertTriangle className="h-4 w-4 text-yellow-600 flex-shrink-0 mt-0.5" />
                          <span className="text-sm text-yellow-700 dark:text-yellow-300">
                            <strong>Important:</strong> Wildcard certificates require {dnsChallenge.txt_records.length} TXT records with the same name but different values. Add ALL records below.
                          </span>
                        </div>
                      </div>
                      {dnsChallenge.txt_records.map((record, index) => (
                        <div key={index} className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 space-y-3">
                          <div className="flex items-center gap-2 text-sm font-medium text-gray-600 dark:text-gray-400">
                            <span className="px-2 py-0.5 bg-primary-100 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300 rounded">
                              Record {index + 1} of {dnsChallenge.txt_records!.length}
                            </span>
                          </div>
                          <div>
                            <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                              Name / Host
                            </label>
                            <div className="flex items-center gap-2">
                              <p className="text-gray-900 dark:text-white font-mono text-sm break-all">{record.name}</p>
                              <button
                                onClick={() => copyToClipboard(record.name)}
                                className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                              >
                                <Copy className="h-4 w-4" />
                              </button>
                            </div>
                          </div>
                          <div>
                            <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                              Value / Content
                            </label>
                            <div className="flex items-center gap-2">
                              <p className="text-gray-900 dark:text-white font-mono text-sm break-all">{record.value}</p>
                              <button
                                onClick={() => copyToClipboard(record.value)}
                                className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                              >
                                <Copy className="h-4 w-4" />
                              </button>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 space-y-3">
                      <div>
                        <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                          Record Type
                        </label>
                        <p className="text-gray-900 dark:text-white font-mono">TXT</p>
                      </div>
                      <div>
                        <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                          Name / Host
                        </label>
                        <div className="flex items-center gap-2">
                          <p className="text-gray-900 dark:text-white font-mono text-sm break-all">{dnsChallenge.txt_name}</p>
                          <button
                            onClick={() => copyToClipboard(dnsChallenge.txt_name)}
                            className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                          >
                            <Copy className="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                      <div>
                        <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                          Value / Content
                        </label>
                        <div className="flex items-center gap-2">
                          <p className="text-gray-900 dark:text-white font-mono text-sm break-all">{dnsChallenge.txt_record}</p>
                          <button
                            onClick={() => copyToClipboard(dnsChallenge.txt_record)}
                            className="p-1 text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
                          >
                            <Copy className="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Instructions */}
                  <div className="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                    <h4 className="font-medium text-blue-700 dark:text-blue-300 mb-2">
                      Instructions:
                    </h4>
                    <ol className="text-sm text-blue-600 dark:text-blue-400 space-y-1 list-decimal list-inside">
                      <li>Log in to your DNS provider (Cloudflare, GoDaddy, etc.)</li>
                      <li>Add a new TXT record with the name and value above</li>
                      <li>Wait 1-5 minutes for DNS propagation</li>
                      <li>Click &quot;Verify TXT Record&quot; to check</li>
                      <li>Once verified, click &quot;Complete Challenge&quot;</li>
                    </ol>
                  </div>

                  {/* Verification Status */}
                  {txtVerified && (
                    <div className="p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg flex items-center gap-2">
                      <Check className="h-5 w-5 text-green-500" />
                      <span className="text-sm text-green-700 dark:text-green-400">
                        TXT record verified! You can now complete the challenge.
                      </span>
                    </div>
                  )}

                  {requestStatus === "pending" && (
                    <div className="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                      <div className="flex items-center gap-3">
                        <Loader2 className="h-5 w-5 animate-spin text-blue-500" />
                        <div>
                          <p className="font-medium text-blue-700 dark:text-blue-300">
                            Completing DNS Challenge...
                          </p>
                          <p className="text-sm text-blue-600 dark:text-blue-400">
                            This may take up to 3 minutes
                          </p>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Actions */}
                  <div className="flex justify-between pt-4">
                    <Button variant="secondary" onClick={() => goToStep("email")} icon={ArrowLeft}>
                      Back
                    </Button>
                    <div className="flex gap-2">
                      <Button
                        variant="ghost"
                        onClick={verifyTXTRecord}
                        disabled={loading}
                        icon={loading ? Loader2 : RefreshCw}
                      >
                        Verify TXT Record
                      </Button>
                      <Button
                        variant="primary"
                        onClick={() => {
                          const parentDomain = domain.split('.').slice(-2).join('.');
                          completeDNSChallenge(`*.${parentDomain}`);
                        }}
                        disabled={loading || requestStatus === "pending"}
                        icon={loading ? Loader2 : ShieldCheck}
                      >
                        Complete Challenge
                      </Button>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              )}
            </div>
          )}

          {/* Step: Request */}
          {step === "request" && (
            <div className="space-y-4">
              <div className="text-center mb-4">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Request Certificate
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Ready to request SSL certificate for {includeWWW ? `${domain} and www.${domain}` : domain}
                </p>
              </div>

              <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 space-y-3">
                <div className="flex items-start gap-2">
                  <Globe className="h-4 w-4 text-gray-400 mt-0.5" />
                  <span className="font-medium text-gray-700 dark:text-gray-300">{includeWWW ? "Domains:" : "Domain:"}</span>
                  <div className="text-gray-900 dark:text-white">
                    {includeWWW ? (
                      <div className="flex flex-col">
                        <span>{domain}</span>
                        <span>www.{domain}</span>
                      </div>
                    ) : (
                      <span>{domain}</span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Mail className="h-4 w-4 text-gray-400" />
                  <span className="font-medium text-gray-700 dark:text-gray-300">Email:</span>
                  <span className="text-gray-900 dark:text-white">{email}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-gray-400" />
                  <span className="font-medium text-gray-700 dark:text-gray-300">Environment:</span>
                  <span className="text-gray-900 dark:text-white">{staging ? "Staging (Test)" : "Production"}</span>
                </div>
              </div>

              {requestStatus === "pending" && (
                <div className="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                  <div className="flex items-center gap-3">
                    <Loader2 className="h-5 w-5 animate-spin text-blue-500" />
                    <div>
                      <p className="font-medium text-blue-700 dark:text-blue-300">
                        Requesting Certificate...
                      </p>
                      <p className="text-sm text-blue-600 dark:text-blue-400">
                        This may take up to 60 seconds
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {/* Actions */}
              <div className="flex justify-between pt-4">
                <Button variant="secondary" onClick={() => goToStep("email")} disabled={loading} icon={ArrowLeft}>
                  Back
                </Button>
                <Button
                  variant="primary"
                  onClick={requestCertificate}
                  disabled={loading}
                  icon={loading ? Loader2 : ShieldCheck}
                >
                  Request Certificate
                </Button>
              </div>
            </div>
          )}

          {/* Step: Complete */}
          {step === "complete" && (
            <div className="space-y-4 text-center py-4">
              <div className="flex justify-center">
                <div className="w-16 h-16 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center">
                  <ShieldCheck className="h-8 w-8 text-green-600" />
                </div>
              </div>

              <div>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  SSL Certificate Issued!
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  {resultMessage || `SSL certificate has been issued for ${includeWWW ? `${domain} and www.${domain}` : domain}`}
                </p>
              </div>

              <div className="p-4 bg-green-50 dark:bg-green-900/20 rounded-lg border border-green-200 dark:border-green-800 text-sm text-left space-y-2">
                <p className="flex items-center gap-2 text-green-700 dark:text-green-400">
                  <Check className="h-4 w-4" />
                  <span>{sslSource === "external" ? "Certificate uploaded and installed" : "Certificate successfully issued by Let's Encrypt"}</span>
                </p>
                <p className="flex items-center gap-2 text-green-700 dark:text-green-400">
                  <Check className="h-4 w-4" />
                  <span>Nginx configuration updated</span>
                </p>
                {staging && (
                  <p className="flex items-center gap-2 text-yellow-600 dark:text-yellow-400">
                    <AlertTriangle className="h-4 w-4" />
                    <span>This is a staging certificate (not trusted by browsers)</span>
                  </p>
                )}
              </div>

              <p className="text-sm text-gray-500 dark:text-gray-400">
                Your site is now accessible via HTTPS at{" "}
                <a
                  href={`https://${domain}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary-600 hover:underline"
                >
                  https://{domain}
                </a>
              </p>

              <Button variant="primary" onClick={(e) => { e?.stopPropagation(); onOpenChange(false); }} className="mt-4">
                Done
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
