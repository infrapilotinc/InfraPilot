"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Building2,
  Users,
  Key,
  Trash2,
  Copy,
  Plus,
  Check,
  X,
  RefreshCw,
  AlertTriangle,
} from "lucide-react";
import Link from "next/link";
import {
  api,
  Organization,
  OrganizationMember,
  EnrollmentToken,
  OrgMemberRole,
} from "@/lib/api";
import { useAuthStore } from "@/lib/auth";

export default function OrgSettingsPage() {
  const params = useParams();
  const router = useRouter();
  const orgId = params.id as string;
  const { user } = useAuthStore();

  const [org, setOrg] = useState<Organization | null>(null);
  const [members, setMembers] = useState<OrganizationMember[]>([]);
  const [tokens, setTokens] = useState<EnrollmentToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<"general" | "members" | "tokens">(
    "general"
  );

  // Form states
  const [orgName, setOrgName] = useState("");
  const [saving, setSaving] = useState(false);

  // Invite modal
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<OrgMemberRole>("member");

  // Token modal
  const [showTokenModal, setShowTokenModal] = useState(false);
  const [tokenName, setTokenName] = useState("");
  const [tokenMaxUses, setTokenMaxUses] = useState<number | undefined>();
  const [tokenExpiry, setTokenExpiry] = useState("");
  const [newToken, setNewToken] = useState<string | null>(null);

  // Delete confirmation
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  useEffect(() => {
    loadData();
  }, [orgId]);

  const loadData = async () => {
    try {
      setLoading(true);
      const [orgData, membersData, tokensData] = await Promise.all([
        api.getOrganization(orgId),
        api.getOrganizationMembers(orgId),
        api.getEnrollmentTokens(orgId),
      ]);
      setOrg(orgData);
      setOrgName(orgData.name);
      setMembers(membersData || []);
      setTokens(tokensData || []);
    } catch (error) {
      console.error("Failed to load organization:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateOrg = async () => {
    if (!org || !orgName.trim()) return;
    try {
      setSaving(true);
      await api.updateOrganization(orgId, { name: orgName.trim() });
      setOrg({ ...org, name: orgName.trim() });
    } catch (error) {
      console.error("Failed to update organization:", error);
    } finally {
      setSaving(false);
    }
  };

  const handleInviteMember = async () => {
    if (!inviteEmail.trim()) return;
    try {
      await api.createOrganizationInvitation(orgId, {
        email: inviteEmail.trim(),
        role: inviteRole,
      });
      setShowInviteModal(false);
      setInviteEmail("");
      setInviteRole("member");
      loadData();
    } catch (error) {
      console.error("Failed to invite member:", error);
    }
  };

  const handleRemoveMember = async (userId: string) => {
    try {
      await api.removeOrganizationMember(orgId, userId);
      loadData();
    } catch (error) {
      console.error("Failed to remove member:", error);
    }
  };

  const handleUpdateMemberRole = async (
    userId: string,
    role: OrgMemberRole
  ) => {
    try {
      await api.updateOrganizationMember(orgId, userId, role);
      loadData();
    } catch (error) {
      console.error("Failed to update member role:", error);
    }
  };

  const handleCreateToken = async () => {
    if (!tokenName.trim()) return;
    try {
      const result = await api.createEnrollmentToken(orgId, {
        name: tokenName.trim(),
        max_uses: tokenMaxUses,
        expires_at: tokenExpiry || undefined,
      });
      setNewToken(result.token || null);
      setTokenName("");
      setTokenMaxUses(undefined);
      setTokenExpiry("");
      loadData();
    } catch (error) {
      console.error("Failed to create token:", error);
    }
  };

  const handleRevokeToken = async (tokenId: string) => {
    try {
      await api.revokeEnrollmentToken(orgId, tokenId);
      loadData();
    } catch (error) {
      console.error("Failed to revoke token:", error);
    }
  };

  const handleDeleteOrg = async () => {
    if (deleteConfirmText !== org?.name) return;
    try {
      await api.deleteOrganization(orgId);
      router.push("/");
    } catch (error) {
      console.error("Failed to delete organization:", error);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (!org) {
    return (
      <div className="h-full flex flex-col items-center justify-center gap-4">
        <p className="text-gray-500">Organization not found</p>
        <Link href="/" className="text-primary-600 hover:underline">
          Go back home
        </Link>
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-4xl mx-auto py-6 px-4">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <Link
            href="/"
            className="p-2 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
              Organization Settings
            </h1>
            <p className="text-sm text-gray-500">{org.name}</p>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700 mb-6">
          {[
            { id: "general", label: "General", icon: Building2 },
            { id: "members", label: "Members", icon: Users },
            { id: "tokens", label: "Enrollment Tokens", icon: Key },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as typeof activeTab)}
              className={`flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                activeTab === tab.id
                  ? "border-primary-600 text-primary-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
              }`}
            >
              <tab.icon className="h-4 w-4" />
              {tab.label}
            </button>
          ))}
        </div>

        {/* General Tab */}
        {activeTab === "general" && (
          <div className="space-y-6">
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-6">
              <h2 className="text-lg font-semibold mb-4">
                Organization Details
              </h2>

              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Name
                  </label>
                  <input
                    type="text"
                    value={orgName}
                    onChange={(e) => setOrgName(e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Slug
                  </label>
                  <input
                    type="text"
                    value={org.slug}
                    disabled
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-gray-100 dark:bg-gray-700 text-gray-500"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Slug cannot be changed
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Plan
                  </label>
                  <div className="px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg">
                    <span className="capitalize">{org.plan}</span>
                  </div>
                </div>

                <button
                  onClick={handleUpdateOrg}
                  disabled={saving || orgName === org.name}
                  className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save Changes"}
                </button>
              </div>
            </div>

            {/* Danger Zone */}
            <div className="bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800 p-6">
              <h2 className="text-lg font-semibold text-red-600 dark:text-red-400 mb-2">
                Danger Zone
              </h2>
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
                Deleting an organization will permanently remove all associated
                data including agents, proxies, and configurations.
              </p>
              <button
                onClick={() => setShowDeleteConfirm(true)}
                className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700"
              >
                Delete Organization
              </button>
            </div>
          </div>
        )}

        {/* Members Tab */}
        {activeTab === "members" && (
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <h2 className="text-lg font-semibold">Team Members</h2>
              <button
                onClick={() => setShowInviteModal(true)}
                className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
              >
                <Plus className="h-4 w-4" />
                Invite Member
              </button>
            </div>

            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 divide-y divide-gray-200 dark:divide-gray-700">
              {members.map((member) => (
                <div
                  key={member.id}
                  className="flex items-center justify-between p-4"
                >
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-primary-600 flex items-center justify-center text-white font-medium">
                      {(member.email || member.user_name)?.[0]?.toUpperCase() || "U"}
                    </div>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        {member.email || member.user_name || "Unknown"}
                      </p>
                      <p className="text-sm text-gray-500">
                        Joined{" "}
                        {new Date(member.joined_at).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <select
                      value={member.role}
                      onChange={(e) =>
                        handleUpdateMemberRole(
                          member.user_id,
                          e.target.value as OrgMemberRole
                        )
                      }
                      disabled={member.user_id === user?.id}
                      className="px-3 py-1 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-sm disabled:opacity-50"
                    >
                      <option value="owner">Owner</option>
                      <option value="admin">Admin</option>
                      <option value="member">Member</option>
                      <option value="viewer">Viewer</option>
                    </select>
                    {member.user_id !== user?.id && (
                      <button
                        onClick={() => handleRemoveMember(member.user_id)}
                        className="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                </div>
              ))}
              {members.length === 0 && (
                <div className="p-8 text-center text-gray-500">
                  No members yet
                </div>
              )}
            </div>
          </div>
        )}

        {/* Enrollment Tokens Tab */}
        {activeTab === "tokens" && (
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <div>
                <h2 className="text-lg font-semibold">Enrollment Tokens</h2>
                <p className="text-sm text-gray-500">
                  Generate tokens for one-liner agent installation
                </p>
              </div>
              <button
                onClick={() => setShowTokenModal(true)}
                className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
              >
                <Plus className="h-4 w-4" />
                Create Token
              </button>
            </div>

            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 divide-y divide-gray-200 dark:divide-gray-700">
              {tokens.map((token) => (
                <div key={token.id} className="p-4">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <Key className="h-4 w-4 text-gray-400" />
                      <span className="font-medium text-gray-900 dark:text-white">
                        {token.name || "Unnamed Token"}
                      </span>
                      {!token.enabled && (
                        <span className="px-2 py-0.5 text-xs bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 rounded">
                          Revoked
                        </span>
                      )}
                    </div>
                    {token.enabled && (
                      <button
                        onClick={() => handleRevokeToken(token.id)}
                        className="text-sm text-red-500 hover:underline"
                      >
                        Revoke
                      </button>
                    )}
                  </div>
                  <div className="text-sm text-gray-500 space-y-1">
                    <p>
                      Uses: {token.use_count}
                      {token.max_uses ? ` / ${token.max_uses}` : " (unlimited)"}
                    </p>
                    {token.expires_at && (
                      <p>
                        Expires:{" "}
                        {new Date(token.expires_at).toLocaleDateString()}
                      </p>
                    )}
                  </div>
                </div>
              ))}
              {tokens.length === 0 && (
                <div className="p-8 text-center text-gray-500">
                  <Key className="h-8 w-8 mx-auto mb-2 opacity-50" />
                  <p>No enrollment tokens</p>
                  <p className="text-sm">
                    Create a token to enable one-liner agent installation
                  </p>
                </div>
              )}
            </div>

            {/* Install command example */}
            <div className="bg-gray-800 rounded-lg p-4">
              <p className="text-sm text-gray-400 mb-2">
                Example install command:
              </p>
              <code className="text-sm text-green-400">
                curl -fsSL https://get.infrapilot.dev | sh -s -- --token
                YOUR_TOKEN
              </code>
            </div>
          </div>
        )}

        {/* Invite Modal */}
        {showInviteModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md">
              <h3 className="text-lg font-semibold mb-4">Invite Member</h3>
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium mb-1">
                    Email
                  </label>
                  <input
                    type="email"
                    value={inviteEmail}
                    onChange={(e) => setInviteEmail(e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                    placeholder="user@example.com"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">Role</label>
                  <select
                    value={inviteRole}
                    onChange={(e) =>
                      setInviteRole(e.target.value as OrgMemberRole)
                    }
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                  >
                    <option value="admin">Admin</option>
                    <option value="member">Member</option>
                    <option value="viewer">Viewer</option>
                  </select>
                </div>
                <div className="flex gap-3 justify-end">
                  <button
                    onClick={() => setShowInviteModal(false)}
                    className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleInviteMember}
                    className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
                  >
                    Send Invite
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Create Token Modal */}
        {showTokenModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md">
              {newToken ? (
                <>
                  <div className="flex items-center gap-2 text-green-600 mb-4">
                    <Check className="h-5 w-5" />
                    <h3 className="text-lg font-semibold">Token Created</h3>
                  </div>
                  <p className="text-sm text-gray-500 mb-4">
                    Copy this token now. You won&apos;t be able to see it again.
                  </p>
                  <div className="flex items-center gap-2 p-3 bg-gray-100 dark:bg-gray-800 rounded-lg mb-4">
                    <code className="flex-1 text-sm break-all">{newToken}</code>
                    <button
                      onClick={() => copyToClipboard(newToken)}
                      className="p-2 hover:bg-gray-200 dark:hover:bg-gray-700 rounded"
                    >
                      <Copy className="h-4 w-4" />
                    </button>
                  </div>
                  <button
                    onClick={() => {
                      setNewToken(null);
                      setShowTokenModal(false);
                    }}
                    className="w-full px-4 py-2 bg-primary-600 text-white rounded-lg"
                  >
                    Done
                  </button>
                </>
              ) : (
                <>
                  <h3 className="text-lg font-semibold mb-4">
                    Create Enrollment Token
                  </h3>
                  <div className="space-y-4">
                    <div>
                      <label className="block text-sm font-medium mb-1">
                        Name
                      </label>
                      <input
                        type="text"
                        value={tokenName}
                        onChange={(e) => setTokenName(e.target.value)}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                        placeholder="Production servers"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium mb-1">
                        Max Uses (optional)
                      </label>
                      <input
                        type="number"
                        value={tokenMaxUses || ""}
                        onChange={(e) =>
                          setTokenMaxUses(
                            e.target.value ? parseInt(e.target.value) : undefined
                          )
                        }
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                        placeholder="Unlimited"
                        min={1}
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium mb-1">
                        Expires (optional)
                      </label>
                      <input
                        type="datetime-local"
                        value={tokenExpiry}
                        onChange={(e) => setTokenExpiry(e.target.value)}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                      />
                    </div>
                    <div className="flex gap-3 justify-end">
                      <button
                        onClick={() => setShowTokenModal(false)}
                        className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg"
                      >
                        Cancel
                      </button>
                      <button
                        onClick={handleCreateToken}
                        className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
                      >
                        Create Token
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}

        {/* Delete Confirmation Modal */}
        {showDeleteConfirm && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md">
              <div className="flex items-center gap-2 text-red-600 mb-4">
                <AlertTriangle className="h-5 w-5" />
                <h3 className="text-lg font-semibold">Delete Organization</h3>
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
                This action cannot be undone. Type{" "}
                <strong>{org.name}</strong> to confirm.
              </p>
              <input
                type="text"
                value={deleteConfirmText}
                onChange={(e) => setDeleteConfirmText(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 mb-4"
                placeholder={org.name}
              />
              <div className="flex gap-3 justify-end">
                <button
                  onClick={() => {
                    setShowDeleteConfirm(false);
                    setDeleteConfirmText("");
                  }}
                  className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg"
                >
                  Cancel
                </button>
                <button
                  onClick={handleDeleteOrg}
                  disabled={deleteConfirmText !== org.name}
                  className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
