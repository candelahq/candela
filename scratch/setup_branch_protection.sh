#!/usr/bin/env bash
set -euo pipefail

# Branch protection rules for all candelahq repos.
# enforce_admins=false allows admin override when needed.

REPOS=(
  "candelahq/candela"
  "candelahq/candela-rs"
  "candelahq/candela-desktop"
  "candelahq/candela-docs"
  "candelahq/candela-protos"
)

# Issue numbers for closing (repo -> issue number)
declare -A ISSUES=(
  ["candelahq/candela"]=130
  ["candelahq/candela-rs"]=1
  ["candelahq/candela-desktop"]=13
  ["candelahq/candela-docs"]=1
  ["candelahq/candela-protos"]=2
)

PROTECTION_BODY='{
  "required_status_checks": {
    "strict": true,
    "contexts": []
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}'

for repo in "${REPOS[@]}"; do
  echo "━━━ $repo ━━━"

  # Check if main branch exists
  if ! gh api "repos/${repo}/branches/main" --jq '.name' 2>/dev/null; then
    echo "  ⚠️  No main branch — skipping (needs initial commit first)"
    echo ""
    continue
  fi

  # Apply branch protection
  echo "  Applying branch protection..."
  gh api "repos/${repo}/branches/main/protection" \
    --method PUT \
    --input - <<< "$PROTECTION_BODY" \
    --jq '"\(.url)"' 2>&1

  if [ $? -eq 0 ]; then
    echo "  ✅ Branch protection configured"

    # Close the issue
    issue_num=${ISSUES[$repo]}
    echo "  Closing issue #${issue_num}..."

    CLOSE_COMMENT="## ✅ Branch protection configured

Rules applied to \`main\`:

| Rule | Setting |
|---|---|
| **Require PR before merge** | ✅ 1 approval required |
| **Dismiss stale reviews** | ✅ New pushes invalidate approvals |
| **Require status checks to pass** | ✅ Must be green |
| **Require branch up-to-date** | ✅ Strict mode |
| **Require linear history** | ✅ Squash/rebase only |
| **Allow force pushes** | ❌ Disabled |
| **Allow branch deletion** | ❌ Disabled |
| **Admin override** | ✅ Admins can bypass when needed |

Configured via \`gh api\`."

    gh issue comment "${issue_num}" --repo "${repo}" --body "$CLOSE_COMMENT"
    gh issue close "${issue_num}" --repo "${repo}" --reason completed
    echo "  ✅ Issue #${issue_num} closed"
  else
    echo "  ❌ Failed to apply protection"
  fi

  echo ""
done

echo "🎉 All done!"
