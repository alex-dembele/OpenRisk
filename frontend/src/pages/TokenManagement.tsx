// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useFormat } from '../hooks/useI18n';
import { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { Key, Trash2, Lock, Copy, Plus, Search, Calendar, ChevronDown } from 'lucide-react';
import { api } from '../lib/api';
import { toast } from 'sonner';
import { Button } from '../shared/ds';

interface APIToken {
  id: string;
  name: string;
  description?: string;
  token_prefix?: string;
  status: string;
  permissions?: string[];
  scopes?: string[];
  expires_at?: string;
  created_at: string;
  last_used_at?: string;
}

interface CreateTokenRequest {
  name: string;
  description: string;
  permissions?: string[];
  scopes?: string[];
  expires_at?: string;
}

export const TokenManagement = () => {
  const fmt = useFormat();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreating, setIsCreating] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [newTokenValue, setNewTokenValue] = useState<string>('');
  const [formData, setFormData] = useState<CreateTokenRequest>({
    name: '',
    description: '',
  });

  useEffect(() => {
    fetchTokens();
  }, []);

  const fetchTokens = async () => {
    setIsLoading(true);
    try {
      const response = await api.get('/tokens');
      setTokens(response.data || []);
    } catch (err: any) {
      toast.error('Unable to load your API tokens. Please refresh the page.');
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreateToken = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData.name.trim()) {
      toast.error('Token name is required');
      return;
    }

    setIsCreating(true);
    try {
      const response = await api.post('/tokens', {
        name: formData.name,
        description: formData.description,
        permissions: formData.permissions || [],
        scopes: formData.scopes || [],
        expires_at: formData.expires_at,
      });

      setNewTokenValue(response.data.token_value);
      setFormData({ name: '', description: '' });
      await fetchTokens();
      toast.success(
        "Token created successfully. Copy the token value now - you won't see it again!",
      );
    } catch (err: any) {
      toast.error(
        err.response?.data?.message ||
          "Couldn't create the API token. Please check your input and try again.",
      );
    } finally {
      setIsCreating(false);
    }
  };

  const handleRevokeToken = async (tokenId: string) => {
    if (!confirm('Are you sure you want to revoke this token? It will no longer be usable.')) {
      return;
    }

    try {
      await api.post(`/tokens/${tokenId}/revoke`);
      setTokens(tokens.map((t) => (t.id === tokenId ? { ...t, status: 'revoked' } : t)));
      toast.success('Token revoked');
    } catch (err) {
      toast.error("We couldn't disable this token. Please try again.");
    }
  };

  const handleDeleteToken = async (tokenId: string) => {
    if (!confirm('Are you sure you want to permanently delete this token?')) {
      return;
    }

    try {
      await api.delete(`/tokens/${tokenId}`);
      setTokens(tokens.filter((t) => t.id !== tokenId));
      toast.success('Token deleted');
    } catch (err) {
      toast.error("We couldn't remove this token. Please try again.");
    }
  };

  const handleRotateToken = async (tokenId: string) => {
    if (!confirm('This will create a new token and keep the old one. Continue?')) {
      return;
    }

    try {
      const response = await api.post(`/tokens/${tokenId}/rotate`);
      setNewTokenValue(response.data.new_token_value);
      await fetchTokens();
      toast.success('Token rotated successfully. Copy the new token value now!');
    } catch (err) {
      toast.error("We couldn't refresh the token. Please try again.");
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast.success('Copied to clipboard');
  };

  const filteredTokens = tokens.filter((token) =>
    token.name.toLowerCase().includes(searchTerm.toLowerCase()),
  );

  const activeTokens = filteredTokens.filter((t) => t.status === 'active').length;

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never';
    return fmt.date(dateString, { month: 'short', day: 'numeric', year: 'numeric' });
  };

  const isExpiringSoon = (expiresAt?: string) => {
    if (!expiresAt) return false;
    const expiryDate = new Date(expiresAt);
    const daysUntilExpiry = Math.floor((expiryDate.getTime() - Date.now()) / (1000 * 60 * 60 * 24));
    return daysUntilExpiry <= 7 && daysUntilExpiry > 0;
  };

  const isExpired = (expiresAt?: string) => {
    if (!expiresAt) return false;
    return new Date(expiresAt) < new Date();
  };

  return (
    <div className="min-h-screen bg-surface-0">
      {/* Header */}
      <div className="border-b border-border-subtle bg-linear-to-b from-zinc-900 to-zinc-950 p-6">
        <div className="max-w-6xl mx-auto">
          <div className="flex items-center gap-3 mb-2">
            <Key className="w-8 h-8 text-warning-text" />
            <h1 className="text-3xl font-bold text-fg-primary">API Tokens</h1>
          </div>
          <p className="text-fg-secondary">Manage your API tokens for programmatic access</p>
        </div>
      </div>

      {/* Main Content */}
      <div className="max-w-6xl mx-auto p-6">
        {/* Stats Bar */}
        <div className="grid grid-cols-3 gap-4 mb-6">
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-surface-1 border border-border-subtle rounded-lg p-4"
          >
            <div className="text-fg-secondary text-sm font-medium">Total Tokens</div>
            <div className="text-2xl font-bold text-fg-primary mt-1">{tokens.length}</div>
          </motion.div>
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.1 }}
            className="bg-surface-1 border border-border-subtle rounded-lg p-4"
          >
            <div className="text-fg-secondary text-sm font-medium">Active</div>
            <div className="text-2xl font-bold text-success-text mt-1">{activeTokens}</div>
          </motion.div>
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2 }}
            className="bg-surface-1 border border-border-subtle rounded-lg p-4"
          >
            <div className="text-fg-secondary text-sm font-medium">Revoked</div>
            <div className="text-2xl font-bold text-danger-text mt-1">
              {tokens.filter((t) => t.status === 'revoked').length}
            </div>
          </motion.div>
        </div>

        {/* New Token Display */}
        {newTokenValue && (
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className="mb-6 bg-success-surface border border-success/50 rounded-lg p-4"
          >
            <div className="flex items-start justify-between">
              <div className="flex-1">
                <h3 className="font-semibold text-success-text mb-2">New Token Created</h3>
                <p className="text-sm text-fg-secondary mb-3">
                  Save this token now. You won't be able to see it again!
                </p>
                <div className="bg-surface-1 rounded px-3 py-2 flex items-center justify-between">
                  <code className="text-sm text-fg-primary break-all">{newTokenValue}</code>
                  <button
                    onClick={() => copyToClipboard(newTokenValue)}
                    className="ml-3 p-1 hover:bg-surface-2 rounded"
                  >
                    <Copy className="w-4 h-4 text-success-text" />
                  </button>
                </div>
              </div>
              <button
                onClick={() => setNewTokenValue('')}
                className="text-fg-secondary hover:text-fg-primary ml-4"
              >
                ✕
              </button>
            </div>
          </motion.div>
        )}

        {/* Controls */}
        <div className="flex gap-4 mb-6">
          <Button
            variant="primary"
            onClick={() => setShowCreateForm(!showCreateForm)}
            className="gap-2"
          >
            <Plus className="w-4 h-4" />
            New Token
          </Button>
        </div>

        {/* Create Form */}
        {showCreateForm && (
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            className="mb-6 bg-surface-1 border border-border-subtle rounded-lg p-4"
          >
            <form onSubmit={handleCreateToken} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-fg-secondary mb-1">
                  Token Name *
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="e.g., CI/CD Pipeline, DataSync"
                  className="w-full bg-surface-2 border border-border-default rounded px-3 py-2 text-fg-primary text-sm focus:outline-none focus:border-accent"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-fg-secondary mb-1">
                  Description
                </label>
                <textarea
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  placeholder="What is this token for?"
                  className="w-full bg-surface-2 border border-border-default rounded px-3 py-2 text-fg-primary text-sm focus:outline-none focus:border-accent resize-none"
                  rows={3}
                />
              </div>
              <div className="flex gap-3">
                <Button variant="primary" type="submit" disabled={isCreating} className="gap-2">
                  {isCreating ? 'Creating...' : 'Create Token'}
                </Button>
                <Button type="button" onClick={() => setShowCreateForm(false)} variant="secondary">
                  Cancel
                </Button>
              </div>
            </form>
          </motion.div>
        )}

        {/* Search */}
        <div className="mb-6 flex items-center gap-2">
          <Search className="w-4 h-4 text-fg-secondary" />
          <input
            type="text"
            placeholder="Search tokens..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="flex-1 bg-surface-1 border border-border-subtle rounded px-3 py-2 text-fg-primary text-sm focus:outline-none focus:border-accent"
          />
        </div>

        {/* Tokens List */}
        {isLoading ? (
          <div className="text-center py-12">
            <p className="text-fg-secondary">Loading tokens...</p>
          </div>
        ) : filteredTokens.length === 0 ? (
          <div className="text-center py-12">
            <Key className="w-12 h-12 text-fg-primary mx-auto mb-4" />
            <p className="text-fg-secondary">No tokens found</p>
          </div>
        ) : (
          <div className="space-y-3">
            {filteredTokens.map((token, index) => (
              <motion.div
                key={token.id}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.05 }}
                className="bg-surface-1 border border-border-subtle rounded-lg p-4 hover:border-border-default transition"
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-1">
                      <h3 className="font-semibold text-fg-primary">{token.name}</h3>
                      <span
                        className={`text-xs px-2 py-1 rounded font-medium ${
                          token.status === 'active'
                            ? 'bg-success/10 text-success-text border border-success/20'
                            : 'bg-danger/10 text-danger-text border border-danger/20'
                        }`}
                      >
                        {token.status.charAt(0).toUpperCase() + token.status.slice(1)}
                      </span>
                      {token.token_prefix && (
                        <span className="text-xs text-fg-muted font-mono">
                          {token.token_prefix}...
                        </span>
                      )}
                    </div>
                    {token.description && (
                      <p className="text-sm text-fg-secondary mb-3">{token.description}</p>
                    )}
                    <div className="flex gap-4 text-xs text-fg-muted">
                      <div className="flex items-center gap-1">
                        <Calendar className="w-3 h-3" />
                        Created: {formatDate(token.created_at)}
                      </div>
                      {token.last_used_at && (
                        <div className="flex items-center gap-1">
                          <Calendar className="w-3 h-3" />
                          Last used: {formatDate(token.last_used_at)}
                        </div>
                      )}
                      {token.expires_at && (
                        <div
                          className={`flex items-center gap-1 ${
                            isExpired(token.expires_at)
                              ? 'text-danger-text'
                              : isExpiringSoon(token.expires_at)
                                ? 'text-warning-text'
                                : ''
                          }`}
                        >
                          <Lock className="w-3 h-3" />
                          Expires: {formatDate(token.expires_at)}
                          {isExpiringSoon(token.expires_at) && ' (Soon)'}
                          {isExpired(token.expires_at) && ' (Expired)'}
                        </div>
                      )}
                    </div>
                    {token.permissions && token.permissions.length > 0 && (
                      <div className="mt-3">
                        <p className="text-xs text-fg-secondary mb-1">Permissions:</p>
                        <div className="flex flex-wrap gap-1">
                          {token.permissions.map((perm) => (
                            <span
                              key={perm}
                              className="text-xs bg-accent-soft text-info-text border border-accent-line rounded px-2 py-1"
                            >
                              {perm}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2 ml-4">
                    {token.status === 'active' && (
                      <>
                        <button
                          onClick={() => handleRotateToken(token.id)}
                          className="p-2 text-info-text hover:bg-accent-soft rounded transition"
                          title="Rotate token"
                        >
                          <ChevronDown className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handleRevokeToken(token.id)}
                          className="p-2 text-warning-text hover:bg-warning/10 rounded transition"
                          title="Revoke token"
                        >
                          <Lock className="w-4 h-4" />
                        </button>
                      </>
                    )}
                    <button
                      onClick={() => handleDeleteToken(token.id)}
                      className="p-2 text-danger-text hover:bg-danger/10 rounded transition"
                      title="Delete token"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </motion.div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
