# Checkly Monitoring - Security Best Practices

## 🔐 Managing Secrets and API Keys

### ⚠️ NEVER commit API keys to Git

API keys, tokens, and other secrets should **NEVER** be hardcoded in check files or committed to version control.

### ✅ Use Checkly Environment Variables

Checkly supports environment variables that can be securely stored and referenced in your checks.

#### How to set up environment variables:

1. **Via Checkly Dashboard:**
   - Go to https://app.checklyhq.com/
   - Navigate to your project
   - Go to **Settings** → **Environment Variables**
   - Add your variables (e.g., `ORACLE_MONITOR_API_KEY`)

2. **Reference in your checks:**

   ```typescript
   headers: [
     {
       key: 'X-API-Key',
       value: '{{ORACLE_MONITOR_API_KEY}}', // References the env var
     },
   ];
   ```

3. **Deploy your checks:**
   ```bash
   npx checkly deploy
   ```

### 📋 Current Environment Variables Needed

| Variable Name            | Description                                      | Used In                   |
| ------------------------ | ------------------------------------------------ | ------------------------- |
| `ORACLE_MONITOR_API_KEY` | API key for Oracle Free Tier monitoring endpoint | `oracle-monitor.check.ts` |

### 🔄 Rotating API Keys

If an API key is compromised:

1. **Generate a new key** in the service that uses it
2. **Update the environment variable** in Checkly dashboard
3. **Redeploy checks** (if needed): `npx checkly deploy`
4. **Revoke the old key** in the service

### 📝 Files to Ignore

The following files should be in `.gitignore`:

```gitignore
# Checkly exports (may contain sensitive data)
checkly_*.json
```

### 🚨 What to do if you accidentally commit a secret

1. **Immediately revoke/rotate** the exposed secret
2. **Remove from Git history:**
   ```bash
   git filter-repo --invert-paths --path <file-with-secret> --force
   ```
3. **Force push** to remote:
   ```bash
   git push --force --all
   ```
4. **Update** the secret in Checkly environment variables

## 📚 Resources

- [Checkly Environment Variables Documentation](https://www.checklyhq.com/docs/cli/using-environment-variables/)
- [Checkly CLI Documentation](https://www.checklyhq.com/docs/cli/)
- [GitGuardian - Secret Detection](https://www.gitguardian.com/)
