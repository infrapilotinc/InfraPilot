"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function DeploymentDetailPage() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/docker/deployments");
  }, [router]);
  return null;
}
