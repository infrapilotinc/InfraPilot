import { redirect } from "next/navigation";

// "Services" and "Apps" were two near-duplicate views of the same data
// (api.listServices). Apps is the canonical, nav-linked page — consolidate here.
export default function ServicesPage() {
  redirect("/docker/apps");
}
