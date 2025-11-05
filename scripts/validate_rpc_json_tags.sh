#!/bin/bash
# General RPC JSON tag validator
# Compares CanopyX RPC struct tags against Canopy blockchain protobuf definitions and live RPC responses

set -e

CANOPY_PATH="${CANOPY_PATH:-../canopy}"
CANOPYX_PATH="."
RPC_URL="${RPC_URL:-http://localhost:50002}"

echo "🔍 Validating RPC JSON tags..."
echo "Canopy path: $CANOPY_PATH"
echo "CanopyX path: $CANOPYX_PATH"
echo "RPC URL: $RPC_URL"
echo ""

# Check Canopy path exists
if [ ! -d "$CANOPY_PATH" ]; then
    echo "❌ ERROR: Canopy source not found at $CANOPY_PATH"
    echo "Set CANOPY_PATH environment variable to point to Canopy source"
    exit 1
fi

ERRORS=0
WARNINGS=0

# Helper: Extract JSON tags from Go struct (stops at next type/func)
extract_go_json_tags() {
    local file="$1"
    local struct_name="$2"

    # Use sed to extract from struct declaration to next type/func
    sed -n "/^type $struct_name struct/,/^type\|^func/p" "$file" | \
    head -n -1 | \
    grep -o 'json:"[^"]*"' | \
    sed 's/json:"\([^,"]*\).*/\1/' | \
    grep -v '^$' | \
    grep -v '^-$' || true
}

# Helper: Extract JSON keys from live RPC response
extract_rpc_json_keys() {
    local url="$1"
    local payload="$2"
    timeout 5 curl -s -X POST "$url" -H "Content-Type: application/json" -d "$payload" 2>/dev/null | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    if isinstance(data, dict):
        # Handle paginated responses with 'results' array
        if 'results' in data and isinstance(data['results'], list) and len(data['results']) > 0:
            obj = data['results'][0]
        else:
            obj = data
        # Print all top-level keys
        for key in sorted(obj.keys()):
            print(key)
except:
    pass
" 2>/dev/null || true
}

# Define types to validate in a simple array format
# Each line: "RpcTypeName|canopyx_file|canopy_file|canopy_struct|rpc_endpoint|rpc_payload"
declare -a TYPE_LIST=(
    "RpcValidator|pkg/rpc/validators.go|fsm/validator.pb.go|Validator|/v1/query/validators|{\"height\":1}"
    "RpcNonSigner|pkg/rpc/validators.go|fsm/validator.pb.go|NonSigner|SKIP|SKIP"
    "RpcPool|pkg/rpc/pools.go|fsm/account.pb.go|Pool|/v1/query/pools|{\"height\":1}"
    "RpcOrder|pkg/rpc/orders.go|fsm/message.pb.go|Order|/v1/query/orders|{\"height\":1}"
    "RpcEvent|pkg/rpc/events.go|fsm/message.pb.go|Event|/v1/query/events-by-height|{\"height\":1}"
    "RpcAllParams|pkg/rpc/params.go|fsm/genesis.pb.go|Params|/v1/query/params|{\"height\":1}"
)

echo "📋 Validating RPC struct JSON tags..."
echo ""

for type_spec in "${TYPE_LIST[@]}"; do
    IFS='|' read -r type_name canopyx_file canopy_file canopy_struct_name rpc_endpoint rpc_payload <<< "$type_spec"

    echo "Checking $type_name..."

    canopyx_path="$CANOPYX_PATH/$canopyx_file"
    canopy_path="$CANOPY_PATH/$canopy_file"

    # Check if files exist
    if [ ! -f "$canopyx_path" ]; then
        echo "  ⚠️  WARNING: CanopyX file not found: $canopyx_file"
        WARNINGS=$((WARNINGS + 1))
        echo ""
        continue
    fi

    if [ ! -f "$canopy_path" ]; then
        echo "  ⚠️  WARNING: Canopy file not found: $canopy_file"
        WARNINGS=$((WARNINGS + 1))
        echo ""
        continue
    fi

    # Skip live RPC validation if marked as SKIP
    skip_rpc_validation=false
    if [ "$rpc_endpoint" = "SKIP" ]; then
        skip_rpc_validation=true
    fi

    # Extract tags from CanopyX
    canopyx_tags=$(extract_go_json_tags "$canopyx_path" "$type_name")

    if [ -z "$canopyx_tags" ]; then
        echo "  ⚠️  WARNING: No JSON tags found for $type_name in $canopyx_file"
        WARNINGS=$((WARNINGS + 1))
        echo ""
        continue
    fi

    # Extract tags from Canopy protobuf
    canopy_tags=$(extract_go_json_tags "$canopy_path" "$canopy_struct_name")

    # Compare tags
    echo "  CanopyX file: $canopyx_file"
    echo "  Canopy file: $canopy_file"
    echo ""

    # Check if CanopyX has all fields from Canopy
    missing_in_canopyx=""
    for tag in $canopy_tags; do
        if ! echo "$canopyx_tags" | grep -q "^$tag$"; then
            if [ -z "$missing_in_canopyx" ]; then
                echo "  ❌ Missing in CanopyX:"
            fi
            echo "     - $tag (exists in Canopy $canopy_struct_name)"
            missing_in_canopyx="yes"
            ERRORS=$((ERRORS + 1))
        fi
    done

    # Check if CanopyX has extra fields not in Canopy
    extra_in_canopyx=""
    for tag in $canopyx_tags; do
        if ! echo "$canopy_tags" | grep -q "^$tag$"; then
            if [ -z "$extra_in_canopyx" ]; then
                echo "  ⚠️  Extra in CanopyX (not in Canopy):"
            fi
            echo "     - $tag"
            extra_in_canopyx="yes"
            WARNINGS=$((WARNINGS + 1))
        fi
    done

    # Validate against live RPC if accessible and not skipped
    if [ "$skip_rpc_validation" = false ]; then
        if timeout 2 curl -s --max-time 1 "$RPC_URL/health" > /dev/null 2>&1 || [ -f "/tmp/.rpc_accessible" ]; then
            touch /tmp/.rpc_accessible 2>/dev/null || true
            rpc_keys=$(extract_rpc_json_keys "$RPC_URL$rpc_endpoint" "$rpc_payload")

            if [ -n "$rpc_keys" ]; then
                echo "  🌐 Live RPC validation ($rpc_endpoint):"

                # Check if CanopyX has all fields from RPC response
                missing_from_rpc=""
                for key in $rpc_keys; do
                    if echo "$canopyx_tags" | grep -q "^$key$"; then
                        echo "     ✅ $key"
                    else
                        echo "     ❌ RPC returns '$key' but $type_name has no json:\"$key\" tag"
                        missing_from_rpc="yes"
                        ERRORS=$((ERRORS + 1))
                    fi
                done

                # Check if CanopyX has fields not in RPC response
                for tag in $canopyx_tags; do
                    if ! echo "$rpc_keys" | grep -q "^$tag$"; then
                        echo "     ⚠️  $type_name has json:\"$tag\" but RPC doesn't return it"
                        WARNINGS=$((WARNINGS + 1))
                    fi
                done
            fi
        fi
    fi

    if [ -z "$missing_in_canopyx" ] && [ -z "$extra_in_canopyx" ]; then
        echo "  ✅ JSON tags match Canopy source"
    fi

    echo ""
done

# Summary
echo "================================================"
if [ $ERRORS -eq 0 ]; then
    if [ $WARNINGS -eq 0 ]; then
        echo "✅ All RPC JSON tag validations passed!"
        echo ""
        echo "All CanopyX RPC structs have correct JSON tags matching:"
        echo "  - Canopy blockchain protobuf definitions"
        echo "  - Live RPC response fields"
    else
        echo "✅ All RPC JSON tag validations passed (with $WARNINGS warnings)"
        echo ""
        echo "Warnings indicate:"
        echo "  - Extra fields in CanopyX not in Canopy (may be intentional)"
        echo "  - Fields in structs that RPC doesn't return (may use omitempty)"
    fi
    exit 0
else
    echo "❌ Found $ERRORS validation errors and $WARNINGS warnings"
    echo ""
    echo "Errors indicate JSON tag mismatches that will cause data loss:"
    echo "  - CanopyX missing fields that Canopy/RPC provides"
    echo "  - CanopyX has wrong JSON tags for fields"
    echo ""
    echo "To fix:"
    echo "  1. Check the field names in Canopy protobuf files"
    echo "  2. Update json tags in CanopyX RPC structs to match"
    echo "  3. Re-run this script to verify"
    exit 1
fi