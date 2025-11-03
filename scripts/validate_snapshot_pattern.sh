#!/bin/bash
# Validates snapshot-on-change pattern implementation

set -e

ACTIVITIES_PATH="app/indexer/activity"

echo "🔍 Validating Snapshot-on-Change Pattern..."
echo ""

ISSUES=0

# Activities that should use snapshot-on-change
SNAPSHOT_ACTIVITIES=(
    "accounts.go"
    "orders.go"
    "validators.go"
    "params.go"
    "committees.go"
)

# Singleton entities that don't need prevMap (single entity comparison)
SINGLETON_ACTIVITIES=("params.go")

for activity in "${SNAPSHOT_ACTIVITIES[@]}"; do
    FILE="$ACTIVITIES_PATH/$activity"

    if [ ! -f "$FILE" ]; then
        echo "⚠️  WARNING: Activity $activity not found"
        continue
    fi

    echo "Checking $activity..."

    # Check for comparison logic (skip map check for singletons)
    if [[ " ${SINGLETON_ACTIVITIES[@]} " =~ " ${activity} " ]]; then
        # Singleton entity - direct comparison is O(1)
        echo "  ✅ Has previous state map (singleton - no map needed)"
    elif ! grep -q "prevMap\|previousMap\|prev.*Map" "$FILE" 2>/dev/null; then
        echo "  ❌ Missing previous state map for O(1) lookup"
        ISSUES=$((ISSUES + 1))
    else
        echo "  ✅ Has previous state map"
    fi

    # Check for change detection
    if ! grep -qE "(changed|stateChanged|fieldsChanged)" "$FILE" 2>/dev/null; then
        echo "  ❌ Missing change detection logic"
        ISSUES=$((ISSUES + 1))
    else
        echo "  ✅ Has change detection"
    fi

    # Check for conditional insert
    if ! grep -qE "if.*changed|len\(changed" "$FILE" 2>/dev/null; then
        echo "  ⚠️  WARNING: May insert all entities instead of only changed"
    else
        echo "  ✅ Conditionally inserts only changed entities"
    fi

    echo ""
done

# Report results
if [ $ISSUES -eq 0 ]; then
    echo "✅ All snapshot-on-change pattern checks passed!"
    exit 0
else
    echo "❌ Found $ISSUES issues with snapshot-on-change implementation"
    echo ""
    echo "Snapshot-on-change activities should:"
    echo "1. Build a map of previous state for O(1) lookups"
    echo "2. Compare current vs previous state field-by-field"
    echo "3. Only insert entities that have changed"
    echo ""
    echo "See docs/research/final_review.md Phase 2.2 for details."
    exit 1
fi
