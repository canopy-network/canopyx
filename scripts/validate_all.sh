#!/bin/bash
# Runs all validation checks

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "======================================"
echo "CanopyX Code Review Validation Suite"
echo "======================================"
echo ""

FAILED=0

# Script 1: RPC Types
echo "📋 Step 1/4: Validating RPC Types..."
if bash "$SCRIPT_DIR/validate_rpc_types.sh"; then
    echo ""
else
    FAILED=$((FAILED + 1))
    echo ""
fi

# Script 2: RPC Pattern
echo "📋 Step 2/4: Validating RPC(H) vs RPC(H-1) Pattern..."
if bash "$SCRIPT_DIR/validate_rpc_pattern.sh"; then
    echo ""
else
    FAILED=$((FAILED + 1))
    echo ""
fi

# Script 3: Database Schemas
echo "📋 Step 3/4: Validating Database Schemas..."
if bash "$SCRIPT_DIR/validate_db_schema.sh"; then
    echo ""
else
    FAILED=$((FAILED + 1))
    echo ""
fi

# Script 4: Snapshot Pattern
echo "📋 Step 4/4: Validating Snapshot-on-Change Pattern..."
if bash "$SCRIPT_DIR/validate_snapshot_pattern.sh"; then
    echo ""
else
    FAILED=$((FAILED + 1))
    echo ""
fi

# Final Report
echo "======================================"
if [ $FAILED -eq 0 ]; then
    echo "✅ ALL VALIDATIONS PASSED!"
    echo "======================================"
    echo ""
    echo "Your CanopyX codebase is compliant with all architectural"
    echo "patterns identified in the code review."
    echo ""
    echo "Validator fixes verified:"
    echo "  ✅ All 10 fields from Canopy captured"
    echo "  ✅ No phantom fields"
    echo "  ✅ Correct JSON unmarshaling"
    echo "  ✅ Historical state queries working"
    echo "  ✅ Status derivation implemented"
    echo "  ✅ Database schemas aligned"
    exit 0
else
    echo "❌ $FAILED VALIDATION(S) FAILED"
    echo "======================================"
    echo ""
    echo "Please fix the issues identified above before deploying."
    echo "See docs/review/IMPLEMENTATION_SUMMARY.md for detailed remediation steps."
    exit 1
fi
