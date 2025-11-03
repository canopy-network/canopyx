#!/bin/bash
# Validates RPC(H) vs RPC(H-1) pattern compliance in activities

set -e

ACTIVITIES_PATH="app/indexer/activity"

echo "🔍 Checking RPC(H) vs RPC(H-1) Pattern Compliance..."
echo ""

VIOLATIONS=0

# Check for illegal database queries for previous state
echo "Scanning for illegal database queries in activities..."

# Search for patterns like: chainDb.Get/Query/Select/Has with input.Height-1
if grep -rnE "chainDb\.(Get|Query|Select|Has).*input\.Height\s*-\s*1" "$ACTIVITIES_PATH" 2>/dev/null; then
    echo "❌ VIOLATION: Found database query for previous state (H-1)"
    echo "Activities MUST fetch previous state from RPC, not database"
    VIOLATIONS=$((VIOLATIONS + 1))
else
    echo "✅ No illegal database queries for previous state found"
fi

echo ""

# Check that height-aware activities fetch H-1 from RPC
echo "Checking RPC client calls for height parameters..."

# Activities that SHOULD use height parameters and fetch H-1
declare -A CHECKS=(
    ["accounts.go"]="cli\.AccountsByHeight.*input\.Height\s*-\s*1"
    ["orders.go"]="cli\.OrdersByHeight.*input\.Height\s*-\s*1"
    ["params.go"]="cli\.AllParams.*input\.Height\s*-\s*1|in\.Height\s*-\s*1"
    ["committees.go"]="cli\.CommitteesData.*in\.Height\s*-\s*1"
    ["validators.go"]="cli\.Validators.*input\.Height\s*-\s*1"
)

for file in "${!CHECKS[@]}"; do
    pattern="${CHECKS[$file]}"

    if [ -f "$ACTIVITIES_PATH/$file" ]; then
        if grep -qE "$pattern" "$ACTIVITIES_PATH/$file" 2>/dev/null; then
            echo "✅ $file correctly fetches H-1 from RPC"
        else
            echo "❌ VIOLATION: $file doesn't fetch previous state with height H-1"
            VIOLATIONS=$((VIOLATIONS + 1))
        fi
    else
        echo "⚠️  WARNING: $file not found"
    fi
done

echo ""

# Additional check: Verify validators.go also fetches NonSigners with height
echo "Checking NonSigners RPC calls in validators.go..."
if [ -f "$ACTIVITIES_PATH/validators.go" ]; then
    if grep -qE "cli\.NonSigners.*input\.Height\s*-\s*1" "$ACTIVITIES_PATH/validators.go"; then
        echo "✅ validators.go correctly fetches previous NonSigners with H-1"
    else
        echo "❌ VIOLATION: validators.go doesn't fetch previous NonSigners with height H-1"
        VIOLATIONS=$((VIOLATIONS + 1))
    fi

    if grep -qE "cli\.NonSigners.*input\.Height\)" "$ACTIVITIES_PATH/validators.go"; then
        echo "✅ validators.go correctly fetches current NonSigners with H"
    else
        echo "⚠️  WARNING: validators.go may not fetch current NonSigners with explicit height"
    fi
else
    echo "⚠️  WARNING: validators.go not found"
fi

echo ""

# Verify RPC client methods accept height parameter
echo "Checking RPC client implementation..."
RPC_VALIDATORS_FILE="pkg/rpc/validators.go"
if [ -f "$RPC_VALIDATORS_FILE" ]; then
    if grep -q "func.*Validators.*height uint64" "$RPC_VALIDATORS_FILE"; then
        echo "✅ Validators() method accepts height parameter"
    else
        echo "❌ VIOLATION: Validators() method doesn't accept height parameter"
        VIOLATIONS=$((VIOLATIONS + 1))
    fi

    if grep -q "func.*NonSigners.*height uint64" "$RPC_VALIDATORS_FILE"; then
        echo "✅ NonSigners() method accepts height parameter"
    else
        echo "❌ VIOLATION: NonSigners() method doesn't accept height parameter"
        VIOLATIONS=$((VIOLATIONS + 1))
    fi
else
    echo "⚠️  WARNING: RPC validators.go not found"
fi

echo ""

# Report results
if [ $VIOLATIONS -eq 0 ]; then
    echo "✅ All RPC pattern compliance checks passed!"
    exit 0
else
    echo "❌ Found $VIOLATIONS pattern violations"
    echo ""
    echo "CRITICAL: Activities MUST fetch previous state from RPC at height H-1,"
    echo "not from the database. This ensures stateless execution and enables"
    echo "gap filling and re-indexing."
    echo ""
    echo "See docs/review/IMPLEMENTATION_SUMMARY.md for details."
    exit 1
fi
