"use client";

import React, { useState, useEffect, Fragment, useCallback } from "react";
import { Dialog, Transition } from "@headlessui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  X,
  Layers,
  FileCode,
  FileText,
  Variable,
  Server,
  Network,
  HardDrive,
  CheckCircle,
  AlertTriangle,
  ArrowRight,
  ArrowLeft,
  Loader2,
  Upload,
  Eye,
  EyeOff,
  Plus,
  Trash2,
  RefreshCw,
  ClipboardPaste,
  Download,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui/page-layout";
import {
  api,
  ParsedCompose,
  ComposeService,
  ComposeVariable,
  ComposeNetwork,
  ComposeVolume,
  CreateStackRequest,
  ServiceOverride,
  StackProgress,
} from "@/lib/api";

export interface StackDeployWizardProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: (stackId: string) => void;
}

type WizardStep = "yaml" | "variables" | "services" | "resources" | "files" | "review";
type DeployStage = "wizard" | "deploying" | "success" | "error";

const WIZARD_STEPS: { id: WizardStep; label: string; icon: React.ElementType }[] = [
  { id: "yaml", label: "YAML", icon: FileCode },
  { id: "variables", label: "Variables", icon: Variable },
  { id: "services", label: "Services", icon: Server },
  { id: "resources", label: "Resources", icon: Network },
  { id: "files", label: "Files", icon: FileText },
  { id: "review", label: "Review", icon: CheckCircle },
];

interface ServiceConfig {
  enabled: boolean;
  tagOverride?: string;
  envOverrides: Record<string, string>;
}

export function StackDeployWizard({
  isOpen,
  onClose,
  onSuccess,
}: StackDeployWizardProps) {
  const queryClient = useQueryClient();

  // Wizard state
  const [currentStep, setCurrentStep] = useState<WizardStep>("yaml");
  const [stage, setStage] = useState<DeployStage>("wizard");
  const [stackId, setStackId] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  // Step 1: YAML
  const [composeYaml, setComposeYaml] = useState("");
  const [stackName, setStackName] = useState("");
  const [environment, setEnvironment] = useState("dev");
  const [parseError, setParseError] = useState<string | null>(null);
  const [parsedCompose, setParsedCompose] = useState<ParsedCompose | null>(null);

  // v3/45 — companion files: content for each relative bind mount the compose ships (init scripts,
  // entrypoints, configs), keyed by the required file's normalized path.
  const [companionFiles, setCompanionFiles] = useState<Record<string, { content: string; executable: boolean }>>({});

  // Step 2: Variables
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [envInputMethod, setEnvInputMethod] = useState<"manual" | "file">("manual");
  const [envFileContent, setEnvFileContent] = useState("");
  const [envParseWarning, setEnvParseWarning] = useState<string | null>(null);

  // Step 3: Services
  const [serviceConfigs, setServiceConfigs] = useState<Record<string, ServiceConfig>>({});
  const [serviceEnvFiles, setServiceEnvFiles] = useState<Record<string, string>>({});
  const [expandedEnvSections, setExpandedEnvSections] = useState<Record<string, boolean>>({});

  // Step 5: Review
  const [skipScanning, setSkipScanning] = useState(false);

  // Progress tracking
  const [progress, setProgress] = useState<StackProgress | null>(null);

  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const activeAgents = agents?.filter((a) => a.status === "active") || [];
  const defaultAgent = activeAgents[0];

  // Parse mutation
  const parseMutation = useMutation({
    mutationFn: () =>
      api.parseComposeYAML(defaultAgent!.id, {
        compose_yaml: composeYaml,
        variables,
      }),
    onSuccess: (result) => {
      setParsedCompose(result);
      setParseError(null);

      // Initialize variables from parsed compose
      const newVars: Record<string, string> = {};
      result.variables.forEach((v) => {
        newVars[v.name] = variables[v.name] || v.default || "";
      });
      setVariables(newVars);

      // Initialize service configs
      const newConfigs: Record<string, ServiceConfig> = {};
      result.services.forEach((s) => {
        newConfigs[s.name] = serviceConfigs[s.name] || {
          enabled: true,
          tagOverride: undefined,
          envOverrides: {},
        };
      });
      setServiceConfigs(newConfigs);

      if (result.errors && result.errors.length > 0) {
        setParseError(result.errors.join("\n"));
      }
    },
    onError: (error: Error) => {
      setParseError(error.message);
      setParsedCompose(null);
    },
  });

  // Create stack mutation
  const createStackMutation = useMutation({
    mutationFn: (request: CreateStackRequest) =>
      api.createStack(defaultAgent!.id, request),
    onSuccess: (result) => {
      setStackId(result.id);
      setStage("deploying");
      // Start polling for progress
      startPolling(result.id);
    },
    onError: (error: Error) => {
      setErrorMessage(error.message);
      setStage("error");
    },
  });

  // Progress polling
  const startPolling = useCallback((id: string) => {
    const poll = async () => {
      try {
        const progress = await api.getStackProgress(defaultAgent!.id, id);
        setProgress(progress);

        if (progress.status === "running" || progress.status === "partial") {
          setStage("success");
          queryClient.invalidateQueries({ queryKey: ["managed-stacks"] });
          queryClient.invalidateQueries({ queryKey: ["deployments"] });
          onSuccess?.(id);
        } else if (progress.status === "failed") {
          setErrorMessage("Stack deployment failed");
          setStage("error");
        } else if (progress.status === "deploying" || progress.status === "pending") {
          // Continue polling
          setTimeout(poll, 2000);
        }
      } catch (error) {
        console.error("Failed to poll progress:", error);
        setTimeout(poll, 2000);
      }
    };
    poll();
  }, [defaultAgent, queryClient, onSuccess]);

  // Reset state when dialog opens
  useEffect(() => {
    if (isOpen) {
      setCurrentStep("yaml");
      setStage("wizard");
      setComposeYaml("");
      setStackName("");
      setEnvironment("dev");
      setParseError(null);
      setParsedCompose(null);
      setCompanionFiles({});
      setVariables({});
      setEnvInputMethod("manual");
      setEnvFileContent("");
      setEnvParseWarning(null);
      setServiceConfigs({});
      setServiceEnvFiles({});
      setExpandedEnvSections({});
      setSkipScanning(false);
      setStackId(null);
      setErrorMessage(null);
      setProgress(null);
    }
  }, [isOpen]);

  // Parse .env file content and update variables
  const parseEnvFile = (content: string) => {
    const parsed: Record<string, string> = {};
    const lines = content.split("\n");
    let matched = 0;
    let total = 0;

    for (const line of lines) {
      const trimmed = line.trim();
      // Skip empty lines and comments
      if (!trimmed || trimmed.startsWith("#")) continue;

      // Parse KEY=VALUE format
      const match = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
      if (match) {
        total++;
        const [, key, value] = match;
        // Remove surrounding quotes if present
        let cleanValue = value.trim();
        if ((cleanValue.startsWith('"') && cleanValue.endsWith('"')) ||
            (cleanValue.startsWith("'") && cleanValue.endsWith("'"))) {
          cleanValue = cleanValue.slice(1, -1);
        }

        // Only set if this variable is expected by the compose file
        if (parsedCompose?.variables.some((v) => v.name === key)) {
          parsed[key] = cleanValue;
          matched++;
        }
      }
    }

    // Update variables with parsed values (merge with existing)
    setVariables((prev) => ({ ...prev, ...parsed }));

    // Show warning if some variables weren't matched
    if (total > 0 && matched < total) {
      setEnvParseWarning(`Matched ${matched} of ${total} variables from .env file`);
    } else if (matched > 0) {
      setEnvParseWarning(null);
    }
  };

  // Handle .env file upload
  const handleEnvFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (event) => {
        const content = event.target?.result as string;
        setEnvFileContent(content);
        parseEnvFile(content);
      };
      reader.readAsText(file);
    }
  };

  // File upload handler
  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (event) => {
        const content = event.target?.result as string;
        setComposeYaml(content);
        // Auto-set stack name from filename
        if (!stackName) {
          const name = file.name.replace(/\.(yml|yaml)$/i, "");
          setStackName(name.replace(/[^a-zA-Z0-9-]/g, "-").toLowerCase());
        }
      };
      reader.readAsText(file);
    }
  };

  // Parse per-service .env file content
  const parseServiceEnvFile = (serviceName: string, content: string) => {
    const parsed: Record<string, string> = {};
    for (const line of content.split("\n")) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#")) continue;
      const match = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
      if (match) {
        let value = match[2].trim();
        if ((value.startsWith('"') && value.endsWith('"')) ||
            (value.startsWith("'") && value.endsWith("'"))) {
          value = value.slice(1, -1);
        }
        parsed[match[1]] = value;
      }
    }
    setServiceConfigs(prev => ({
      ...prev,
      [serviceName]: {
        ...prev[serviceName],
        envOverrides: { ...prev[serviceName]?.envOverrides, ...parsed },
      },
    }));
  };

  // Handle per-service .env file upload
  const handleServiceEnvFileUpload = (serviceName: string, e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (event) => {
        const content = event.target?.result as string;
        setServiceEnvFiles(prev => ({ ...prev, [serviceName]: content }));
        parseServiceEnvFile(serviceName, content);
      };
      reader.readAsText(file);
    }
  };

  // Import global variables into a service's env overrides
  const importGlobalVarsToService = (serviceName: string) => {
    const globalVars: Record<string, string> = {};
    for (const [key, value] of Object.entries(variables)) {
      if (value) {
        globalVars[key] = value;
      }
    }
    setServiceConfigs(prev => ({
      ...prev,
      [serviceName]: {
        ...prev[serviceName],
        envOverrides: { ...prev[serviceName]?.envOverrides, ...globalVars },
      },
    }));
  };

  // Remove a per-service env var
  const removeServiceEnvVar = (serviceName: string, key: string) => {
    setServiceConfigs(prev => {
      const newOverrides = { ...prev[serviceName].envOverrides };
      delete newOverrides[key];
      return {
        ...prev,
        [serviceName]: {
          ...prev[serviceName],
          envOverrides: newOverrides,
        },
      };
    });
  };

  // Navigation
  const canProceed = () => {
    switch (currentStep) {
      case "yaml":
        return composeYaml.trim() !== "" && stackName.trim() !== "" && parsedCompose !== null;
      case "variables": {
        // Check that all required variables (no default) have values
        if (!parsedCompose?.variables) return true;
        const missingRequired = parsedCompose.variables.filter(
          (v) => v.required && !v.default && !variables[v.name]
        );
        return missingRequired.length === 0;
      }
      case "services":
        return Object.values(serviceConfigs).some((c) => c.enabled);
      case "resources":
        return true;
      case "files": {
        // Fail-closed: every relative bind-mount source must be supplied before deploy (v3/45).
        const required = parsedCompose?.required_files ?? [];
        return required.every((rf) => (companionFiles[rf.path]?.content ?? "").length > 0);
      }
      case "review":
        return true;
      default:
        return false;
    }
  };

  const handleNext = () => {
    const steps = WIZARD_STEPS.map((s) => s.id);
    const currentIndex = steps.indexOf(currentStep);

    // Re-parse compose with variables when moving from Variables -> Services
    if (currentStep === "variables" && steps[currentIndex + 1] === "services") {
      parseMutation.mutate();
    }

    if (currentIndex < steps.length - 1) {
      setCurrentStep(steps[currentIndex + 1]);
    }
  };

  const handleBack = () => {
    const steps = WIZARD_STEPS.map((s) => s.id);
    const currentIndex = steps.indexOf(currentStep);
    if (currentIndex > 0) {
      setCurrentStep(steps[currentIndex - 1]);
    }
  };

  const handleDeploy = () => {
    if (!defaultAgent) return;

    const overrides: ServiceOverride[] = Object.entries(serviceConfigs).map(
      ([serviceName, config]) => ({
        service_name: serviceName,
        tag_override: config.tagOverride,
        env_overrides: Object.keys(config.envOverrides).length > 0 ? config.envOverrides : undefined,
        enabled: config.enabled,
      })
    );

    // v3/45 — companion files: send the content for each relative bind mount the compose ships.
    const files = (parsedCompose?.required_files ?? [])
      .map((rf) => {
        const f = companionFiles[rf.path];
        return f ? { path: rf.path, content: f.content, mode: f.executable ? "0755" : "0644" } : null;
      })
      .filter((f): f is { path: string; content: string; mode: string } => f !== null);

    createStackMutation.mutate({
      name: stackName,
      environment,
      compose_yaml: composeYaml,
      variables: Object.keys(variables).length > 0 ? variables : undefined,
      overrides: overrides.length > 0 ? overrides : undefined,
      skip_scanning: skipScanning,
      files: files.length > 0 ? files : undefined,
    });
  };

  // Service config helpers
  const toggleService = (serviceName: string) => {
    setServiceConfigs((prev) => ({
      ...prev,
      [serviceName]: {
        ...prev[serviceName],
        enabled: !prev[serviceName].enabled,
      },
    }));
  };

  const updateServiceTag = (serviceName: string, tag: string) => {
    setServiceConfigs((prev) => ({
      ...prev,
      [serviceName]: {
        ...prev[serviceName],
        tagOverride: tag || undefined,
      },
    }));
  };

  const updateServiceEnv = (serviceName: string, key: string, value: string) => {
    setServiceConfigs((prev) => ({
      ...prev,
      [serviceName]: {
        ...prev[serviceName],
        envOverrides: {
          ...prev[serviceName].envOverrides,
          [key]: value,
        },
      },
    }));
  };

  // Count enabled services
  const enabledServiceCount = Object.values(serviceConfigs).filter((c) => c.enabled).length;

  // Render step content
  const renderStepContent = () => {
    switch (currentStep) {
      case "yaml":
        return (
          <div className="space-y-4">
            <div className="flex gap-4">
              <div className="flex-1">
                <label className="block text-sm font-medium text-zinc-300 mb-1">
                  Stack Name
                </label>
                <Input
                  value={stackName}
                  onChange={(e) => setStackName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-"))}
                  placeholder="my-application"
                />
              </div>
              <div className="w-40">
                <label className="block text-sm font-medium text-zinc-300 mb-1">
                  Environment
                </label>
                <select
                  value={environment}
                  onChange={(e) => setEnvironment(e.target.value)}
                  className="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-white"
                >
                  <option value="dev">Development</option>
                  <option value="staging">Staging</option>
                  <option value="prod">Production</option>
                </select>
              </div>
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="text-sm font-medium text-zinc-300">
                  Docker Compose YAML
                </label>
                <label className="cursor-pointer text-sm text-indigo-400 hover:text-indigo-300 flex items-center gap-1">
                  <Upload className="h-4 w-4" />
                  Upload File
                  <input
                    type="file"
                    accept=".yml,.yaml"
                    onChange={handleFileUpload}
                    className="hidden"
                  />
                </label>
              </div>
              <textarea
                value={composeYaml}
                onChange={(e) => setComposeYaml(e.target.value)}
                placeholder="Paste your docker-compose.yml content here..."
                className="w-full h-64 px-3 py-2 bg-zinc-900 border border-zinc-700 rounded-lg text-white font-mono text-sm resize-none focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>

            <div className="flex justify-end">
              <Button
                onClick={() => parseMutation.mutate()}
                disabled={!composeYaml.trim() || parseMutation.isPending}
              >
                {parseMutation.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin mr-2" />
                    Parsing...
                  </>
                ) : (
                  <>
                    <CheckCircle className="h-4 w-4 mr-2" />
                    Validate YAML
                  </>
                )}
              </Button>
            </div>

            {parseError && (
              <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                <div className="flex items-start gap-2">
                  <AlertTriangle className="h-4 w-4 mt-0.5 flex-shrink-0" />
                  <pre className="whitespace-pre-wrap">{parseError}</pre>
                </div>
              </div>
            )}

            {parsedCompose && !parseError && (
              <div className="p-3 bg-green-500/10 border border-green-500/30 rounded-lg text-green-400 text-sm">
                <div className="flex items-center gap-2">
                  <CheckCircle className="h-4 w-4" />
                  <span>
                    Found {parsedCompose.services.length} services,{" "}
                    {parsedCompose.networks.length} networks,{" "}
                    {parsedCompose.volumes.length} volumes
                  </span>
                </div>
              </div>
            )}
          </div>
        );

      case "variables":
        return (
          <div className="space-y-4">
            {/* Input method tabs */}
            <div className="flex gap-2 p-1 bg-zinc-800/50 rounded-lg">
              <button
                onClick={() => setEnvInputMethod("manual")}
                className={cn(
                  "flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm transition-colors",
                  envInputMethod === "manual"
                    ? "bg-zinc-700 text-white"
                    : "text-zinc-400 hover:text-white"
                )}
              >
                <Variable className="h-4 w-4" />
                Manual Entry
              </button>
              <button
                onClick={() => setEnvInputMethod("file")}
                className={cn(
                  "flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm transition-colors",
                  envInputMethod === "file"
                    ? "bg-zinc-700 text-white"
                    : "text-zinc-400 hover:text-white"
                )}
              >
                <FileText className="h-4 w-4" />
                From .env File
              </button>
            </div>

            {envInputMethod === "file" ? (
              <div className="space-y-3">
                <p className="text-sm text-zinc-400">
                  Upload or paste your <code className="text-indigo-400">.env</code> file.
                  Matching variables will be automatically populated.
                </p>

                <div className="flex items-center gap-2">
                  <label className="cursor-pointer flex items-center gap-2 px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-300 hover:bg-zinc-700 transition-colors">
                    <Upload className="h-4 w-4" />
                    Upload .env
                    <input
                      type="file"
                      accept=".env,.env.*"
                      onChange={handleEnvFileUpload}
                      className="hidden"
                    />
                  </label>
                  <span className="text-zinc-500 text-sm">or paste below</span>
                </div>

                <textarea
                  value={envFileContent}
                  onChange={(e) => {
                    setEnvFileContent(e.target.value);
                    parseEnvFile(e.target.value);
                  }}
                  placeholder={`# Paste your .env content here\nPOSTGRES_PASSWORD=secret123\nJWT_SECRET=my-jwt-secret\nREDIS_PASSWORD=redis-pass`}
                  className="w-full h-40 px-3 py-2 bg-zinc-900 border border-zinc-700 rounded-lg text-white font-mono text-sm resize-none focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />

                {envParseWarning && (
                  <div className="p-2 bg-amber-500/10 border border-amber-500/30 rounded-lg text-amber-400 text-sm flex items-center gap-2">
                    <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                    {envParseWarning}
                  </div>
                )}

                {Object.keys(variables).filter((k) => variables[k]).length > 0 && (
                  <div className="p-3 bg-green-500/10 border border-green-500/30 rounded-lg text-green-400 text-sm">
                    <div className="flex items-center gap-2">
                      <CheckCircle className="h-4 w-4" />
                      <span>
                        {Object.keys(variables).filter((k) => variables[k]).length} variables populated
                      </span>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <>
                <p className="text-sm text-zinc-400">
                  Set values for variables found in your compose file. Variables use the format{" "}
                  <code className="text-indigo-400">${"{VARIABLE}"}</code> or{" "}
                  <code className="text-indigo-400">${"{VARIABLE:-default}"}</code>.
                </p>

                {parsedCompose?.variables && parsedCompose.variables.length > 0 ? (
                  <div className="space-y-3 max-h-64 overflow-y-auto pr-2">
                    {parsedCompose.variables
                      .sort((a, b) => (a.required === b.required ? 0 : a.required ? -1 : 1))
                      .map((v) => (
                      <div key={v.name} className="flex items-center gap-3">
                        <div className="w-48 flex-shrink-0">
                          <span className="text-sm font-mono text-zinc-300">
                            {v.name}
                            {v.required && <span className="text-red-400 ml-1">*</span>}
                          </span>
                          {v.default && (
                            <span className="text-xs text-zinc-500 block truncate">
                              default: {v.default}
                            </span>
                          )}
                          {v.required && !v.default && (
                            <span className="text-xs text-red-400 block">required</span>
                          )}
                        </div>
                        <Input
                          value={variables[v.name] || ""}
                          onChange={(e) =>
                            setVariables((prev) => ({ ...prev, [v.name]: e.target.value }))
                          }
                          placeholder={v.default || (v.required ? "Required" : "Enter value...")}
                          className={`flex-1 ${v.required && !variables[v.name] && !v.default ? "border-red-500/50" : ""}`}
                        />
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-center py-8 text-zinc-500">
                    No variables found in your compose file.
                  </div>
                )}
              </>
            )}

            <div className="pt-4 border-t border-zinc-800">
              <p className="text-xs text-zinc-500">
                Common variables: <code>REGISTRY</code> (e.g., ghcr.io/myorg),{" "}
                <code>TAG</code> (e.g., latest, v1.0.0)
              </p>
            </div>
          </div>
        );

      case "services":
        return (
          <div className="space-y-4">
            <p className="text-sm text-zinc-400">
              Enable or disable services and optionally override tags or environment variables.
            </p>

            <div className="space-y-3">
              {parsedCompose?.services.map((service) => {
                const config = serviceConfigs[service.name];
                if (!config) return null;

                return (
                  <div
                    key={service.name}
                    className={cn(
                      "p-4 rounded-lg border transition-colors",
                      config.enabled
                        ? "bg-zinc-800/50 border-zinc-700"
                        : "bg-zinc-900/50 border-zinc-800 opacity-50"
                    )}
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-3">
                        <button
                          onClick={() => toggleService(service.name)}
                          className={cn(
                            "w-5 h-5 rounded border flex items-center justify-center",
                            config.enabled
                              ? "bg-indigo-500 border-indigo-500"
                              : "bg-transparent border-zinc-600"
                          )}
                        >
                          {config.enabled && <CheckCircle className="h-3 w-3 text-white" />}
                        </button>
                        <div>
                          <h4 className="font-medium text-white">{service.name}</h4>
                          <p className="text-sm text-zinc-400 font-mono">{service.image}</p>
                        </div>
                      </div>
                      <span className="text-xs text-zinc-500">Order: {service.order + 1}</span>
                    </div>

                    {config.enabled && (
                      <div className="mt-3 pt-3 border-t border-zinc-700 space-y-3">
                        <div className="flex items-center gap-2">
                          <label className="text-xs text-zinc-400 w-20">Tag Override:</label>
                          <Input
                            value={config.tagOverride || ""}
                            onChange={(e) => updateServiceTag(service.name, e.target.value)}
                            placeholder="e.g., v1.2.3"
                            className="flex-1 text-sm"
                          />
                        </div>

                        {service.depends_on && service.depends_on.length > 0 && (
                          <p className="text-xs text-zinc-500">
                            Depends on: {service.depends_on.join(", ")}
                          </p>
                        )}

                        {/* Resolved environment variables from YAML */}
                        {service.environment && Object.keys(service.environment).length > 0 && (
                          <div className="pt-2 border-t border-zinc-700/50">
                            <button
                              onClick={() => setExpandedEnvSections(prev => ({
                                ...prev,
                                [`${service.name}-env`]: !prev[`${service.name}-env`],
                              }))}
                              className="flex items-center gap-1.5 text-xs text-zinc-400 hover:text-zinc-300 transition-colors w-full"
                            >
                              {expandedEnvSections[`${service.name}-env`] ? (
                                <ChevronDown className="h-3.5 w-3.5" />
                              ) : (
                                <ChevronRight className="h-3.5 w-3.5" />
                              )}
                              <Variable className="h-3.5 w-3.5" />
                              <span>{Object.keys(service.environment).length} environment variable{Object.keys(service.environment).length !== 1 ? "s" : ""} from YAML</span>
                            </button>
                            {expandedEnvSections[`${service.name}-env`] && (
                              <div className="mt-2 space-y-1 max-h-40 overflow-y-auto pl-5">
                                {Object.entries(service.environment).map(([key, value]) => (
                                  <div key={key} className="flex items-center gap-2 text-xs">
                                    <span className="font-mono text-zinc-400 w-40 truncate flex-shrink-0">{key}</span>
                                    <span className="font-mono text-zinc-500 truncate">
                                      {value || <span className="italic text-zinc-600">empty</span>}
                                    </span>
                                  </div>
                                ))}
                              </div>
                            )}
                          </div>
                        )}

                        {/* Per-service env_file section */}
                        {service.env_files && service.env_files.length > 0 && (
                          <div className="pt-2 border-t border-zinc-700/50 space-y-2">
                            <div className="flex items-center gap-2">
                              <FileText className="h-3.5 w-3.5 text-zinc-400" />
                              <span className="text-xs text-zinc-400">
                                env_file:{" "}
                                <span className="text-zinc-300 font-mono">
                                  {service.env_files.join(", ")}
                                </span>
                              </span>
                            </div>

                            <div className="flex items-center gap-2 flex-wrap">
                              {Object.keys(variables).filter(k => variables[k]).length > 0 && (
                                <button
                                  onClick={() => importGlobalVarsToService(service.name)}
                                  className="flex items-center gap-1.5 px-2.5 py-1.5 bg-indigo-500/10 border border-indigo-500/30 rounded text-xs text-indigo-400 hover:bg-indigo-500/20 transition-colors"
                                >
                                  <Download className="h-3.5 w-3.5" />
                                  Import from Global Variables ({Object.keys(variables).filter(k => variables[k]).length})
                                </button>
                              )}
                              <label className="cursor-pointer flex items-center gap-1.5 px-2.5 py-1.5 bg-zinc-800 border border-zinc-700 rounded text-xs text-zinc-300 hover:bg-zinc-700 transition-colors">
                                <Upload className="h-3.5 w-3.5" />
                                Upload .env
                                <input
                                  type="file"
                                  accept=".env,.env.*"
                                  onChange={(e) => handleServiceEnvFileUpload(service.name, e)}
                                  className="hidden"
                                />
                              </label>
                              <span className="text-zinc-500 text-xs">or paste below</span>
                            </div>

                            <textarea
                              value={serviceEnvFiles[service.name] || ""}
                              onChange={(e) => {
                                const content = e.target.value;
                                setServiceEnvFiles(prev => ({ ...prev, [service.name]: content }));
                                parseServiceEnvFile(service.name, content);
                              }}
                              placeholder="# Paste .env content here&#10;DB_HOST=localhost&#10;DB_PASSWORD=secret"
                              className="w-full h-24 px-2.5 py-2 bg-zinc-900 border border-zinc-700 rounded text-white font-mono text-xs resize-none focus:outline-none focus:ring-1 focus:ring-indigo-500"
                            />

                            {Object.keys(config.envOverrides).length > 0 && (
                              <div className="space-y-1">
                                <div className="flex items-center gap-1.5 text-xs text-green-400">
                                  <CheckCircle className="h-3.5 w-3.5" />
                                  {Object.keys(config.envOverrides).length} variable{Object.keys(config.envOverrides).length !== 1 ? "s" : ""} loaded
                                </div>
                                <div className="space-y-1 max-h-32 overflow-y-auto">
                                  {Object.entries(config.envOverrides).map(([key, value]) => (
                                    <div key={key} className="flex items-center gap-2">
                                      <span className="text-xs font-mono text-zinc-400 w-32 truncate flex-shrink-0">{key}</span>
                                      <Input
                                        value={value}
                                        onChange={(e) => updateServiceEnv(service.name, key, e.target.value)}
                                        className="flex-1 text-xs h-7"
                                      />
                                      <button
                                        onClick={() => removeServiceEnvVar(service.name, key)}
                                        className="p-1 text-zinc-500 hover:text-red-400 transition-colors"
                                      >
                                        <Trash2 className="h-3 w-3" />
                                      </button>
                                    </div>
                                  ))}
                                </div>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>

            <div className="text-sm text-zinc-400">
              {enabledServiceCount} of {parsedCompose?.services.length || 0} services enabled
            </div>
          </div>
        );

      case "resources":
        return (
          <div className="space-y-6">
            <div>
              <h4 className="text-sm font-medium text-zinc-300 flex items-center gap-2 mb-3">
                <Network className="h-4 w-4" />
                Networks
              </h4>
              {parsedCompose?.networks && parsedCompose.networks.length > 0 ? (
                <div className="space-y-2">
                  {parsedCompose.networks.map((net) => (
                    <div
                      key={net.name}
                      className="flex items-center justify-between p-3 bg-zinc-800/50 rounded-lg border border-zinc-700"
                    >
                      <div>
                        <span className="text-white">{net.name}</span>
                        {net.driver && (
                          <span className="text-xs text-zinc-500 ml-2">({net.driver})</span>
                        )}
                      </div>
                      {net.external ? (
                        <span className="text-xs text-amber-400">External</span>
                      ) : (
                        <span className="text-xs text-green-400">Will be created</span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-zinc-500">No custom networks defined.</p>
              )}
            </div>

            <div>
              <h4 className="text-sm font-medium text-zinc-300 flex items-center gap-2 mb-3">
                <HardDrive className="h-4 w-4" />
                Volumes
              </h4>
              {parsedCompose?.volumes && parsedCompose.volumes.length > 0 ? (
                <div className="space-y-2">
                  {parsedCompose.volumes.map((vol) => (
                    <div
                      key={vol.name}
                      className="flex items-center justify-between p-3 bg-zinc-800/50 rounded-lg border border-zinc-700"
                    >
                      <div>
                        <span className="text-white">{vol.name}</span>
                        {vol.driver && (
                          <span className="text-xs text-zinc-500 ml-2">({vol.driver})</span>
                        )}
                      </div>
                      {vol.external ? (
                        <span className="text-xs text-amber-400">External</span>
                      ) : (
                        <span className="text-xs text-green-400">Will be created</span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-zinc-500">No named volumes defined.</p>
              )}
            </div>

            <div className="p-3 bg-zinc-800/50 rounded-lg border border-zinc-700">
              <p className="text-sm text-zinc-400">
                Networks and volumes marked as "external" must already exist on the host.
                Others will be created automatically during deployment.
              </p>
            </div>
          </div>
        );

      case "files": {
        // v3/45 — companion files: supply each relative bind-mount source (init scripts, entrypoints,
        // configs) the compose ships. Fail-closed: Deploy is blocked until every one has content.
        const required = parsedCompose?.required_files ?? [];
        const setFile = (path: string, source: string, content: string) =>
          setCompanionFiles((p) => ({
            ...p,
            [path]: { content, executable: p[path]?.executable ?? source.endsWith(".sh") },
          }));
        return (
          <div className="space-y-4">
            <div className="flex items-start gap-2 text-sm text-zinc-300">
              <FileText className="h-4 w-4 mt-0.5 flex-shrink-0" />
              <span>
                This compose bind-mounts local files (scripts, entrypoints, configs). Paste or upload
                each one — they are written to the host and mounted when the stack deploys.
              </span>
            </div>
            {required.length > 0 ? (
              <div className="space-y-3">
                {required.map((rf) => {
                  const cur = companionFiles[rf.path];
                  const provided = (cur?.content ?? "").length > 0;
                  return (
                    <div key={rf.path} className="p-3 bg-zinc-800/50 rounded-lg border border-zinc-700 space-y-2">
                      <div className="flex items-center justify-between gap-2">
                        <div className="min-w-0">
                          <span className="font-mono text-sm text-white">{rf.source}</span>
                          <div className="text-[11px] text-zinc-500 truncate">
                            {rf.service} → <span className="font-mono">{rf.target}</span>
                          </div>
                        </div>
                        <div className="flex items-center gap-3 flex-shrink-0">
                          {provided ? (
                            <CheckCircle className="h-4 w-4 text-green-500" />
                          ) : (
                            <span className="text-[11px] text-amber-400">required</span>
                          )}
                          <label className="flex items-center gap-1.5 text-xs text-zinc-400 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={cur?.executable ?? rf.source.endsWith(".sh")}
                              onChange={(e) =>
                                setCompanionFiles((p) => ({
                                  ...p,
                                  [rf.path]: { content: p[rf.path]?.content ?? "", executable: e.target.checked },
                                }))
                              }
                            />
                            Executable
                          </label>
                        </div>
                      </div>
                      <textarea
                        value={cur?.content ?? ""}
                        onChange={(e) => setFile(rf.path, rf.source, e.target.value)}
                        placeholder={`Paste the contents of ${rf.source}…`}
                        rows={5}
                        className="w-full rounded border border-zinc-700 bg-zinc-900 px-2 py-1 text-xs font-mono text-white"
                      />
                      <label className="flex items-center gap-2 text-xs text-zinc-400 cursor-pointer">
                        <Upload className="h-3.5 w-3.5" />
                        <span>Upload file</span>
                        <input
                          type="file"
                          className="hidden"
                          onChange={async (e) => {
                            const file = e.target.files?.[0];
                            if (file) setFile(rf.path, rf.source, await file.text());
                            e.target.value = ""; // allow re-selecting the same file
                          }}
                        />
                      </label>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-zinc-500">This stack bind-mounts no local files — nothing to supply.</p>
            )}
          </div>
        );
      }

      case "review":
        return (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="p-4 bg-zinc-800/50 rounded-lg border border-zinc-700">
                <h4 className="text-xs text-zinc-500 uppercase mb-1">Stack Name</h4>
                <p className="text-white font-medium">{stackName}</p>
              </div>
              <div className="p-4 bg-zinc-800/50 rounded-lg border border-zinc-700">
                <h4 className="text-xs text-zinc-500 uppercase mb-1">Environment</h4>
                <p className="text-white font-medium capitalize">{environment}</p>
              </div>
            </div>

            <div className="p-4 bg-zinc-800/50 rounded-lg border border-zinc-700">
              <h4 className="text-xs text-zinc-500 uppercase mb-2">Services to Deploy</h4>
              <div className="space-y-2">
                {parsedCompose?.services
                  .filter((s) => serviceConfigs[s.name]?.enabled)
                  .map((service) => {
                    const envCount = Object.keys(serviceConfigs[service.name]?.envOverrides || {}).length;
                    return (
                      <div key={service.name}>
                        <div className="flex items-center justify-between text-sm">
                          <span className="text-white">{service.name}</span>
                          <span className="text-zinc-400 font-mono text-xs">
                            {serviceConfigs[service.name]?.tagOverride
                              ? `${service.image.split(":")[0]}:${serviceConfigs[service.name].tagOverride}`
                              : service.image}
                          </span>
                        </div>
                        {envCount > 0 && (
                          <p className="text-xs text-zinc-500 mt-0.5 ml-2">
                            {envCount} env variable{envCount !== 1 ? "s" : ""} configured
                          </p>
                        )}
                      </div>
                    );
                  })}
              </div>
            </div>

            {Object.keys(variables).length > 0 && (
              <div className="p-4 bg-zinc-800/50 rounded-lg border border-zinc-700">
                <h4 className="text-xs text-zinc-500 uppercase mb-2">Variables</h4>
                <div className="space-y-1">
                  {Object.entries(variables)
                    .filter(([_, v]) => v)
                    .map(([key, value]) => (
                      <div key={key} className="flex items-center justify-between text-sm">
                        <span className="text-zinc-400 font-mono">{key}</span>
                        <span className="text-white">{value}</span>
                      </div>
                    ))}
                </div>
              </div>
            )}

            <div className="p-4 bg-zinc-800/50 rounded-lg border border-zinc-700">
              <h4 className="text-xs text-zinc-500 uppercase mb-3">Security Options</h4>
              <label className="flex items-start gap-3 cursor-pointer group">
                <div className="relative flex items-center">
                  <input
                    type="checkbox"
                    checked={!skipScanning}
                    onChange={(e) => setSkipScanning(!e.target.checked)}
                    className="w-5 h-5 rounded border-zinc-600 bg-zinc-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500 focus:ring-offset-0 cursor-pointer"
                  />
                </div>
                <div className="flex-1">
                  <div className="text-sm font-medium text-white group-hover:text-indigo-300 transition-colors">
                    Security Scanning
                    {environment === "prod" && (
                      <span className="ml-2 text-xs font-normal text-amber-400">(Recommended for Production)</span>
                    )}
                  </div>
                  <p className="text-xs text-zinc-400 mt-0.5">
                    Scan images for vulnerabilities and generate SBOMs before deployment.
                    {skipScanning ? (
                      <>
                        <span className="text-amber-400 block mt-1">⚠ Scanning disabled - images will deploy immediately</span>
                        {environment === "prod" && (
                          <span className="text-red-400 block mt-1 font-medium">
                            🚨 WARNING: Deploying to production without security scanning is not recommended
                          </span>
                        )}
                      </>
                    ) : (
                      <span className="text-zinc-500 block mt-1">Adds ~2-3 minutes per service</span>
                    )}
                  </p>
                </div>
              </label>
            </div>

            <div className="p-4 bg-indigo-500/10 border border-indigo-500/30 rounded-lg">
              <p className="text-sm text-indigo-300">
                <strong>{enabledServiceCount}</strong> services will be deployed in order based on
                their dependencies.{!skipScanning && " Each service will be scanned for vulnerabilities before deployment."}
              </p>
            </div>
          </div>
        );

      default:
        return null;
    }
  };

  // Render deploying/success/error stages
  const renderDeployingStage = () => (
    <div className="py-8 text-center">
      <Loader2 className="h-12 w-12 animate-spin text-indigo-500 mx-auto mb-4" />
      <h3 className="text-lg font-medium text-white mb-2">Deploying Stack...</h3>
      <p className="text-zinc-400 mb-6">
        {progress?.running_count || 0} of {progress?.service_count || enabledServiceCount} services
        deployed
      </p>

      {progress && (
        <div className="space-y-2 text-left max-w-sm mx-auto">
          {progress.services.map((service) => (
            <div
              key={service.name}
              className="flex items-center justify-between p-2 bg-zinc-800/50 rounded"
            >
              <span className="text-sm text-white">{service.name}</span>
              <span
                className={cn(
                  "text-xs px-2 py-0.5 rounded",
                  service.status === "running"
                    ? "bg-green-500/20 text-green-400"
                    : service.status === "failed"
                    ? "bg-red-500/20 text-red-400"
                    : service.status === "scanning" || service.status === "deploying"
                    ? "bg-blue-500/20 text-blue-400"
                    : "bg-zinc-700 text-zinc-400"
                )}
              >
                {service.status}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );

  const renderSuccessStage = () => (
    <div className="py-8 text-center">
      <CheckCircle className="h-12 w-12 text-green-500 mx-auto mb-4" />
      <h3 className="text-lg font-medium text-white mb-2">Stack Deployed!</h3>
      <p className="text-zinc-400 mb-6">
        {progress?.running_count || enabledServiceCount} services are now running.
      </p>

      {progress && progress.failed_count > 0 && (
        <div className="p-3 bg-amber-500/10 border border-amber-500/30 rounded-lg text-amber-400 text-sm mb-4">
          <AlertTriangle className="h-4 w-4 inline mr-2" />
          {progress.failed_count} service(s) failed to deploy
        </div>
      )}

      <Button onClick={onClose}>Close</Button>
    </div>
  );

  const renderErrorStage = () => (
    <div className="py-8 text-center">
      <AlertTriangle className="h-12 w-12 text-red-500 mx-auto mb-4" />
      <h3 className="text-lg font-medium text-white mb-2">Deployment Failed</h3>
      <p className="text-zinc-400 mb-6">{errorMessage || "An unexpected error occurred."}</p>
      <div className="flex justify-center gap-3">
        <Button variant="secondary" onClick={() => setStage("wizard")}>
          Try Again
        </Button>
        <Button onClick={onClose}>Close</Button>
      </div>
    </div>
  );

  return (
    <Transition.Root show={isOpen} as={Fragment}>
      <Dialog as="div" className="relative z-50" onClose={onClose}>
        <Transition.Child
          as={Fragment}
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black/70 backdrop-blur-sm" />
        </Transition.Child>

        <div className="fixed inset-0 overflow-y-auto">
          <div className="flex min-h-full items-center justify-center p-4">
            <Transition.Child
              as={Fragment}
              enter="ease-out duration-300"
              enterFrom="opacity-0 scale-95"
              enterTo="opacity-100 scale-100"
              leave="ease-in duration-200"
              leaveFrom="opacity-100 scale-100"
              leaveTo="opacity-0 scale-95"
            >
              <Dialog.Panel className="w-full max-w-2xl bg-zinc-900 rounded-xl shadow-xl border border-zinc-800">
                {/* Header */}
                <div className="flex items-center justify-between p-4 border-b border-zinc-800">
                  <div className="flex items-center gap-3">
                    <div className="p-2 bg-indigo-500/20 rounded-lg">
                      <Layers className="h-5 w-5 text-indigo-400" />
                    </div>
                    <Dialog.Title className="text-lg font-semibold text-white">
                      Deploy Stack
                    </Dialog.Title>
                  </div>
                  <button
                    onClick={onClose}
                    className="p-1 rounded hover:bg-zinc-800 text-zinc-400 hover:text-white"
                  >
                    <X className="h-5 w-5" />
                  </button>
                </div>

                {stage === "wizard" && (
                  <>
                    {/* Step indicators */}
                    <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
                      {WIZARD_STEPS.map((step, index) => {
                        const Icon = step.icon;
                        const isActive = currentStep === step.id;
                        const isPast =
                          WIZARD_STEPS.findIndex((s) => s.id === currentStep) > index;

                        return (
                          <div key={step.id} className="flex items-center">
                            <div
                              className={cn(
                                "flex items-center gap-2 px-3 py-1.5 rounded-full text-sm transition-colors",
                                isActive
                                  ? "bg-indigo-500/20 text-indigo-400"
                                  : isPast
                                  ? "text-zinc-300"
                                  : "text-zinc-500"
                              )}
                            >
                              <Icon className="h-4 w-4" />
                              <span className="hidden sm:inline">{step.label}</span>
                            </div>
                            {index < WIZARD_STEPS.length - 1 && (
                              <ArrowRight className="h-4 w-4 text-zinc-600 mx-1" />
                            )}
                          </div>
                        );
                      })}
                    </div>

                    {/* Content */}
                    <div className="p-6 min-h-[300px]">{renderStepContent()}</div>

                    {/* Footer */}
                    <div className="flex items-center justify-between p-4 border-t border-zinc-800">
                      <Button
                        variant="secondary"
                        onClick={handleBack}
                        disabled={currentStep === "yaml"}
                      >
                        <ArrowLeft className="h-4 w-4 mr-2" />
                        Back
                      </Button>

                      {currentStep === "review" ? (
                        <Button
                          onClick={handleDeploy}
                          disabled={createStackMutation.isPending || !defaultAgent}
                        >
                          {createStackMutation.isPending ? (
                            <>
                              <Loader2 className="h-4 w-4 animate-spin mr-2" />
                              Deploying...
                            </>
                          ) : (
                            <>
                              <Layers className="h-4 w-4 mr-2" />
                              Deploy Stack
                            </>
                          )}
                        </Button>
                      ) : (
                        <Button onClick={handleNext} disabled={!canProceed()}>
                          Next
                          <ArrowRight className="h-4 w-4 ml-2" />
                        </Button>
                      )}
                    </div>
                  </>
                )}

                {stage === "deploying" && (
                  <div className="p-6">{renderDeployingStage()}</div>
                )}

                {stage === "success" && <div className="p-6">{renderSuccessStage()}</div>}

                {stage === "error" && <div className="p-6">{renderErrorStage()}</div>}
              </Dialog.Panel>
            </Transition.Child>
          </div>
        </div>
      </Dialog>
    </Transition.Root>
  );
}
