# Corrective and Preventive Action (CAPA)

## Incident: API Key Exposure in GitHub Release Notes

**Date:** January 29, 2026
**Severity:** Critical
**Status:** Resolved

---

## Executive Summary

An OpenAI API key was inadvertently exposed in public GitHub release notes (v1.0.0) due to improper escaping of GitHub Actions expression syntax in the release automation workflow. The key was immediately revoked, the release deleted, and preventive measures implemented.

---

## What Happened

### Incident Timeline

1. **18:08 UTC** - Release automation workflow triggered for tag v1.0.0
2. **18:08 UTC** - Workflow generated release notes using template in `.github/workflows/release.yml`
3. **18:08 UTC** - GitHub Actions interpolated `${{ secrets.OPENAI_API_KEY }}` with actual key value
4. **18:09 UTC** - Release published publicly with exposed API key in usage example
5. **18:20 UTC** - Exposure discovered by repository owner
6. **18:21 UTC** - Release and tag deleted immediately
7. **18:25 UTC** - API key revoked at OpenAI
8. **18:27 UTC** - Root cause identified and fix implemented
9. **18:30 UTC** - New release created with corrected template

**Total Exposure Duration:** ~12 minutes

---

## Root Cause Analysis

### Primary Cause

Improper escaping of GitHub Actions expression syntax in release notes template.

**Vulnerable Code** (`.github/workflows/release.yml` line 57):
```yaml
api_key: \${{ secrets.OPENAI_API_KEY }}
```

**Issue:** The backslash (`\`) only escapes the dollar sign for bash, but GitHub Actions processes `${{ }}` expressions **before** bash sees the heredoc. This resulted in the actual secret value being interpolated into the release notes.

### Contributing Factors

1. **Misunderstanding of GitHub Actions execution order:**
   - GitHub Actions preprocesses YAML and evaluates `${{ }}` expressions first
   - Shell escaping (`\$`) does not prevent GitHub Actions interpolation
   - The heredoc (`<< 'EOF'`) prevents shell variable expansion but not GitHub Actions expression evaluation

2. **Lack of testing:**
   - Release workflow was not tested with actual repository secrets before production use
   - No validation that example code would display placeholders vs actual values

3. **Insufficient documentation:**
   - GitHub Actions expression escaping requirements not clearly documented in workflow

---

## Impact Assessment

### Security Impact
- ✅ **Key Revoked:** API key revoked within 5 minutes of discovery
- ✅ **Limited Exposure:** Release was public for ~12 minutes
- ⚠️ **Potential Usage:** Unknown if key was accessed/used by third parties during exposure window
- ✅ **No Billing Impact:** No unusual API usage detected (monitored post-incident)

### Operational Impact
- ⚠️ Minor delay in v1.0.0 release availability (~30 minutes)
- ✅ No impact on existing users (first release)
- ✅ No data breach or unauthorized access to systems

---

## Resolution

### Immediate Actions Taken

1. **Deleted exposed release:**
   ```bash
   gh release delete v1.0.0 --yes --cleanup-tag
   ```

2. **Revoked compromised API key:**
   - Deleted key at https://platform.openai.com/api-keys
   - Generated new key
   - Updated repository secret

3. **Fixed release workflow template:**
   ```yaml
   # Before (vulnerable):
   api_key: \${{ secrets.OPENAI_API_KEY }}

   # After (secure):
   api_key: \$\{{ secrets.OPENAI_API_KEY }}
   ```

   **Explanation:** Escaping both `$` and `{{` prevents GitHub Actions from recognizing it as an expression.

4. **Verified fix:**
   - Created new v1.0.0 release
   - Confirmed placeholder text appears instead of actual key value

---

## Preventive Measures

### Short-term (Completed)

1. ✅ **Corrected escaping syntax** in all workflow templates
2. ✅ **Documented proper escaping** in workflow comments
3. ✅ **Verified release notes** display placeholder text correctly
4. ✅ **Created this COE document** for future reference

### Long-term (Recommended)

1. **Pre-release validation:**
   - Add workflow that validates release notes don't contain actual secret values
   - Use pattern matching to detect exposed keys before publish
   - Implement dry-run mode for release workflows

2. **Secret scanning:**
   - Enable GitHub secret scanning (free for public repos)
   - Configure alerts for exposed secrets
   - Use pre-commit hooks to prevent committing secrets

3. **Documentation improvements:**
   - Document GitHub Actions expression escaping in workflow files
   - Create security checklist for release automation
   - Add examples of proper vs improper escaping

4. **Monitoring:**
   - Set up API usage alerts for abnormal patterns
   - Monitor for unauthorized access attempts
   - Implement key rotation schedule (quarterly)

5. **Testing procedures:**
   - Test release workflows in fork/test environment first
   - Validate with dummy secrets before using production secrets
   - Peer review workflow changes before merging

---

## Lessons Learned

### What Went Well
- ✅ Rapid detection (within minutes of exposure)
- ✅ Immediate response and mitigation
- ✅ Clear understanding of root cause
- ✅ Quick implementation of fix
- ✅ No actual security impact detected

### What Could Be Improved
- ⚠️ Should have tested release workflow before production use
- ⚠️ Better understanding of GitHub Actions expression processing needed
- ⚠️ Automated validation of release content needed
- ⚠️ Documentation of escaping requirements missing

### Key Takeaways

1. **GitHub Actions processes expressions before shell scripts run**
   - `${{ }}` syntax is evaluated during YAML preprocessing
   - Shell escaping (`\$`) is insufficient for GitHub Actions expressions
   - Must escape both `$` and braces: `\$\{{`

2. **Heredoc quotes don't prevent GitHub Actions interpolation**
   - `<< 'EOF'` prevents shell variable expansion
   - Does NOT prevent GitHub Actions expression evaluation
   - Expression syntax is processed before shell receives the content

3. **Test automation workflows with production-like data**
   - Use test secrets to validate behavior
   - Verify output matches expectations
   - Automated testing is critical for security

4. **Defense in depth is essential**
   - Don't rely on single security measure
   - Implement multiple layers (escaping + scanning + monitoring)
   - Fast detection and response limits impact

---

## Technical Details: GitHub Actions Expression Processing

### Processing Order

```
1. GitHub Actions reads workflow YAML
2. Evaluates ${{ }} expressions (including secrets)
3. Substitutes expression results into YAML
4. Passes processed content to shell
5. Shell processes heredocs and scripts
```

### Escaping Methods

| Method | Prevents Shell Expansion | Prevents GitHub Actions Interpolation |
|--------|-------------------------|--------------------------------------|
| `\$` | ✅ Yes | ❌ No |
| `<< 'EOF'` | ✅ Yes | ❌ No |
| `\$\{{` | ✅ Yes | ✅ Yes |
| `$${{` | ⚠️ Partial | ✅ Yes |

**Recommended:** Use `\$\{{` for displaying GitHub Actions expression syntax as literal text.

---

## Verification

### Before Fix
```yaml
api_key: \${{ secrets.OPENAI_API_KEY }}
```
**Result in release notes:** `api_key: sk-proj-ZJsc...` (actual key value) ❌

### After Fix
```yaml
api_key: \$\{{ secrets.OPENAI_API_KEY }}
```
**Result in release notes:** `api_key: ${{ secrets.OPENAI_API_KEY }}` (placeholder) ✅

---

## References

- [GitHub Actions: Expressions](https://docs.github.com/en/actions/learn-github-actions/expressions)
- [GitHub Actions: Encrypted Secrets](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [Bash Heredoc Documentation](https://www.gnu.org/software/bash/manual/html_node/Redirections.html)

---

## Sign-off

**Created by:** Development Team
**Reviewed by:** Security Team
**Approved by:** Repository Owner
**Date:** January 29, 2026

**Status:** Incident resolved, preventive measures implemented, monitoring ongoing.
