# Proposal: Enterprise Deployment

- **Proposal ID**: 0033
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement enterprise deployment features including centralized configuration, SSO authentication, audit logging, and policy management for organizational deployments of mycode.

## Motivation

Enterprise environments have unique requirements:

1. **Centralized config**: Organization-wide settings
2. **Authentication**: SSO/SAML integration
3. **Audit logging**: Compliance requirements
4. **Policy enforcement**: Usage restrictions
5. **License management**: Seat-based licensing

Enterprise features enable organizational adoption.

## Detailed Design

### API Design

```typescript
// src/enterprise/types.ts
interface EnterpriseConfig {
  orgId: string;
  configUrl?: string;
  sso?: SSOConfig;
  policies: PolicyConfig;
  audit: AuditConfig;
  features: FeatureFlags;
}

interface SSOConfig {
  provider: 'saml' | 'oidc' | 'ldap';
  issuer: string;
  clientId: string;
  redirectUri: string;
  scopes: string[];
}

interface PolicyConfig {
  allowedProviders: string[];
  allowedModels: string[];
  maxTokensPerDay?: number;
  blockedTools?: string[];
  requireApproval?: string[];
  dataRetention: number;  // Days
}

interface AuditConfig {
  enabled: boolean;
  destination: 'local' | 'remote' | 'siem';
  endpoint?: string;
  events: AuditEventType[];
}

type AuditEventType =
  | 'session_start'
  | 'session_end'
  | 'tool_execution'
  | 'message_sent'
  | 'file_access'
  | 'permission_granted';

interface AuditEvent {
  timestamp: Date;
  type: AuditEventType;
  userId: string;
  sessionId: string;
  details: Record<string, unknown>;
  ip?: string;
}
```

### Enterprise Manager

```typescript
// src/enterprise/manager.ts
class EnterpriseManager {
  private config: EnterpriseConfig | null = null;
  private authenticated: boolean = false;

  async initialize(): Promise<void> {
    // Check for enterprise config
    const configPath = process.env.MYCODE_ENTERPRISE_CONFIG;
    if (!configPath) return;

    this.config = await this.loadConfig(configPath);

    if (this.config.sso) {
      await this.authenticateSSO();
    }
  }

  async enforcePolicy(action: string, context: unknown): Promise<boolean> {
    if (!this.config) return true;

    const policies = this.config.policies;

    // Check provider restrictions
    if (action === 'use_provider') {
      const provider = context as string;
      if (!policies.allowedProviders.includes(provider)) {
        throw new Error(`Provider ${provider} not allowed by policy`);
      }
    }

    // Check tool restrictions
    if (action === 'execute_tool') {
      const tool = (context as { tool: string }).tool;
      if (policies.blockedTools?.includes(tool)) {
        throw new Error(`Tool ${tool} blocked by policy`);
      }
    }

    return true;
  }

  async logAudit(event: AuditEvent): Promise<void> {
    if (!this.config?.audit.enabled) return;

    switch (this.config.audit.destination) {
      case 'local':
        await this.logLocal(event);
        break;
      case 'remote':
        await this.logRemote(event);
        break;
      case 'siem':
        await this.logSIEM(event);
        break;
    }
  }

  private async authenticateSSO(): Promise<void> {
    const sso = this.config!.sso!;

    switch (sso.provider) {
      case 'oidc':
        await this.authenticateOIDC(sso);
        break;
      case 'saml':
        await this.authenticateSAML(sso);
        break;
      case 'ldap':
        await this.authenticateLDAP(sso);
        break;
    }
  }
}

export const enterpriseManager = new EnterpriseManager();
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/enterprise/types.ts` | Create | Type definitions |
| `src/enterprise/manager.ts` | Create | Enterprise management |
| `src/enterprise/auth/*.ts` | Create | SSO providers |
| `src/enterprise/audit.ts` | Create | Audit logging |
| `src/enterprise/policy.ts` | Create | Policy enforcement |

## User Experience

### Enterprise Login
```
$ mycode

Enterprise Mode: Acme Corp
Authenticating via SSO...

Opening browser for authentication...
✓ Authenticated as user@company.com

Session policies:
- Allowed providers: anthropic, openai
- Daily token limit: 100,000
- Audit logging: enabled
```

### Policy Violation
```
Agent: [Bash: rm -rf /tmp/data]

⚠️ Policy Violation

This operation is blocked by organization policy.
Reason: Bash tool restricted for security

Contact your administrator for access.
```

## Security Considerations

1. Secure config transmission
2. Token encryption
3. Audit log integrity
4. Policy tampering prevention
5. Session isolation

## Migration Path

1. **Phase 1**: Centralized config
2. **Phase 2**: SSO authentication
3. **Phase 3**: Policy enforcement
4. **Phase 4**: Audit logging
5. **Phase 5**: SIEM integration

## References

- [OIDC Specification](https://openid.net/connect/)
- [SAML 2.0](https://docs.oasis-open.org/security/saml/v2.0/)
