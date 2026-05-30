"use client";

import React, { useState, useEffect, Fragment, useCallback } from "react";
import { Dialog, Transition, Tab } from "@headlessui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  X,
  Rocket,
  Package,
  GitBranch,
  Loader2,
  CheckCircle,
  AlertTriangle,
  ExternalLink,
  ArrowRight,
  ArrowLeft,
  Plus,
  Trash2,
  Upload,
  Key,
  Lock,
  Network,
  HardDrive,
  Eye,
  EyeOff,
  Settings,
  Server,
  Check,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui/page-layout";
import {
  api,
  CreateDeploymentRequest,
  DeploymentContainerConfig,
  NetworkInfo,
  VolumeInfo,
} from "@/lib/api";

export interface DeployWizardProps {
  isOpen: boolean;
  onClose: () => void;
  imageRepository?: string;
  imageTag?: string;
  imageDigest?: string;
  registryId?: string;
  onSuccess?: (deploymentId: string) => void;
}

type WizardStep = "basics" | "environment" | "networks" | "volumes" | "review";
type DeployStage = "wizard" | "deploying" | "success" | "error";
type SecretMethod = "env_vars" | "docker_secrets";

interface EnvVar {
  key: string;
  value: string;
  masked: boolean;
}

interface NetworkConfig {
  networkId?: string;
  networkName: string;
  isNew?: boolean;
  driver?: string;
}

interface VolumeConfig {
  volumeName?: string;
  mountPath: string;
  isNew?: boolean;
  hostPath?: string;
  readOnly?: boolean;
}

const WIZARD_STEPS: { id: WizardStep; label: string; icon: React.ElementType }[] = [
  { id: "basics", label: "Basics", icon: Package },
  { id: "environment", label: "Environment", icon: Key },
  { id: "networks", label: "Networks", icon: Network },
  { id: "volumes", label: "Volumes", icon: HardDrive },
  { id: "review", label: "Review", icon: CheckCircle },
];

export function DeployWizard({
  isOpen,
  onClose,
  imageRepository,
  imageTag,
  imageDigest,
  registryId,
  onSuccess,
}: DeployWizardProps) {
  const queryClient = useQueryClient();

  // Wizard state
  const [currentStep, setCurrentStep] = useState<WizardStep>("basics");
  const [stage, setStage] = useState<DeployStage>("wizard");
  const [deploymentId, setDeploymentId] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  // Step 1: Basics
  const [serviceName, setServiceName] = useState("");
  const [environment, setEnvironment] = useState("dev");
  const [tag, setTag] = useState(imageTag || "latest");
  // When no imageRepository is pre-filled, user types the full image ref here (e.g. nginx:latest)
  const [imageInput, setImageInput] = useState("");
  // Source: deploy a prebuilt image, or build from a git repo (push-to-deploy).
  const [sourceMode, setSourceMode] = useState<"image" | "git">("image");
  const [gitRepo, setGitRepo] = useState("");
  const [gitBranch, setGitBranch] = useState("main");

  // Step 2: Environment
  const [envVars, setEnvVars] = useState<EnvVar[]>([]);
  const [secretMethod, setSecretMethod] = useState<SecretMethod>("env_vars");
  const [envFileContent, setEnvFileContent] = useState("");
  const [envTabIndex, setEnvTabIndex] = useState(0);

  // Step 3: Networks
  const [selectedNetworks, setSelectedNetworks] = useState<NetworkConfig[]>([]);
  const [newNetworkName, setNewNetworkName] = useState("");
  const [newNetworkDriver, setNewNetworkDriver] = useState("bridge");
  const [showNewNetworkForm, setShowNewNetworkForm] = useState(false);

  // Step 4: Volumes
  const [selectedVolumes, setSelectedVolumes] = useState<VolumeConfig[]>([]);
  const [newVolumeName, setNewVolumeName] = useState("");
  const [newVolumeMountPath, setNewVolumeMountPath] = useState("");
  const [newVolumeHostPath, setNewVolumeHostPath] = useState("");
  const [newVolumeReadOnly, setNewVolumeReadOnly] = useState(false);
  const [newVolumeIsBindMount, setNewVolumeIsBindMount] = useState(false);
  const [showNewVolumeForm, setShowNewVolumeForm] = useState(false);

  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const activeAgents = agents?.filter((a) => a.status === "active") || [];
  const defaultAgent = activeAgents[0];

  // Build-from-git (push-to-deploy) is available in CE. Only applies when not
  // launched pre-filled from a specific image (the images page).
  const isGit = sourceMode === "git" && !imageRepository;

  // Fetch networks for the default agent
  const { data: networks, isLoading: networksLoading } = useQuery({
    queryKey: ["networks", defaultAgent?.id],
    queryFn: () => api.getNetworks(defaultAgent!.id),
    enabled: !!defaultAgent?.id && isOpen,
  });

  // Fetch volumes for the default agent
  const { data: volumes, isLoading: volumesLoading } = useQuery({
    queryKey: ["volumes", defaultAgent?.id],
    queryFn: () => api.getDockerVolumes(defaultAgent!.id),
    enabled: !!defaultAgent?.id && isOpen,
  });

  // Reset state when dialog opens
  useEffect(() => {
    if (isOpen) {
      setCurrentStep("basics");
      setStage("wizard");
      setServiceName("");
      setEnvironment("dev");
      setTag(imageTag || "latest");
      setImageInput("");
      setSourceMode("image");
      setGitRepo("");
      setGitBranch("main");
      setEnvVars([]);
      setSecretMethod("env_vars");
      setEnvFileContent("");
      setEnvTabIndex(0);
      setParseWarning(null);
      setSelectedNetworks([]);
      setNewNetworkName("");
      setNewNetworkDriver("bridge");
      setShowNewNetworkForm(false);
      setSelectedVolumes([]);
      setNewVolumeName("");
      setNewVolumeMountPath("");
      setNewVolumeHostPath("");
      setNewVolumeReadOnly(false);
      setNewVolumeIsBindMount(false);
      setShowNewVolumeForm(false);
      setDeploymentId(null);
      setErrorMessage(null);
    }
  }, [isOpen, imageTag]);

  // Create deployment mutation
  const deployMutation = useMutation({
    mutationFn: (request: CreateDeploymentRequest) =>
      api.createDeployment(defaultAgent!.id, request),
    onSuccess: (deployment) => {
      setDeploymentId(deployment.id);
      setStage("success");
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      onSuccess?.(deployment.id);
    },
    onError: (error: Error) => {
      setErrorMessage(error.message);
      setStage("error");
    },
  });

  // Derive effective image repo + tag (either from props or from freeform imageInput)
  const effectiveRepo = imageRepository || imageInput.split(":")[0] || "";
  const effectiveTag = imageRepository
    ? (tag || "latest")
    : (imageInput.includes(":") ? imageInput.split(":").slice(1).join(":") || "latest" : tag || "latest");

  // Generate suggested service name
  const getSuggestedServiceName = () => {
    const src = isGit ? gitRepo : imageRepository || imageInput;
    if (!src) return "service";
    const parts = src.replace(/\.git$/, "").split("/");
    const name = (parts[parts.length - 1] || "service").split(":")[0];
    return name.replace(/[^a-zA-Z0-9-]/g, "-").toLowerCase();
  };

  // Parse .env file content
  const parseEnvContent = useCallback((content: string): { parsed: EnvVar[]; skipped: string[] } => {
    const lines = content.split("\n");
    const parsed: EnvVar[] = [];
    const skipped: string[] = [];

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#")) continue;

      // Find the first = sign to split key and value
      const eqIndex = trimmed.indexOf("=");
      if (eqIndex > 0) {
        const key = trimmed.substring(0, eqIndex).trim();
        let value = trimmed.substring(eqIndex + 1).trim();

        // Remove surrounding quotes if present
        if (
          (value.startsWith('"') && value.endsWith('"')) ||
          (value.startsWith("'") && value.endsWith("'"))
        ) {
          value = value.slice(1, -1);
        }

        // Validate key format (warn but still allow)
        if (key) {
          parsed.push({ key, value, masked: true });
        }
      } else {
        // Line doesn't contain =, skip it and record
        skipped.push(trimmed);
      }
    }

    return { parsed, skipped };
  }, []);

  // State for parse warnings
  const [parseWarning, setParseWarning] = useState<string | null>(null);

  // Handle .env file upload
  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (evt) => {
        const content = evt.target?.result as string;
        setEnvFileContent(content);
        const { parsed, skipped } = parseEnvContent(content);
        setEnvVars(parsed);
        setEnvTabIndex(0); // Switch to manual tab to show parsed vars
        if (skipped.length > 0) {
          setParseWarning(`${skipped.length} line(s) skipped (no KEY=value format): ${skipped.slice(0, 3).join(", ")}${skipped.length > 3 ? "..." : ""}`);
        } else {
          setParseWarning(null);
        }
      };
      reader.readAsText(file);
    }
  };

  // Handle paste .env content
  const handlePasteEnv = () => {
    const { parsed, skipped } = parseEnvContent(envFileContent);
    setEnvVars(parsed);
    setEnvTabIndex(0);
    if (skipped.length > 0) {
      setParseWarning(`${skipped.length} line(s) skipped (no KEY=value format): ${skipped.slice(0, 3).join(", ")}${skipped.length > 3 ? "..." : ""}`);
    } else {
      setParseWarning(null);
    }
  };

  // Handle paste in key input field - auto-parse KEY=VALUE format
  const handleKeyPaste = (e: React.ClipboardEvent<HTMLInputElement>, index: number) => {
    const pastedText = e.clipboardData.getData("text").trim();
    // Check if pasted text contains = (likely KEY=VALUE format)
    if (pastedText.includes("=")) {
      e.preventDefault();
      const { parsed } = parseEnvContent(pastedText);
      if (parsed.length > 0) {
        // Replace current empty row with first parsed, add rest
        const updated = [...envVars];
        updated[index] = parsed[0];
        // Add remaining parsed vars after current index
        for (let i = 1; i < parsed.length; i++) {
          updated.splice(index + i, 0, parsed[i]);
        }
        setEnvVars(updated);
      }
    }
  };

  // Add env var
  const addEnvVar = () => {
    setEnvVars([...envVars, { key: "", value: "", masked: true }]);
  };

  // Update env var
  const updateEnvVar = (index: number, field: "key" | "value", value: string) => {
    const updated = [...envVars];
    updated[index] = { ...updated[index], [field]: value };
    setEnvVars(updated);
  };

  // Toggle env var masking
  const toggleEnvVarMask = (index: number) => {
    const updated = [...envVars];
    updated[index] = { ...updated[index], masked: !updated[index].masked };
    setEnvVars(updated);
  };

  // Remove env var
  const removeEnvVar = (index: number) => {
    setEnvVars(envVars.filter((_, i) => i !== index));
  };

  // Toggle network selection
  const toggleNetworkSelection = (network: NetworkInfo) => {
    const existing = selectedNetworks.find((n) => n.networkId === network.id);
    if (existing) {
      setSelectedNetworks(selectedNetworks.filter((n) => n.networkId !== network.id));
    } else {
      setSelectedNetworks([
        ...selectedNetworks,
        { networkId: network.id, networkName: network.name },
      ]);
    }
  };

  // Add new network
  const addNewNetwork = () => {
    if (!newNetworkName.trim()) return;
    setSelectedNetworks([
      ...selectedNetworks,
      {
        networkName: newNetworkName.trim(),
        isNew: true,
        driver: newNetworkDriver,
      },
    ]);
    setNewNetworkName("");
    setNewNetworkDriver("bridge");
    setShowNewNetworkForm(false);
  };

  // Remove network
  const removeNetwork = (index: number) => {
    setSelectedNetworks(selectedNetworks.filter((_, i) => i !== index));
  };

  // Toggle volume selection
  const toggleVolumeSelection = (volume: VolumeInfo) => {
    const existing = selectedVolumes.find((v) => v.volumeName === volume.name);
    if (existing) {
      setSelectedVolumes(selectedVolumes.filter((v) => v.volumeName !== volume.name));
    } else {
      setSelectedVolumes([
        ...selectedVolumes,
        { volumeName: volume.name, mountPath: "" },
      ]);
    }
  };

  // Update volume mount path
  const updateVolumeMountPath = (index: number, mountPath: string) => {
    const updated = [...selectedVolumes];
    updated[index] = { ...updated[index], mountPath };
    setSelectedVolumes(updated);
  };

  // Add new volume
  const addNewVolume = () => {
    if (!newVolumeMountPath.trim()) return;
    if (!newVolumeIsBindMount && !newVolumeName.trim()) return;
    if (newVolumeIsBindMount && !newVolumeHostPath.trim()) return;

    setSelectedVolumes([
      ...selectedVolumes,
      {
        volumeName: newVolumeIsBindMount ? undefined : newVolumeName.trim(),
        mountPath: newVolumeMountPath.trim(),
        isNew: !newVolumeIsBindMount,
        hostPath: newVolumeIsBindMount ? newVolumeHostPath.trim() : undefined,
        readOnly: newVolumeReadOnly,
      },
    ]);
    setNewVolumeName("");
    setNewVolumeMountPath("");
    setNewVolumeHostPath("");
    setNewVolumeReadOnly(false);
    setNewVolumeIsBindMount(false);
    setShowNewVolumeForm(false);
  };

  // Remove volume
  const removeVolume = (index: number) => {
    setSelectedVolumes(selectedVolumes.filter((_, i) => i !== index));
  };

  // Navigation
  const stepIndex = WIZARD_STEPS.findIndex((s) => s.id === currentStep);

  const canGoNext = () => {
    switch (currentStep) {
      case "basics":
        return (
          serviceName.trim() !== "" &&
          !!defaultAgent &&
          (isGit ? gitRepo.trim() !== "" : !!imageRepository || imageInput.trim() !== "")
        );
      case "environment":
        return envVars.every((e) => e.key.trim() !== "");
      case "networks":
        return true;
      case "volumes":
        return selectedVolumes.every((v) => v.mountPath.trim() !== "");
      case "review":
        return true;
      default:
        return false;
    }
  };

  const goNext = () => {
    if (stepIndex < WIZARD_STEPS.length - 1) {
      setCurrentStep(WIZARD_STEPS[stepIndex + 1].id);
    }
  };

  const goBack = () => {
    if (stepIndex > 0) {
      setCurrentStep(WIZARD_STEPS[stepIndex - 1].id);
    }
  };

  // Deploy
  const handleDeploy = () => {
    if (!serviceName.trim() || !defaultAgent) return;

    setStage("deploying");
    setErrorMessage(null);

    // Build env vars map
    const envVarsMap: Record<string, string> = {};
    envVars.forEach((e) => {
      if (e.key.trim()) {
        envVarsMap[e.key.trim()] = e.value;
      }
    });

    // Build secrets array (for docker_secrets method)
    const secrets = secretMethod === "docker_secrets"
      ? envVars
          .filter((e) => e.key.trim())
          .map((e) => ({
            name: e.key.trim(),
            value: e.value,
            mount_path: `/run/secrets/${e.key.trim()}`,
          }))
      : undefined;

    // Build container_config
    const containerConfig: DeploymentContainerConfig = {};

    if (Object.keys(envVarsMap).length > 0) {
      if (secretMethod === "env_vars") {
        containerConfig.env_vars = envVarsMap;
      } else {
        containerConfig.secrets = secrets;
        containerConfig.secret_method = secretMethod;
      }
    }

    if (selectedNetworks.length > 0) {
      containerConfig.networks = selectedNetworks.map((n) => ({
        network_id: n.networkId,
        network_name: n.networkName,
        create_if_missing: n.isNew,
        driver: n.driver,
      }));
    }

    // Filter out volumes with empty or invalid mount paths
    const validVolumes = selectedVolumes.filter(
      (v) => v.mountPath.trim() !== "" && v.mountPath.trim() !== "/"
    );
    if (validVolumes.length > 0) {
      containerConfig.volumes = validVolumes.map((v) => ({
        volume_name: v.volumeName,
        mount_path: v.mountPath.trim(),
        create_if_missing: v.isNew,
        host_path: v.hostPath,
        read_only: v.readOnly,
      }));
    }

    const request: CreateDeploymentRequest = isGit
      ? {
          // Source build (push-to-deploy): no prebuilt image — the agent clones + builds.
          service_name: serviceName.trim(),
          environment,
          image_repository: "",
          git_repo: gitRepo.trim(),
          git_branch: gitBranch.trim() || "main",
          container_config:
            Object.keys(containerConfig).length > 0 ? containerConfig : undefined,
        }
      : {
          service_name: serviceName.trim(),
          environment,
          image_repository: effectiveRepo,
          image_tag: effectiveTag || undefined,
          image_digest: imageDigest || undefined,
          container_config:
            Object.keys(containerConfig).length > 0 ? containerConfig : undefined,
        };

    deployMutation.mutate(request);
  };

  const handleClose = () => {
    if (stage === "deploying") return;
    onClose();
  };

  const handleRetry = () => {
    setStage("wizard");
    setCurrentStep("review");
    setErrorMessage(null);
  };

  // Full image reference for display
  const fullImageRef = effectiveRepo ? `${effectiveRepo}:${effectiveTag}` : "";

  return (
    <Transition.Root show={isOpen} as={Fragment}>
      <Dialog as="div" className="relative z-50" onClose={handleClose}>
        <Transition.Child
          as={Fragment}
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-gray-900/50 dark:bg-gray-950/80 backdrop-blur-sm transition-opacity" />
        </Transition.Child>

        <div className="fixed inset-0 z-10 overflow-y-auto">
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
              <Dialog.Panel className="relative transform overflow-hidden rounded-xl bg-white dark:bg-gray-900 shadow-xl transition-all w-full max-w-2xl">
                {/* Header */}
                <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-lg bg-primary-100 dark:bg-primary-900/30">
                      <Rocket className="h-5 w-5 text-primary-600 dark:text-primary-400" />
                    </div>
                    <div>
                      <Dialog.Title className="text-lg font-semibold text-gray-900 dark:text-white">
                        {stage === "wizard" && (isGit ? "Build & Deploy from Git" : "Deploy Image")}
                        {stage === "deploying" && "Deploying..."}
                        {stage === "success" && "Deployment Created"}
                        {stage === "error" && "Deployment Failed"}
                      </Dialog.Title>
                    </div>
                  </div>
                  <button
                    onClick={handleClose}
                    disabled={stage === "deploying"}
                    className={cn(
                      "text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors",
                      stage === "deploying" && "opacity-50 cursor-not-allowed"
                    )}
                  >
                    <X className="h-5 w-5" />
                  </button>
                </div>

                {/* Wizard Steps Indicator */}
                {stage === "wizard" && (
                  <div className="px-6 py-3 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
                    <div className="flex items-center justify-between">
                      {WIZARD_STEPS.map((step, idx) => {
                        const StepIcon = step.icon;
                        const isActive = step.id === currentStep;
                        const isPast = idx < stepIndex;
                        return (
                          <div key={step.id} className="flex items-center">
                            <button
                              onClick={() => isPast && setCurrentStep(step.id)}
                              className={cn(
                                "flex items-center gap-2 px-3 py-1.5 rounded-lg transition-colors",
                                isActive && "bg-primary-100 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300",
                                isPast && "text-green-600 dark:text-green-400 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700",
                                !isActive && !isPast && "text-gray-400 cursor-not-allowed"
                              )}
                              disabled={!isPast && !isActive}
                            >
                              <StepIcon className={cn("h-4 w-4", isPast && "text-green-600 dark:text-green-400")} />
                              <span className="text-sm font-medium hidden sm:inline">{step.label}</span>
                            </button>
                            {idx < WIZARD_STEPS.length - 1 && (
                              <ArrowRight className="h-4 w-4 mx-1 text-gray-300 dark:text-gray-600" />
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Body */}
                <div className="px-6 py-4 max-h-[60vh] overflow-y-auto">
                  {/* Image Info — only shown when pre-filled from images page */}
                  {imageRepository && (
                    <div className="mb-4 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                      <div className="flex items-center gap-2 text-sm">
                        <Package className="h-4 w-4 text-gray-400" />
                        <span className="font-mono text-gray-900 dark:text-white truncate">
                          {fullImageRef}
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Wizard Steps */}
                  {stage === "wizard" && (
                    <>
                      {/* Step 1: Basics */}
                      {currentStep === "basics" && (
                        <div className="space-y-4">
                          {!defaultAgent && (
                            <div className="p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                              <div className="flex items-center gap-2 text-yellow-700 dark:text-yellow-400 text-sm">
                                <AlertTriangle className="h-4 w-4" />
                                <span>No active agents available. Please add an agent first.</span>
                              </div>
                            </div>
                          )}

                          {/* Source toggle — prebuilt image vs build from git (push-to-deploy) */}
                          {!imageRepository && (
                            <div>
                              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                                Source
                              </label>
                              <div className="grid grid-cols-2 gap-2">
                                <button
                                  type="button"
                                  onClick={() => setSourceMode("image")}
                                  className={cn(
                                    "flex items-center justify-center gap-2 py-2.5 rounded-lg border text-sm font-medium transition-colors",
                                    sourceMode === "image"
                                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300"
                                      : "border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                                  )}
                                >
                                  <Package className="w-4 h-4" /> Prebuilt image
                                </button>
                                <button
                                  type="button"
                                  onClick={() => setSourceMode("git")}
                                  className={cn(
                                    "flex items-center justify-center gap-2 py-2.5 rounded-lg border text-sm font-medium transition-colors",
                                    sourceMode === "git"
                                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300"
                                      : "border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                                  )}
                                >
                                  <GitBranch className="w-4 h-4" /> Build from Git
                                </button>
                              </div>
                            </div>
                          )}

                          {/* Image input — prebuilt-image mode, not pre-filled from images page */}
                          {!imageRepository && !isGit && (
                            <div>
                              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Image <span className="text-red-500">*</span>
                              </label>
                              <input
                                type="text"
                                value={imageInput}
                                onChange={(e) => setImageInput(e.target.value)}
                                placeholder="e.g. nginx:latest or myregistry.com/app:1.0"
                                className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 font-mono text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                              />
                              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                                Docker Hub image or full registry URL with optional tag
                              </p>
                            </div>
                          )}

                          {/* Git inputs — build-from-git mode */}
                          {isGit && (
                            <>
                              <div>
                                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                  Git repository URL <span className="text-red-500">*</span>
                                </label>
                                <input
                                  type="text"
                                  value={gitRepo}
                                  onChange={(e) => setGitRepo(e.target.value)}
                                  placeholder="https://github.com/you/app"
                                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 font-mono text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                                />
                                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                                  InfraPilot clones and builds (Nixpacks or your Dockerfile), then deploys — no CI required.
                                </p>
                              </div>
                              <div>
                                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                  Branch
                                </label>
                                <input
                                  type="text"
                                  value={gitBranch}
                                  onChange={(e) => setGitBranch(e.target.value)}
                                  placeholder="main"
                                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 font-mono text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                                />
                              </div>
                            </>
                          )}

                          <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                              Service Name <span className="text-red-500">*</span>
                            </label>
                            <input
                              type="text"
                              value={serviceName}
                              onChange={(e) => setServiceName(e.target.value)}
                              placeholder={getSuggestedServiceName()}
                              className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                            />
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                              A unique name for this deployment
                            </p>
                          </div>

                          <div>
                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                              Environment <span className="text-red-500">*</span>
                            </label>
                            <select
                              value={environment}
                              onChange={(e) => setEnvironment(e.target.value)}
                              className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                            >
                              <option value="dev">Development</option>
                              <option value="staging">Staging</option>
                              <option value="prod">Production</option>
                            </select>
                          </div>

                          {/* Tag field — shown when pre-filled from images page, or when imageInput has no tag */}
                          {!isGit && (imageRepository || !imageInput.includes(":")) && (
                            <div>
                              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                                Image Tag
                              </label>
                              <input
                                type="text"
                                value={tag}
                                onChange={(e) => setTag(e.target.value)}
                                placeholder="latest"
                                className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                              />
                            </div>
                          )}
                        </div>
                      )}

                      {/* Step 2: Environment */}
                      {currentStep === "environment" && (
                        <div className="space-y-4">
                          <Tab.Group selectedIndex={envTabIndex} onChange={setEnvTabIndex}>
                            <Tab.List className="flex space-x-1 rounded-lg bg-gray-100 dark:bg-gray-800 p-1">
                              <Tab
                                className={({ selected }) =>
                                  cn(
                                    "w-full rounded-md py-2 text-sm font-medium leading-5 transition-colors",
                                    selected
                                      ? "bg-white dark:bg-gray-700 text-primary-700 dark:text-primary-300 shadow"
                                      : "text-gray-600 dark:text-gray-400 hover:bg-white/50 dark:hover:bg-gray-700/50"
                                  )
                                }
                              >
                                Manual Entry
                              </Tab>
                              <Tab
                                className={({ selected }) =>
                                  cn(
                                    "w-full rounded-md py-2 text-sm font-medium leading-5 transition-colors",
                                    selected
                                      ? "bg-white dark:bg-gray-700 text-primary-700 dark:text-primary-300 shadow"
                                      : "text-gray-600 dark:text-gray-400 hover:bg-white/50 dark:hover:bg-gray-700/50"
                                  )
                                }
                              >
                                Import .env
                              </Tab>
                            </Tab.List>
                            <Tab.Panels className="mt-4">
                              {/* Manual Entry Tab */}
                              <Tab.Panel>
                                <div className="space-y-3">
                                  {envVars.map((envVar, idx) => (
                                    <div key={idx} className="flex items-center gap-2">
                                      <input
                                        type="text"
                                        value={envVar.key}
                                        onChange={(e) => updateEnvVar(idx, "key", e.target.value)}
                                        onPaste={(e) => handleKeyPaste(e, idx)}
                                        placeholder="KEY (or paste KEY=value)"
                                        className="flex-1 px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 text-sm font-mono"
                                      />
                                      <span className="text-gray-400">=</span>
                                      <div className="flex-1 relative">
                                        <input
                                          type={envVar.masked ? "password" : "text"}
                                          value={envVar.value}
                                          onChange={(e) => updateEnvVar(idx, "value", e.target.value)}
                                          placeholder="value"
                                          className="w-full px-3 py-2 pr-10 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 text-sm font-mono"
                                        />
                                        <button
                                          type="button"
                                          onClick={() => toggleEnvVarMask(idx)}
                                          className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                                        >
                                          {envVar.masked ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                                        </button>
                                      </div>
                                      <button
                                        type="button"
                                        onClick={() => removeEnvVar(idx)}
                                        className="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg"
                                      >
                                        <Trash2 className="h-4 w-4" />
                                      </button>
                                    </div>
                                  ))}
                                  <button
                                    type="button"
                                    onClick={addEnvVar}
                                    className="flex items-center gap-2 px-3 py-2 text-sm text-primary-600 dark:text-primary-400 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg transition-colors"
                                  >
                                    <Plus className="h-4 w-4" />
                                    Add Variable
                                  </button>
                                </div>
                              </Tab.Panel>

                              {/* Import .env Tab */}
                              <Tab.Panel>
                                <div className="space-y-4">
                                  <div className="border-2 border-dashed border-gray-300 dark:border-gray-700 rounded-lg p-6 text-center">
                                    <Upload className="h-8 w-8 text-gray-400 mx-auto mb-2" />
                                    <label className="cursor-pointer">
                                      <span className="text-primary-600 dark:text-primary-400 hover:underline">
                                        Upload .env file
                                      </span>
                                      <input
                                        type="file"
                                        accept=".env,.txt"
                                        onChange={handleFileUpload}
                                        className="hidden"
                                      />
                                    </label>
                                    <p className="text-xs text-gray-500 mt-1">or paste content below</p>
                                  </div>
                                  <textarea
                                    value={envFileContent}
                                    onChange={(e) => setEnvFileContent(e.target.value)}
                                    placeholder="KEY=value&#10;ANOTHER_KEY=another_value"
                                    className="w-full h-32 px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 text-sm font-mono"
                                  />
                                  <Button variant="secondary" size="sm" onClick={handlePasteEnv} disabled={!envFileContent.trim()}>
                                    Parse & Import
                                  </Button>
                                  {parseWarning && (
                                    <div className="mt-2 p-2 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                                      <p className="text-xs text-yellow-700 dark:text-yellow-400">
                                        {parseWarning}
                                      </p>
                                      <p className="text-xs text-yellow-600 dark:text-yellow-500 mt-1">
                                        Env vars must be in KEY=value format (one per line)
                                      </p>
                                    </div>
                                  )}
                                </div>
                              </Tab.Panel>
                            </Tab.Panels>
                          </Tab.Group>

                          {/* Secret Method Toggle */}
                          {envVars.length > 0 && (
                            <div className="mt-6 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                                Storage Method
                              </h4>
                              <div className="flex gap-4">
                                <label className="flex items-center gap-2 cursor-pointer">
                                  <input
                                    type="radio"
                                    name="secretMethod"
                                    value="env_vars"
                                    checked={secretMethod === "env_vars"}
                                    onChange={() => setSecretMethod("env_vars")}
                                    className="text-primary-600"
                                  />
                                  <span className="text-sm text-gray-700 dark:text-gray-300">Environment Variables</span>
                                </label>
                                <label className="flex items-center gap-2 cursor-pointer">
                                  <input
                                    type="radio"
                                    name="secretMethod"
                                    value="docker_secrets"
                                    checked={secretMethod === "docker_secrets"}
                                    onChange={() => setSecretMethod("docker_secrets")}
                                    className="text-primary-600"
                                  />
                                  <span className="text-sm text-gray-700 dark:text-gray-300">Docker Secrets</span>
                                  <Lock className="h-3 w-3 text-green-500" />
                                </label>
                              </div>
                              <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                                {secretMethod === "env_vars"
                                  ? "Variables will be set in the container environment. Visible in docker inspect."
                                  : "Secrets will be mounted as files at /run/secrets/. More secure, not visible in docker inspect."}
                              </p>
                            </div>
                          )}
                        </div>
                      )}

                      {/* Step 3: Networks */}
                      {currentStep === "networks" && (
                        <div className="space-y-4">
                          <p className="text-sm text-gray-600 dark:text-gray-400">
                            Select networks to attach to this container. The container will be connected to all selected networks.
                          </p>

                          {networksLoading ? (
                            <div className="flex items-center justify-center py-8">
                              <Loader2 className="h-6 w-6 animate-spin text-primary-500" />
                            </div>
                          ) : (
                            <div className="space-y-2">
                              {networks?.map((network) => (
                                <label
                                  key={network.id}
                                  className={cn(
                                    "flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-colors",
                                    selectedNetworks.some((n) => n.networkId === network.id)
                                      ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                                      : "border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800"
                                  )}
                                >
                                  <div className="flex items-center gap-3">
                                    <input
                                      type="checkbox"
                                      checked={selectedNetworks.some((n) => n.networkId === network.id)}
                                      onChange={() => toggleNetworkSelection(network)}
                                      className="text-primary-600 rounded"
                                    />
                                    <div>
                                      <span className="font-medium text-gray-900 dark:text-white">
                                        {network.name}
                                      </span>
                                      <span className="ml-2 text-xs text-gray-500">({network.driver})</span>
                                    </div>
                                  </div>
                                  {Object.keys(network.containers || {}).length > 0 && (
                                    <span className="text-xs text-gray-500">
                                      {Object.keys(network.containers).length} containers
                                    </span>
                                  )}
                                </label>
                              ))}
                            </div>
                          )}

                          {/* New networks to create */}
                          {selectedNetworks.filter((n) => n.isNew).length > 0 && (
                            <div className="mt-4">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                                New Networks to Create
                              </h4>
                              {selectedNetworks.filter((n) => n.isNew).map((network, idx) => (
                                <div
                                  key={idx}
                                  className="flex items-center justify-between p-3 rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800"
                                >
                                  <div>
                                    <span className="font-medium text-gray-900 dark:text-white">{network.networkName}</span>
                                    <span className="ml-2 text-xs text-gray-500">({network.driver})</span>
                                    <span className="ml-2 px-2 py-0.5 text-xs bg-green-100 dark:bg-green-900/50 text-green-700 dark:text-green-400 rounded">
                                      New
                                    </span>
                                  </div>
                                  <button
                                    type="button"
                                    onClick={() => removeNetwork(selectedNetworks.indexOf(network))}
                                    className="text-red-500 hover:text-red-700"
                                  >
                                    <Trash2 className="h-4 w-4" />
                                  </button>
                                </div>
                              ))}
                            </div>
                          )}

                          {/* Create new network form */}
                          {showNewNetworkForm ? (
                            <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg space-y-3">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300">Create New Network</h4>
                              <div className="flex gap-3">
                                <input
                                  type="text"
                                  value={newNetworkName}
                                  onChange={(e) => setNewNetworkName(e.target.value)}
                                  placeholder="Network name"
                                  className="flex-1 px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                                />
                                <select
                                  value={newNetworkDriver}
                                  onChange={(e) => setNewNetworkDriver(e.target.value)}
                                  className="px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                                >
                                  <option value="bridge">bridge</option>
                                  <option value="macvlan">macvlan</option>
                                </select>
                              </div>
                              <div className="flex gap-2">
                                <Button variant="primary" size="sm" onClick={addNewNetwork} disabled={!newNetworkName.trim()}>
                                  Add Network
                                </Button>
                                <Button variant="secondary" size="sm" onClick={() => setShowNewNetworkForm(false)}>
                                  Cancel
                                </Button>
                              </div>
                            </div>
                          ) : (
                            <button
                              type="button"
                              onClick={() => setShowNewNetworkForm(true)}
                              className="flex items-center gap-2 px-3 py-2 text-sm text-primary-600 dark:text-primary-400 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg transition-colors"
                            >
                              <Plus className="h-4 w-4" />
                              Create New Network
                            </button>
                          )}
                        </div>
                      )}

                      {/* Step 4: Volumes */}
                      {currentStep === "volumes" && (
                        <div className="space-y-4">
                          <p className="text-sm text-gray-600 dark:text-gray-400">
                            Select volumes to mount in this container. Specify the mount path for each volume.
                          </p>

                          {volumesLoading ? (
                            <div className="flex items-center justify-center py-8">
                              <Loader2 className="h-6 w-6 animate-spin text-primary-500" />
                            </div>
                          ) : (
                            <div className="space-y-2">
                              {volumes?.map((volume: VolumeInfo) => {
                                const isSelected = selectedVolumes.some((v) => v.volumeName === volume.name);
                                const selectedVol = selectedVolumes.find((v) => v.volumeName === volume.name);
                                return (
                                  <div
                                    key={volume.name}
                                    className={cn(
                                      "p-3 rounded-lg border transition-colors",
                                      isSelected
                                        ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                                        : "border-gray-200 dark:border-gray-700"
                                    )}
                                  >
                                    <label className="flex items-center gap-3 cursor-pointer">
                                      <input
                                        type="checkbox"
                                        checked={isSelected}
                                        onChange={() => toggleVolumeSelection(volume)}
                                        className="text-primary-600 rounded"
                                      />
                                      <div className="flex-1">
                                        <span className="font-medium text-gray-900 dark:text-white">
                                          {volume.name}
                                        </span>
                                        <span className="ml-2 text-xs text-gray-500">({volume.driver})</span>
                                      </div>
                                    </label>
                                    {isSelected && selectedVol && (
                                      <div className="mt-2 ml-7">
                                        <input
                                          type="text"
                                          value={selectedVol.mountPath}
                                          onChange={(e) =>
                                            updateVolumeMountPath(selectedVolumes.indexOf(selectedVol), e.target.value)
                                          }
                                          placeholder="Mount path (e.g., /data)"
                                          className="w-full px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                                        />
                                      </div>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          )}

                          {/* Selected new volumes */}
                          {selectedVolumes.filter((v) => v.isNew || v.hostPath).length > 0 && (
                            <div className="mt-4">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                                New Volumes / Bind Mounts
                              </h4>
                              {selectedVolumes.filter((v) => v.isNew || v.hostPath).map((volume, idx) => (
                                <div
                                  key={idx}
                                  className="flex items-center justify-between p-3 rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 mb-2"
                                >
                                  <div>
                                    <span className="font-medium text-gray-900 dark:text-white">
                                      {volume.hostPath || volume.volumeName}
                                    </span>
                                    <span className="mx-2 text-gray-400">→</span>
                                    <span className="font-mono text-sm">{volume.mountPath}</span>
                                    {volume.readOnly && (
                                      <span className="ml-2 px-2 py-0.5 text-xs bg-yellow-100 dark:bg-yellow-900/50 text-yellow-700 dark:text-yellow-400 rounded">
                                        read-only
                                      </span>
                                    )}
                                    <span className="ml-2 px-2 py-0.5 text-xs bg-green-100 dark:bg-green-900/50 text-green-700 dark:text-green-400 rounded">
                                      {volume.hostPath ? "Bind Mount" : "New Volume"}
                                    </span>
                                  </div>
                                  <button
                                    type="button"
                                    onClick={() => removeVolume(selectedVolumes.indexOf(volume))}
                                    className="text-red-500 hover:text-red-700"
                                  >
                                    <Trash2 className="h-4 w-4" />
                                  </button>
                                </div>
                              ))}
                            </div>
                          )}

                          {/* Create new volume form */}
                          {showNewVolumeForm ? (
                            <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg space-y-3">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300">Add Volume Mount</h4>

                              <label className="flex items-center gap-2 cursor-pointer">
                                <input
                                  type="checkbox"
                                  checked={newVolumeIsBindMount}
                                  onChange={(e) => setNewVolumeIsBindMount(e.target.checked)}
                                  className="text-primary-600 rounded"
                                />
                                <span className="text-sm text-gray-700 dark:text-gray-300">Bind mount (host path)</span>
                              </label>

                              {newVolumeIsBindMount ? (
                                <input
                                  type="text"
                                  value={newVolumeHostPath}
                                  onChange={(e) => setNewVolumeHostPath(e.target.value)}
                                  placeholder="Host path (e.g., /home/user/data)"
                                  className="w-full px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                                />
                              ) : (
                                <input
                                  type="text"
                                  value={newVolumeName}
                                  onChange={(e) => setNewVolumeName(e.target.value)}
                                  placeholder="Volume name"
                                  className="w-full px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                                />
                              )}

                              <input
                                type="text"
                                value={newVolumeMountPath}
                                onChange={(e) => setNewVolumeMountPath(e.target.value)}
                                placeholder="Mount path (e.g., /app/data)"
                                className="w-full px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                              />

                              <label className="flex items-center gap-2 cursor-pointer">
                                <input
                                  type="checkbox"
                                  checked={newVolumeReadOnly}
                                  onChange={(e) => setNewVolumeReadOnly(e.target.checked)}
                                  className="text-primary-600 rounded"
                                />
                                <span className="text-sm text-gray-700 dark:text-gray-300">Read-only</span>
                              </label>

                              <div className="flex gap-2">
                                <Button
                                  variant="primary"
                                  size="sm"
                                  onClick={addNewVolume}
                                  disabled={
                                    !newVolumeMountPath.trim() ||
                                    (newVolumeIsBindMount ? !newVolumeHostPath.trim() : !newVolumeName.trim())
                                  }
                                >
                                  Add Volume
                                </Button>
                                <Button variant="secondary" size="sm" onClick={() => setShowNewVolumeForm(false)}>
                                  Cancel
                                </Button>
                              </div>
                            </div>
                          ) : (
                            <button
                              type="button"
                              onClick={() => setShowNewVolumeForm(true)}
                              className="flex items-center gap-2 px-3 py-2 text-sm text-primary-600 dark:text-primary-400 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg transition-colors"
                            >
                              <Plus className="h-4 w-4" />
                              Add Volume Mount
                            </button>
                          )}
                        </div>
                      )}

                      {/* Step 5: Review */}
                      {currentStep === "review" && (
                        <div className="space-y-4">
                          {/* Basics Summary */}
                          <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                            <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3 flex items-center gap-2">
                              <Package className="h-4 w-4" />
                              Basics
                            </h4>
                            <dl className="grid grid-cols-2 gap-2 text-sm">
                              <dt className="text-gray-500">Service Name:</dt>
                              <dd className="text-gray-900 dark:text-white font-medium">{serviceName}</dd>
                              <dt className="text-gray-500">Environment:</dt>
                              <dd className="text-gray-900 dark:text-white font-medium capitalize">{environment}</dd>
                              {isGit ? (
                                <>
                                  <dt className="text-gray-500">Source:</dt>
                                  <dd className="text-gray-900 dark:text-white font-mono text-xs flex items-center gap-1">
                                    <GitBranch className="h-3 w-3 shrink-0" />
                                    {gitRepo} @ {gitBranch.trim() || "main"}
                                  </dd>
                                  <dt className="text-gray-500">Build:</dt>
                                  <dd className="text-gray-900 dark:text-white text-xs">Nixpacks / Dockerfile</dd>
                                </>
                              ) : (
                                <>
                                  <dt className="text-gray-500">Image:</dt>
                                  <dd className="text-gray-900 dark:text-white font-mono text-xs">{fullImageRef}</dd>
                                </>
                              )}
                            </dl>
                          </div>

                          {/* Environment Summary */}
                          {envVars.length > 0 && (
                            <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3 flex items-center gap-2">
                                <Key className="h-4 w-4" />
                                Environment ({envVars.length} variables)
                                <span className="ml-auto text-xs text-gray-500">
                                  {secretMethod === "docker_secrets" ? "Docker Secrets" : "Env Vars"}
                                </span>
                              </h4>
                              <div className="space-y-1">
                                {envVars.slice(0, 5).map((env, idx) => (
                                  <div key={idx} className="text-sm font-mono text-gray-600 dark:text-gray-400">
                                    {env.key}=****
                                  </div>
                                ))}
                                {envVars.length > 5 && (
                                  <div className="text-xs text-gray-500">
                                    ... and {envVars.length - 5} more
                                  </div>
                                )}
                              </div>
                            </div>
                          )}

                          {/* Networks Summary */}
                          {selectedNetworks.length > 0 && (
                            <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3 flex items-center gap-2">
                                <Network className="h-4 w-4" />
                                Networks ({selectedNetworks.length})
                              </h4>
                              <div className="space-y-1">
                                {selectedNetworks.map((net, idx) => (
                                  <div key={idx} className="text-sm text-gray-600 dark:text-gray-400 flex items-center gap-2">
                                    <span>{net.networkName}</span>
                                    {net.isNew && (
                                      <span className="px-1.5 py-0.5 text-xs bg-green-100 dark:bg-green-900/50 text-green-700 dark:text-green-400 rounded">
                                        new
                                      </span>
                                    )}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          {/* Volumes Summary */}
                          {selectedVolumes.length > 0 && (
                            <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                              <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3 flex items-center gap-2">
                                <HardDrive className="h-4 w-4" />
                                Volumes ({selectedVolumes.length})
                              </h4>
                              <div className="space-y-1">
                                {selectedVolumes.map((vol, idx) => (
                                  <div key={idx} className="text-sm text-gray-600 dark:text-gray-400">
                                    <span className="font-mono">{vol.hostPath || vol.volumeName}</span>
                                    <span className="mx-1">→</span>
                                    <span className="font-mono">{vol.mountPath}</span>
                                    {vol.readOnly && <span className="ml-1 text-xs">(ro)</span>}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          {/* Deploy Target */}
                          <div className="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                            <div className="flex items-center gap-2 text-sm text-blue-700 dark:text-blue-400">
                              <Server className="h-4 w-4" />
                              <span>
                                Deploying to: <strong>{defaultAgent?.name || defaultAgent?.hostname}</strong>
                              </span>
                            </div>
                          </div>
                        </div>
                      )}
                    </>
                  )}

                  {/* Deploying Stage */}
                  {stage === "deploying" && (
                    <div className="py-8 text-center">
                      <Loader2 className="h-12 w-12 text-primary-600 dark:text-primary-400 animate-spin mx-auto mb-4" />
                      <p className="text-gray-600 dark:text-gray-400">
                        Creating deployment and starting security scan...
                      </p>
                    </div>
                  )}

                  {/* Success Stage */}
                  {stage === "success" && (
                    <div className="py-6 text-center">
                      <div className="w-12 h-12 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center mx-auto mb-4">
                        <CheckCircle className="h-6 w-6 text-green-600 dark:text-green-400" />
                      </div>
                      <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                        Deployment Created
                      </h3>
                      <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
                        Your deployment has been created and the security scan pipeline has started.
                      </p>
                      <a
                        href="/docker/deployments"
                        className="inline-flex items-center gap-1 text-sm text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300"
                      >
                        View Deployments
                        <ExternalLink className="h-3.5 w-3.5" />
                      </a>
                    </div>
                  )}

                  {/* Error Stage */}
                  {stage === "error" && (
                    <div className="py-6">
                      <div className="w-12 h-12 rounded-full bg-red-100 dark:bg-red-900/30 flex items-center justify-center mx-auto mb-4">
                        <AlertTriangle className="h-6 w-6 text-red-600 dark:text-red-400" />
                      </div>
                      <h3 className="text-lg font-medium text-gray-900 dark:text-white text-center mb-2">
                        Deployment Failed
                      </h3>
                      <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {errorMessage || "An unexpected error occurred"}
                        </p>
                      </div>
                    </div>
                  )}
                </div>

                {/* Footer */}
                <div className="flex items-center justify-between gap-3 px-6 py-4 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
                  {stage === "wizard" && (
                    <>
                      <div>
                        {stepIndex > 0 && (
                          <Button variant="secondary" onClick={goBack}>
                            <ArrowLeft className="h-4 w-4 mr-1" />
                            Back
                          </Button>
                        )}
                      </div>
                      <div className="flex gap-3">
                        <Button variant="secondary" onClick={handleClose}>
                          Cancel
                        </Button>
                        {currentStep === "review" ? (
                          <Button
                            variant="primary"
                            onClick={handleDeploy}
                            disabled={!serviceName.trim() || !defaultAgent || (isGit ? !gitRepo.trim() : !effectiveRepo)}
                          >
                            <Rocket className="h-4 w-4 mr-1" />
                            {isGit ? "Build & Deploy" : "Deploy"}
                          </Button>
                        ) : (
                          <Button variant="primary" onClick={goNext} disabled={!canGoNext()}>
                            Next
                            <ArrowRight className="h-4 w-4 ml-1" />
                          </Button>
                        )}
                      </div>
                    </>
                  )}
                  {stage === "deploying" && (
                    <div className="flex-1 flex justify-end">
                      <Button variant="secondary" disabled>
                        <Loader2 className="h-4 w-4 mr-1 animate-spin" />
                        Deploying...
                      </Button>
                    </div>
                  )}
                  {stage === "success" && (
                    <div className="flex-1 flex justify-end">
                      <Button variant="primary" onClick={handleClose}>
                        Done
                      </Button>
                    </div>
                  )}
                  {stage === "error" && (
                    <div className="flex-1 flex justify-end gap-3">
                      <Button variant="secondary" onClick={handleClose}>
                        Close
                      </Button>
                      <Button variant="primary" onClick={handleRetry}>
                        Try Again
                      </Button>
                    </div>
                  )}
                </div>
              </Dialog.Panel>
            </Transition.Child>
          </div>
        </div>
      </Dialog>
    </Transition.Root>
  );
}
